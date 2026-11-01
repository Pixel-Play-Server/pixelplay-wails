package protect

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"pixelplay-launcher/internal/redact"
)

// ScrubLogs periodically masks filesystem paths in Minecraft log files.
func ScrubLogs(ctx context.Context, gameDir string) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	scrub := func() {
		logsDir := filepath.Join(gameDir, "logs")
		for _, name := range []string{"latest.log", "debug.log"} {
			redact.PatchFileInPlace(filepath.Join(logsDir, name), gameDir)
		}
		crashDir := filepath.Join(gameDir, "crash-reports")
		entries, err := os.ReadDir(crashDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if filepath.Ext(n) == ".txt" || filepath.Ext(n) == ".log" {
				redact.PatchFileInPlace(filepath.Join(crashDir, n), gameDir)
			}
		}
	}
	scrub()
	for {
		select {
		case <-ctx.Done():
			scrub()
			return
		case <-tick.C:
			scrub()
		}
	}
}
