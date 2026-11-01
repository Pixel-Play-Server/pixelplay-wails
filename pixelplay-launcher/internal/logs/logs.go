package logs

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/redact"
)

func ListForProfile(profileID string) ([]string, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(paths.ProfileDir(p.Path), "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out, nil
}

func ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		defer zr.Close()
		plain, err := io.ReadAll(zr)
		if err != nil {
			return "", err
		}
		b = plain
	}
	gameDir := filepath.Dir(filepath.Dir(path))
	return redact.Line(string(b), gameDir), nil
}

func UploadMCLogs(content string) (string, error) {
	content = redact.Line(content)
	var dest struct {
		Success bool   `json:"success"`
		ID      string `json:"id"`
		URL     string `json:"url"`
		Error   string `json:"error"`
	}
	form := url.Values{}
	form.Set("content", content)
	if err := httpx.PostForm("https://api.mclo.gs/1/log", form, &dest); err != nil {
		return "", err
	}
	if !dest.Success || dest.URL == "" {
		msg := dest.Error
		if msg == "" {
			msg = "mclo.gs rechazó el log"
		}
		return "", fmt.Errorf("%s", msg)
	}
	return dest.URL, nil
}
