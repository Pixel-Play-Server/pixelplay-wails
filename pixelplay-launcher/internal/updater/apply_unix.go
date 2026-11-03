//go:build unix && !darwin

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// ApplyAndRelaunch replaces the running binary after this process exits.
// The helper is started outside the current systemd cgroup so the user
// unit cannot kill it when this process calls os.Exit.
func ApplyAndRelaunch(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("sin binario nuevo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	logPath := filepath.Join(filepath.Dir(newPath), "apply.log")
	scriptPath := filepath.Join(filepath.Dir(newPath), "pixelplay-apply-update.sh")
	script := fmt.Sprintf(`#!/bin/bash
exec >>%q 2>&1
echo "$(date -Iseconds) apply start pid=$$ oldpid=%d new=%q exe=%q"
oldpid=%d
for i in $(seq 1 150); do
  if ! kill -0 "$oldpid" 2>/dev/null; then
    echo "$(date -Iseconds) old process gone (loop $i)"
    break
  fi
  sleep 0.2
done
sleep 0.4
if [ ! -f %q ]; then
  echo "missing downloaded binary"
  exit 1
fi
cp -f %q %q
chmod +x %q
echo "$(date -Iseconds) exec %q"
rm -f %q
exec %q
`, logPath, os.Getpid(), newPath, exe, os.Getpid(), newPath, newPath, exe, exe, exe, scriptPath, exe)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}
	return startDetachedApply(scriptPath)
}

func startDetachedApply(scriptPath string) error {
	if systemd, err := exec.LookPath("systemd-run"); err == nil {
		cmd := exec.Command(systemd, "--user", "--scope", "--quiet", "/bin/bash", scriptPath)
		cmd.Env = os.Environ()
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
