package modpackupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"
	"pixelplay.gg/ppack"
)

var client = &http.Client{Timeout: 40 * time.Second}

// Check fetches remote version.json for a pack updater URL.
func Check(currentVersion, updaterURL string) (*ppack.VersionManifest, bool, error) {
	updaterURL = strings.TrimSpace(updaterURL)
	if updaterURL == "" {
		return nil, false, fmt.Errorf("sin updater url")
	}
	req, err := http.NewRequest(http.MethodGet, updaterURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "PixelPlayLauncher/5.0 (wails; modpack-updater)")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	res, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, false, err
	}
	if res.StatusCode >= 400 {
		return nil, false, fmt.Errorf("updater %s: %s", updaterURL, res.Status)
	}
	var man ppack.VersionManifest
	if err := json.Unmarshal(body, &man); err != nil {
		return nil, false, err
	}
	newer := IsNewer(man.Version, currentVersion)
	return &man, newer, nil
}

// IsNewer is a dotted numeric compare (0.3.1 > 0.2.0). Equal or older remotes are not updates.
func IsNewer(remote, local string) bool {
	remote = strings.TrimSpace(remote)
	local = strings.TrimSpace(local)
	if remote == "" {
		return false
	}
	if remote == local {
		return false
	}
	rp := parseVer(remote)
	lp := parseVer(local)
	for i := 0; i < 3; i++ {
		if rp[i] > lp[i] {
			return true
		}
		if rp[i] < lp[i] {
			return false
		}
	}
	return false
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		fmt.Sscanf(parts[i], "%d", &n)
		out[i] = n
	}
	return out
}

// Download saves the remote .ppack into dataDir/modpack-updates/{id}/
func Download(packID string, man *ppack.VersionManifest) (string, error) {
	if man == nil || man.URL == "" {
		return "", fmt.Errorf("manifest vacío")
	}
	destDir := filepath.Join(paths.DataDir(), "modpack-updates", packID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	name := packID + "-" + man.Version + ".ppack"
	dest := filepath.Join(destDir, name)
	if man.SHA256 != "" {
		if got, err := ppack.FileSHA256(dest); err == nil && strings.EqualFold(got, man.SHA256) {
			return dest, nil
		}
	}
	if err := download.File(man.URL, dest, nil); err != nil {
		return "", err
	}
	if man.SHA256 != "" {
		got, err := ppack.FileSHA256(dest)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(got, man.SHA256) {
			_ = os.Remove(dest)
			return "", fmt.Errorf("sha256 del .ppack no coincide")
		}
	}
	return dest, nil
}
