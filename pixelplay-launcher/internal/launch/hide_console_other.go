//go:build !windows

package launch

import "os/exec"

func hideGameConsole(cmd *exec.Cmd) {}
