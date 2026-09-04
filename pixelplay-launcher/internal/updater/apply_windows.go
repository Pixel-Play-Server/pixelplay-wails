//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	windowsDetachedProcess      = 0x00000008
	windowsCreateNewProcessGroup = 0x00000200
)

// ApplyAndRelaunch waits for this process to exit, copies the new exe over, then relaunches.
func ApplyAndRelaunch(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("sin binario nuevo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(filepath.Dir(newPath), "apply.log")
	batPath := filepath.Join(filepath.Dir(newPath), "pixelplay-apply-update.bat")
	pid := os.Getpid()
	script := fmt.Sprintf(`@echo off
>>"%s" echo %date% %time% apply start pid=%d new="%s" exe="%s"
:wait
tasklist /FI "PID eq %d" | find "%d" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)
copy /Y "%s" "%s" >>"%s" 2>&1
if errorlevel 1 (
  >>"%s" echo copy failed
  exit /b 1
)
start "" "%s"
del "%%~f0"
`, logPath, pid, newPath, exe, pid, pid, newPath, exe, logPath, logPath, exe)
	if err := os.WriteFile(batPath, []byte(script), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/C", "start", "/B", batPath)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windowsDetachedProcess | windowsCreateNewProcessGroup,
	}
	return cmd.Start()
}
