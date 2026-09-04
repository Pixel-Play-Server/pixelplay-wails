package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var client = &http.Client{Timeout: 0} // no global timeout; large modpacks

type ProgressFn func(done, total int64)

// File downloads url into destPath (creates parent dirs).
func File(url, destPath string, onProgress ProgressFn) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails)")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("download %s: %s", url, res.Status)
	}
	tmp := destPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()

	var done int64
	total := res.ContentLength
	buf := make([]byte, 256*1024)
	lastReport := time.Now()
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				os.Remove(tmp)
				return werr
			}
			done += int64(n)
			if onProgress != nil && time.Since(lastReport) > 200*time.Millisecond {
				onProgress(done, total)
				lastReport = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			os.Remove(tmp)
			return rerr
		}
	}
	if onProgress != nil {
		onProgress(done, total)
	}
	_ = f.Close()
	return os.Rename(tmp, destPath)
}
