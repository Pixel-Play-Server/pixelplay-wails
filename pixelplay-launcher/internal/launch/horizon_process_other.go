//go:build !linux && !windows

package launch

import "os/exec"

func configureHorizonProcess(cmd *exec.Cmd) func() error {
	return func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
func horizonRunning(string) bool { return false }
