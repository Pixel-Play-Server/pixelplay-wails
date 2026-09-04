package osopen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Path opens a file or directory with the OS file manager / default app.
func Path(target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	openTarget := target
	if !info.IsDir() {
		parent := filepath.Dir(target)
		openTarget = parent
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", openTarget)
	case "darwin":
		cmd = exec.Command("open", openTarget)
	case "windows":
		cmd = exec.Command("explorer", openTarget)
	default:
		return fmt.Errorf("abrir ruta no soportado en %s", runtime.GOOS)
	}
	return cmd.Start()
}
