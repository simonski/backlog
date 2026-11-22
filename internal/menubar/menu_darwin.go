package menubar

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include <stdint.h>

int backlog_menu_run(const char *url, const char *username, int showSettings, uintptr_t handle);
void backlog_menu_stop(void);
*/
import "C"

import (
	"context"
	"errors"
	"os"
	"runtime"
	"runtime/cgo"
	"unsafe"

	"harness.engineering/internal/wire"
)

const Available = true

func init() {
	// AppKit requires the process's initial thread.
	if len(os.Args) > 1 && os.Args[1] == "menu" {
		runtime.LockOSThread()
	}
}

type menuCallbacks struct {
	save         func(Settings) error
	checkVersion func(wire.ServerVersion) bool
}

//export backlogMenuSave
func backlogMenuSave(handle C.uintptr_t, url, username *C.char) *C.char {
	callbacks := cgo.Handle(handle).Value().(menuCallbacks)
	if err := callbacks.save(Settings{DashboardURL: C.GoString(url), Username: C.GoString(username)}); err != nil {
		return C.CString(err.Error())
	}
	return nil
}

//export backlogMenuVersion
func backlogMenuVersion(handle C.uintptr_t, version, semver *C.char, legacy C.long) C.int {
	callbacks := cgo.Handle(handle).Value().(menuCallbacks)
	if callbacks.checkVersion(wire.ServerVersion{Version: C.GoString(version), Protocol: int(legacy), ProtocolSemver: C.GoString(semver)}) {
		return 1
	}
	return 0
}

func Run(ctx context.Context, settings Settings, showSettings bool, save func(Settings) error, checkVersion func(wire.ServerVersion) bool) error {
	if ctx.Err() != nil {
		return nil
	}
	handle := cgo.NewHandle(menuCallbacks{save: save, checkVersion: checkVersion})
	defer handle.Delete()
	address := C.CString(settings.DashboardURL)
	defer C.free(unsafe.Pointer(address))
	username := C.CString(settings.Username)
	defer C.free(unsafe.Pointer(username))
	var prompt C.int
	if showSettings {
		prompt = 1
	}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			C.backlog_menu_stop()
		case <-stopped:
		}
	}()
	if C.backlog_menu_run(address, username, prompt, C.uintptr_t(handle)) != 0 {
		return errors.New("could not start the menu bar item in the macOS desktop session")
	}
	return nil
}
