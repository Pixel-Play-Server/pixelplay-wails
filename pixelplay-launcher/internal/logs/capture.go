package logs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/redact"
)

const maxCaptureBytes = 2 << 20

type Snapshot struct {
	LatestLog   string `json:"latest_log"`
	CrashReport string `json:"crash_report"`
}

func (s Snapshot) Combined() string {
	var b strings.Builder
	if strings.TrimSpace(s.CrashReport) != "" {
		b.WriteString("--- CRASH REPORT ---\n")
		b.WriteString(s.CrashReport)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(s.LatestLog) != "" {
		b.WriteString("--- LATEST LOG ---\n")
		b.WriteString(s.LatestLog)
	}
	return strings.TrimSpace(b.String())
}

func CaptureGameDir(gameDir, profileID string) Snapshot {
	snap := Snapshot{
		LatestLog:   readTailFile(filepath.Join(gameDir, "logs", "latest.log"), maxCaptureBytes),
		CrashReport: newestCrashReport(filepath.Join(gameDir, "crash-reports")),
	}
	snap.LatestLog = redact.Line(snap.LatestLog, gameDir)
	snap.CrashReport = redact.Line(snap.CrashReport, gameDir)
	if profileID != "" {
		_ = SaveSnapshot(profileID, snap)
	}
	return snap
}

func SaveSnapshot(profileID string, snap Snapshot) error {
	dir := cacheDir(profileID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.log"), []byte(snap.LatestLog), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "crash-report.txt"), []byte(snap.CrashReport), 0o600)
}

func LoadSnapshot(profileID string) Snapshot {
	dir := cacheDir(profileID)
	latest, _ := os.ReadFile(filepath.Join(dir, "latest.log"))
	crash, _ := os.ReadFile(filepath.Join(dir, "crash-report.txt"))
	return Snapshot{LatestLog: string(latest), CrashReport: string(crash)}
}

func LatestForProfile(profileID string) (string, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return "", err
	}
	fromDisk, err := ReadFile(filepath.Join(paths.ProfileDir(p.Path), "logs", "latest.log"))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(fromDisk) != "" {
		return fromDisk, nil
	}
	return LoadSnapshot(profileID).Combined(), nil
}

func cacheDir(profileID string) string {
	return filepath.Join(paths.DataDir(), "crash-cache", profileID)
}

func newestCrashReport(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(strings.ToLower(n), ".txt") {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return readTailFile(filepath.Join(dir, names[len(names)-1]), maxCaptureBytes)
}

func readTailFile(path string, max int) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	if max > 0 && len(b) > max {
		b = b[len(b)-max:]
	}
	return string(b)
}

func ClipTail(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
