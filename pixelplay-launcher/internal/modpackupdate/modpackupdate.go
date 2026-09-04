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
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails; modpack-updater)")
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
	newer := man.Version != "" && man.Version != currentVersion
	return &man, newer, nil
}

// Download saves the remote .ppack into dataDir/modpack-updates/{id}/
func Download(packID string, man *ppack.VersionManifest) (string, error) {
	if man == nil || man.URL == "" {
		return "", fmt.Errorf("manifest vacío")
	}
	destDir := filepath.Join(paths.DataDir(), "modpack-updates", packID)
	_ = os.MkdirAll(destDir, 0o755)
	name := filepath.Base(man.URL)
	if name == "" || name == "." || name == "/" {
		name = packID + "-" + man.Version + ".ppack"
	}
	dest := filepath.Join(destDir, name)
	if err := download.File(man.URL, dest, nil); err != nil {
		return "", err
	}
	return dest, nil
}
