package protect

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"

	"pixelplay-launcher/internal/paths"
)

//go:embed embed/pixelplay-protect.jar
var agentJar []byte

//go:embed embed/pixelplay-protect.dll
var agentDLL []byte

// JVMAgentArg extracts the capture-protect javaagent and returns a -javaagent: flag.
// gameDir is passed as agent args so in-game "open folder" can be blocked.
// Empty string if the embedded jar is missing.
func JVMAgentArg(gameDir string) string {
	if len(agentJar) < 32 {
		return ""
	}
	dir := filepath.Join(paths.ProtectedRoot(), "_agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	jar := filepath.Join(dir, "pixelplay-protect.jar")
	if err := os.WriteFile(jar, agentJar, 0o600); err != nil {
		return ""
	}
	if runtime.GOOS == "windows" && len(agentDLL) > 64 {
		_ = os.WriteFile(filepath.Join(dir, "pixelplay-protect.dll"), agentDLL, 0o600)
	}
	arg := "-javaagent:" + jar
	if gameDir != "" {
		arg += "=" + gameDir
	}
	return arg
}
