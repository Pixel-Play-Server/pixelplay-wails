package download

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Default client: no overall Timeout (large modpacks), but bound connect/headers
// so a dead peer cannot hang a launch forever.
var client = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// SmallFile is for many tiny GETs (Minecraft assets). Hard cap per object.
var SmallFile = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   24,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

type ProgressFn func(done, total int64)

// File downloads url into destPath (creates parent dirs).
func File(url, destPath string, onProgress ProgressFn) error {
	return FileWith(client, url, destPath, onProgress)
}

// FileWith is File using a custom client (e.g. SmallFile for assets).
func FileWith(c *http.Client, url, destPath string, onProgress ProgressFn) error {
	if c == nil {
		c = client
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails)")
	res, err := c.Do(req)
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
