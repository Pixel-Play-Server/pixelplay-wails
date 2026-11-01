//go:build !linux

package webkitperm

// Install is a no-op outside Linux (Windows WebView2 already auto-allows via Wails).
func Install() {}
