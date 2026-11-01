package launch

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

// The native engine is the tracked process; no launcher parent is involved.
func configureHorizonProcess(cmd *exec.Cmd) func() error {
	return func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
func horizonRunning(gameDir string) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if windows.Process32First(snapshot, &entry) != nil {
		return false
	}
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "horizon-client.exe") {
			handle, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if e == nil {
				buf := make([]uint16, 32768)
				size := uint32(len(buf))
				e = windows.QueryFullProcessImageName(handle, 0, &buf[0], &size)
				windows.CloseHandle(handle)
				if e == nil && strings.HasPrefix(strings.ToLower(windows.UTF16ToString(buf[:size])), strings.ToLower(filepath.Clean(gameDir)+string(filepath.Separator))) {
					return true
				}
			}
		}
		if windows.Process32Next(snapshot, &entry) != nil {
			break
		}
	}
	return false
}
