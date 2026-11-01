//go:build !windows

package notify

func desktopWindows(title, body string) {
	// no-op on non-Windows builds; Desktop() never calls this.
	_, _ = title, body
}
