package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness.engineering/internal/version"
)

func TestServeStopShutsDownRunningServer(t *testing.T) {
	app := isolatedServerApp(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	reader, logs := io.Pipe()
	defer reader.Close()
	defer logs.Close()
	serverExit := make(chan int, 1)
	database := filepath.Join(t.TempDir(), "telemetry.db")
	go func() {
		serverExit <- app.Run(ctx, []string{"serve", "-addr", "127.0.0.1:0", "-db", database}, nil, io.Discard, logs)
		logs.Close()
	}()
	ready := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(reader)
		if scan.Scan() {
			ready <- scan.Text()
		}
		for scan.Scan() {
		}
	}()
	var address string
	select {
	case line := <-ready:
		var found bool
		address, found = strings.CutPrefix(line, "Local dashboard: http://")
		if !found {
			t.Fatalf("unexpected startup message: %s", line)
		}
	case <-ctx.Done():
		t.Fatal("server did not start")
	}
	var out, diagnostics bytes.Buffer
	warning := version.Warning(version.Client, version.Server)
	if code := app.Run(ctx, []string{"serve", "-stop", "-addr", address}, nil, &out, &diagnostics); code != 0 ||
		!strings.Contains(out.String(), "Stopped Backlog") || strings.TrimSpace(diagnostics.String()) != warning {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
	}
	select {
	case code := <-serverExit:
		if code != 0 {
			t.Fatalf("server exited with code %d", code)
		}
	case <-ctx.Done():
		t.Fatal("server did not return after the stop request")
	}
}

func TestServeStopSkipsUnrecognizedListener(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Error("stop was sent to an unrecognized listener")
		}
		http.NotFound(w, r)
	}))
	defer service.Close()
	var out, diagnostics bytes.Buffer
	address := strings.TrimPrefix(service.URL, "http://")
	if code := (App{}).Run(t.Context(), []string{"serve", "-stop", "-addr", address}, nil, &out, &diagnostics); code != 1 ||
		!strings.Contains(diagnostics.String(), "server version unavailable") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
	}
}

func TestServeReportsOccupiedPortBeforeOpeningDatabase(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version bool
		want    string
	}{
		{"another listener", false, "The listener did not identify as Backlog"},
		{"existing Backlog", true, "Backlog 0.10.0 is already running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.version && r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"server_version":"0.10.0","protocol_version":1}`)
					return
				}
				http.NotFound(w, r)
			}))
			defer service.Close()
			_, port, err := net.SplitHostPort(service.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("HARNESS_ADDR", "")
			database := filepath.Join(t.TempDir(), "telemetry.db")
			var out, diagnostics bytes.Buffer
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			code := isolatedServerApp(t).Run(ctx, []string{"serve", "-p", port, "-db", database}, nil, &out, &diagnostics)
			if code != 1 || !strings.Contains(diagnostics.String(), tc.want) ||
				!strings.Contains(diagnostics.String(), port) {
				t.Fatalf("exit=%d stderr=%q", code, &diagnostics)
			}
			if _, err := os.Stat(database); !os.IsNotExist(err) {
				t.Fatalf("serve touched the database after a bind failure: %v", err)
			}
		})
	}
}

func isolatedServerApp(t *testing.T) App {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "backlog-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return App{runtime: serverRuntime(directory)}
}

func TestConcurrentServeAndStopOnCustomPort(t *testing.T) {
	app := isolatedServerApp(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	type result struct {
		code int
		log  string
	}
	exits := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		args := []string{"serve", "-addr", "127.0.0.1:0", "-db", filepath.Join(t.TempDir(), "telemetry.db")}
		go func() {
			<-start
			var diagnostics bytes.Buffer
			code := app.Run(ctx, args, nil, io.Discard, &diagnostics)
			exits <- result{code, diagnostics.String()}
		}()
	}
	close(start)
	select {
	case first := <-exits:
		if first.code != 1 || !strings.Contains(first.log, errServerRunning.Error()) {
			t.Fatalf("concurrent start exited %d: %s", first.code, first.log)
		}
	case <-ctx.Done():
		t.Fatal("concurrent starts did not settle")
	}
	for {
		if instance, err := app.runtime.control(ctx, http.MethodGet, "/status"); err == nil {
			if !strings.HasPrefix(instance.Address, "127.0.0.1:") {
				t.Fatalf("unexpected running address: %s", instance.Address)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("winning server did not become ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
	var out, diagnostics bytes.Buffer
	if code := app.Run(ctx, []string{"stop"}, nil, &out, &diagnostics); code != 0 || !strings.Contains(out.String(), "Stopped Backlog") {
		t.Fatalf("stop exited %d, stdout=%q stderr=%q", code, &out, &diagnostics)
	}
	select {
	case second := <-exits:
		if second.code != 0 {
			t.Fatalf("server exited %d: %s", second.code, second.log)
		}
	case <-ctx.Done():
		t.Fatal("winning server did not stop")
	}
}
