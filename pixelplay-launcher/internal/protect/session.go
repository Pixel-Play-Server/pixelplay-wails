package protect

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DisableScreenshotKey unbinds F2 in options.txt so in-game captures are harder.
func DisableScreenshotKey(gameDir string) {
	p := filepath.Join(gameDir, "options.txt")
	b, err := os.ReadFile(p)
	var lines []string
	if err == nil {
		lines = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	}
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "key_key.screenshot:") {
			lines[i] = "key_key.screenshot:key.keyboard.unknown"
			found = true
		}
	}
	if !found {
		lines = append(lines, "key_key.screenshot:key.keyboard.unknown")
	}
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600)
}

// HardenGameDir tightens permissions so other local users cannot browse the instance.
func HardenGameDir(gameDir string) {
	_ = os.MkdirAll(gameDir, 0o700)
	_ = os.Chmod(gameDir, 0o700)
	for _, name := range []string{"mods", "config", "logs", "crash-reports", "screenshots", "replay_recordings", "resourcepacks", "shaderpacks"} {
		p := filepath.Join(gameDir, name)
		_ = os.MkdirAll(p, 0o700)
		_ = os.Chmod(p, 0o700)
	}
}

// GuardSession watches screenshots/replays and retries OS capture exclusion while the game runs.
func GuardSession(ctx context.Context, pid int, gameDir string) {
	go excludeFromCapture(ctx, pid)
	go wipeCaptureFiles(ctx, gameDir)
}

func wipeCaptureFiles(ctx context.Context, gameDir string) {
	shotDir := filepath.Join(gameDir, "screenshots")
	replayDir := filepath.Join(gameDir, "replay_recordings")
	_ = os.MkdirAll(shotDir, 0o700)
	tick := time.NewTicker(800 * time.Millisecond)
	defer tick.Stop()
	black := blackPNG()
	seen := map[string]struct{}{}
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			n++
			if n == 1 || n%20 == 0 {
				DisableScreenshotKey(gameDir)
			}
			nukeDir(shotDir, black, seen, ".png")
			nukeDir(replayDir, []byte{}, seen, ".mcpr")
		}
	}
}

func nukeDir(dir string, replacement []byte, seen map[string]struct{}, ext string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	ext = strings.ToLower(ext)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ext) {
			continue
		}
		full := filepath.Join(dir, name)
		if _, ok := seen[full]; ok {
			continue
		}
		payload := replacement
		if payload == nil {
			payload = []byte{}
		}
		if err := os.WriteFile(full, payload, 0o600); err == nil {
			seen[full] = struct{}{}
		}
	}
}

func blackPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
