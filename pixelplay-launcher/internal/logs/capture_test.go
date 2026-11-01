package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureGameDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "crash-reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "latest.log"), []byte("[main/ERROR]: boom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crash-reports", "crash-2026-09-04.txt"), []byte("---- Minecraft Crash Report ----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXELPLAY_DATA_DIR", t.TempDir())
	snap := CaptureGameDir(dir, "profile-1")
	if !strings.Contains(snap.LatestLog, "boom") {
		t.Fatalf("latest %q", snap.LatestLog)
	}
	if !strings.Contains(snap.CrashReport, "Crash Report") {
		t.Fatalf("crash %q", snap.CrashReport)
	}
	cached := LoadSnapshot("profile-1")
	if cached.LatestLog != snap.LatestLog {
		t.Fatalf("cache mismatch")
	}
}
