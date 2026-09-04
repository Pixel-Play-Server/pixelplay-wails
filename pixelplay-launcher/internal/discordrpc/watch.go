package discordrpc

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
)

// WatchLatestLog tails Minecraft's logs/latest.log and calls onLine for each new line.
// Starts at EOF so a previous session is not replayed; if the file is rotated/truncated
// (normal on a new game start) it reads the new file from the beginning.
func WatchLatestLog(ctx context.Context, path string, onLine func(string)) {
	if onLine == nil {
		return
	}
	var offset int64
	seen := false
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}

		fi, err := os.Stat(path)
		if err != nil {
			seen = false
			offset = 0
			continue
		}
		if !seen {
			offset = fi.Size()
			seen = true
			continue
		}
		if fi.Size() < offset {
			offset = 0
		}
		if fi.Size() == offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			onLine(sc.Text())
		}
		_ = f.Close()
		offset = fi.Size()
	}
}
