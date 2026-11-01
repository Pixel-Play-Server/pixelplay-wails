//go:build windows

package launch

import (
	"os/exec"
	"syscall"
)

func hideGameConsole(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
