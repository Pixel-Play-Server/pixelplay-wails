//go:build windows

package notify

import (
	"fmt"
	"os"
	"sync"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

const windowsAppID = "PixelPlay.Launcher"

var windowsOnce sync.Once

func ensureWindowsAppData() {
	windowsOnce.Do(func() {
		exe, _ := os.Executable()
		_ = toast.SetAppData(toast.AppData{
			AppID:         windowsAppID,
			ActivationExe: exe,
		})
	})
}

func desktopWindows(title, body string) {
	ensureWindowsAppData()
	n := toast.Notification{
		AppID:    windowsAppID,
		Title:    title,
		Body:     body,
		Audio:    toast.Default,
		Duration: toast.Long,
	}
	if err := n.Push(); err != nil {
		fmt.Fprintf(os.Stderr, "[notify] windows toast: %v (%s — %s)\n", err, title, body)
		desktopWindowsPowerShellFallback(title, body)
	}
}
