package launch

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func configureHorizonProcess(cmd *exec.Cmd) func() error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stop := func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Cancel = stop
	return stop
}

// Inspect only the engine and its home, never its command line or account data.
func horizonRunning(gameDir string) bool {
	entries, _ := os.ReadDir("/proc")
	for _, p := range entries {
		root := filepath.Join("/proc", p.Name())
		binary, err := os.Readlink(filepath.Join(root, "exe"))
		if err != nil || filepath.Base(binary) != "horizon-client" {
			continue
		}
		env, err := os.ReadFile(filepath.Join(root, "environ"))
		if err != nil {
			continue
		}
		for _, entry := range bytes.Split(env, []byte{0}) {
			if string(entry) == "HORIZON_HOME="+gameDir {
				return true
			}
		}
	}
	return false
}
