package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"pixelplay-launcher/internal/config"
)

// DefaultBaseURL is the Pixel SSH/Caddy download host.
const DefaultBaseURL = "https://panel.pixelplay.gg/dl"

// DefaultChannel is "stable" or "beta". Beta builds set this via -ldflags.
var DefaultChannel = "stable"

// AppVersion is the Go/Wails launcher version (semver-ish).
const AppVersion = "5.0.3"

// StatusEvent matches the Tauri updater_status payload shape.
type StatusEvent struct {
	Status      string   `json:"status"`
	Message     string   `json:"message"`
	Progress    *float64 `json:"progress,omitempty"`
	FromVersion string   `json:"from_version,omitempty"`
	ToVersion   string   `json:"to_version,omitempty"`
	URL         string   `json:"url,omitempty"`
}

// Manifest is the Tauri-compatible version.json.
type Manifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

// Emitter sends updater_status events to the frontend.
type Emitter func(StatusEvent)

func baseURL() string {
	if v := strings.TrimSpace(os.Getenv("PIXELPLAY_UPDATER_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultBaseURL
}

// UseBeta reports whether this process should talk to the beta updater tree.
func UseBeta() bool {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("PIXELPLAY_CHANNEL"))); v == "beta" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(DefaultChannel), "beta") {
		return true
	}
	raw, err := config.Raw()
	if err != nil {
		return false
	}
	var cfg struct {
		Check bool `json:"check_beta_channel"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return false
	}
	return cfg.Check
}

func releasesRoot() string {
	base := strings.TrimRight(baseURL(), "/")
	beta := UseBeta()
	switch {
	case strings.Contains(base, "/releases-v2-beta"):
		return base
	case strings.Contains(base, "/releases-v2"):
		if beta {
			return strings.Replace(base, "/releases-v2", "/releases-v2-beta", 1)
		}
		return base
	case beta:
		return base + "/pixelplay/releases-v2-beta"
	default:
		return base + "/pixelplay/releases-v2"
	}
}

func platformTarget() string {
	if v := os.Getenv("PIXELPLAY_UPDATER_PLATFORM_TARGET"); v != "" {
		return v
	}
	switch {
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return "windows-x86_64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "linux-x86_64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "mac-arm"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "mac-intel"
	default:
		return "unknown"
	}
}

// ManifestURL builds the Tauri-style check URL for the current version + channel.
func ManifestURL(currentVersion string) string {
	return fmt.Sprintf("%s/%s/%s/version.json",
		releasesRoot(),
		platformTarget(),
		currentVersion,
	)
}

// Check fetches version.json from the Pixel host and reports whether an update exists.
func Check(currentVersion string, emit Emitter) (*Manifest, bool, error) {
	if emit == nil {
		emit = func(StatusEvent) {}
	}
	emit(StatusEvent{Status: "checking", Message: "Checking for updates on panel.pixelplay.gg…"})

	url := ManifestURL(currentVersion)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		emit(StatusEvent{Status: "error", Message: fmt.Sprintf("No se pudo contactar el updater: %v", err)})
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		emit(StatusEvent{Status: "upToDate", Message: "Ya estás en la última versión."})
		emit(StatusEvent{Status: "close", Message: "up to date"})
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Updater HTTP %d (%s)", resp.StatusCode, url)
		emit(StatusEvent{Status: "error", Message: msg})
		emit(StatusEvent{Status: "close", Message: msg})
		return nil, false, fmt.Errorf("%s", msg)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		emit(StatusEvent{Status: "error", Message: err.Error()})
		return nil, false, err
	}
	var man Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		emit(StatusEvent{Status: "error", Message: "version.json inválido"})
		return nil, false, err
	}
	if man.Version == "" || man.Version == currentVersion || !isNewer(man.Version, currentVersion) {
		emit(StatusEvent{Status: "upToDate", Message: "Ya estás en la última versión."})
		emit(StatusEvent{Status: "close", Message: "up to date"})
		return &man, false, nil
	}

	emit(StatusEvent{
		Status:      "available",
		Message:     fmt.Sprintf("Actualización disponible: %s → %s", currentVersion, man.Version),
		FromVersion: currentVersion,
		ToVersion:   man.Version,
		URL:         man.URL,
	})
	return &man, true, nil
}

// Download pulls the update binary into destDir and returns the local path.
func Download(man *Manifest, destDir string, emit Emitter) (string, error) {
	if emit == nil {
		emit = func(StatusEvent) {}
	}
	if man == nil || man.URL == "" {
		return "", fmt.Errorf("manifest sin url")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	emit(StatusEvent{Status: "downloading", Message: "Descargando actualización…"})
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(man.URL)
	if err != nil {
		emit(StatusEvent{Status: "error", Message: err.Error()})
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("download HTTP %d", resp.StatusCode)
		emit(StatusEvent{Status: "error", Message: msg})
		return "", fmt.Errorf("%s", msg)
	}

	name := filepath.Base(strings.Split(man.URL, "?")[0])
	if name == "" || name == "." || name == "/" {
		name = fmt.Sprintf("pixelplay-launcher-%s", man.Version)
	}
	outPath := filepath.Join(destDir, name)
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var written int64
	buf := make([]byte, 32*1024)
	var total int64 = resp.ContentLength
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return "", werr
			}
			written += int64(n)
			if total > 0 {
				p := float64(written) / float64(total)
				emit(StatusEvent{Status: "downloading", Message: "Descargando…", Progress: &p})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			emit(StatusEvent{Status: "error", Message: readErr.Error()})
			return "", readErr
		}
	}
	_ = os.Chmod(outPath, 0o755)
	emit(StatusEvent{Status: "done", Message: fmt.Sprintf("Descarga lista: %s", outPath)})
	return outPath, nil
}

// isNewer does a loose dotted numeric compare (4.0.1 > 4.0.0; 0.2.0-wails ~ 0.2.0).
func isNewer(remote, local string) bool {
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
	v = strings.TrimPrefix(strings.ToLower(v), "v")
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
