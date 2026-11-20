package menubar

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework WebKit
#include <stdlib.h>

int backlog_menu_run(const char *url);
void backlog_menu_stop(void);
*/
import "C"

import (
	"context"
	"errors"
	"os"
	"runtime"
	"unsafe"
)

const Available = true

func init() {
	// AppKit requires the process's initial thread.
	if len(os.Args) > 1 && os.Args[1] == "menu" {
		runtime.LockOSThread()
	}
}

func Run(ctx context.Context, dashboardURL string) error {
	if ctx.Err() != nil {
		return nil
	}
	address := C.CString(dashboardURL)
	defer C.free(unsafe.Pointer(address))
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			C.backlog_menu_stop()
		case <-stopped:
		}
	}()
	if C.backlog_menu_run(address) != 0 {
		return errors.New("could not start the menu bar item in the macOS desktop session")
	}
	return nil
}
