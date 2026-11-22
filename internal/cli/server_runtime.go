package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

const serverLockFD = "BACKLOG_SERVER_LOCK_FD"

var errServerRunning = errors.New("Backlog is already running or starting on this host")

type serverRuntime string

type runningServer struct {
	Address string `json:"address"`
	PID     int    `json:"pid"`
}

func (runtime serverRuntime) directory() string {
	if runtime == "" {
		return "/tmp"
	}
	return string(runtime)
}

func (runtime serverRuntime) controlPath() string {
	return filepath.Join(runtime.directory(), "backlog-server-"+strconv.Itoa(os.Geteuid()), "control.sock")
}

func (runtime serverRuntime) lock(adopt bool) (*os.File, error) {
	path := filepath.Join(runtime.directory(), "backlog-server.lock")
	if adopt {
		if value := os.Getenv(serverLockFD); value != "" {
			fd, err := strconv.Atoi(value)
			if err != nil || fd < 3 {
				return nil, errors.New("invalid inherited Backlog server lock")
			}
			file := os.NewFile(uintptr(fd), path)
			opened, openedErr := file.Stat()
			current, currentErr := os.Lstat(path)
			if openedErr != nil || currentErr != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
				file.Close()
				return nil, errors.New("cannot verify inherited Backlog server lock")
			}
			unix.CloseOnExec(fd)
			return lockServerFile(file)
		}
	}
	const flags = unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	fd, err := unix.Open(path, flags, 0)
	if errors.Is(err, unix.ENOENT) {
		fd, err = unix.Open(path, flags|unix.O_CREAT|unix.O_EXCL, 0o644)
		if errors.Is(err, unix.EEXIST) {
			fd, err = unix.Open(path, flags, 0)
		} else if err == nil {
			err = unix.Fchmod(fd, 0o644)
			if err != nil {
				unix.Close(fd)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("open Backlog server lock: %w", err)
	}
	return lockServerFile(os.NewFile(uintptr(fd), path))
}

func lockServerFile(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("server lock is not a regular file")
	}
	if err == nil {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	}
	if err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errServerRunning
		}
		return nil, fmt.Errorf("lock Backlog server: %w", err)
	}
	return file, nil
}

func (runtime serverRuntime) checkControlDirectory() error {
	var stat unix.Stat_t
	if err := unix.Lstat(filepath.Dir(runtime.controlPath()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o777 != 0o700 || int(stat.Uid) != os.Geteuid() {
		return errors.New("Backlog control directory must be private and owned by the server user")
	}
	return nil
}

func (runtime serverRuntime) openControl(address string, stop func()) (func(), error) {
	path := runtime.controlPath()
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if err := runtime.checkControlDirectory(); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot replace existing Backlog control path")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	instance := runningServer{Address: address, PID: os.Getpid()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(instance)
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pid") != strconv.Itoa(instance.PID) {
			http.Error(w, "Backlog instance changed before shutdown", http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(instance)
		stop()
	})
	control := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second}
	go control.Serve(listener)
	return func() {
		listener.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := control.Shutdown(ctx); err != nil {
			control.Close()
		}
	}, nil
}

func (runtime serverRuntime) control(ctx context.Context, method, path string) (runningServer, error) {
	var instance runningServer
	if err := runtime.checkControlDirectory(); err != nil {
		return instance, err
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", runtime.controlPath())
		},
	}}
	defer client.CloseIdleConnections()
	err := request(ctx, client, method, "http://backlog.local"+path, nil, &instance, 4096)
	return instance, err
}
