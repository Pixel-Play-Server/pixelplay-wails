//go:build windows

package protect

import (
	"context"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const wdaExcludeFromCapture = 0x00000011

var (
	modUser32                    = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = modUser32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procSetWindowDisplayAffinity = modUser32.NewProc("SetWindowDisplayAffinity")
	procIsWindowVisible          = modUser32.NewProc("IsWindowVisible")
	capturePID                   atomic.Int32
	enumCB                       uintptr
)

func excludeFromCapture(ctx context.Context, pid int) {
	if pid <= 0 {
		return
	}
	capturePID.Store(int32(pid))
	if enumCB == 0 {
		enumCB = syscall.NewCallback(func(hwnd syscall.Handle, lparam uintptr) uintptr {
			var got uint32
			_, _, _ = procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&got)))
			if int32(got) != capturePID.Load() {
				return 1
			}
			vis, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
			if vis == 0 {
				return 1
			}
			_, _, _ = procSetWindowDisplayAffinity.Call(uintptr(hwnd), wdaExcludeFromCapture)
			return 1
		})
	}
	apply := func() {
		_, _, _ = procEnumWindows.Call(enumCB, 0)
	}
	apply()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for i := 0; i < 90; i++ {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			apply()
		}
	}
}
