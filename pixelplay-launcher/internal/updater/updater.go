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

// PrivateBuild disables public update checks for local experimental builds.
var PrivateBuild = ""

// AppVersion is kept for smoke tools; prefer version.App in production paths.
const AppVersion = "6.0.7"

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

func releasesRootFor(beta bool) string {
	base := strings.TrimRight(baseURL(), "/")
	switch {
	case strings.Contains(base, "/releases-v2-beta"):
		if beta {
			return base
		}
		return strings.Replace(base, "/releases-v2-beta", "/releases-v2", 1)
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

func releasesRoot() string {
	return releasesRootFor(UseBeta())
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

// LatestManifestURL points at the channel tip (always backfilled on publish).
func LatestManifestURL() string {
	return fmt.Sprintf("%s/%s/latest/version.json", releasesRoot(), platformTarget())
}

func candidateManifestURLs(currentVersion string) []string {
	plat := platformTarget()
	seen := map[string]bool{}
	var out []string
	add := func(root, ver string) {
		u := fmt.Sprintf("%s/%s/%s/version.json", root, plat, ver)
		if seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	// Always consult stable tip so beta users aren't stuck behind a stale beta tree.
	stableRoot := releasesRootFor(false)
	add(stableRoot, currentVersion)
	add(stableRoot, "latest")
	if UseBeta() {
		betaRoot := releasesRootFor(true)
		add(betaRoot, currentVersion)
		add(betaRoot, "latest")
	}
	return out
}

func fetchManifest(url string) (*Manifest, int, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var man Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		return nil, resp.StatusCode, err
	}
	return &man, resp.StatusCode, nil
}

// Check fetches version.json from the Pixel host and reports whether an update exists.
// It checks the per-version folder (Tauri-style) and always also consults /latest so
// Windows/Linux clients aren't stuck when a backfill was missed for their build.
// If the beta channel is enabled, stable tip is still considered so a stale beta
// tree cannot hide newer stable releases.
func Check(currentVersion string, emit Emitter) (*Manifest, bool, error) {
	if PrivateBuild == "true" {
		if emit != nil {
			emit(StatusEvent{Status: "upToDate", Message: "Compilación privada local: actualizaciones públicas desactivadas"})
			emit(StatusEvent{Status: "close", Message: "private build"})
		}
		return nil, false, nil
	}
	if emit == nil {
		emit = func(StatusEvent) {}
	}
	emit(StatusEvent{Status: "checking", Message: "Checking for updates on panel.pixelplay.gg…"})

	candidates := candidateManifestURLs(currentVersion)
	var best *Manifest
	var lastErr error
	sawOK := false
	for _, url := range candidates {
		man, code, err := fetchManifest(url)
		if err != nil {
			lastErr = err
			continue
		}
		if code == http.StatusNotFound {
			continue
		}
		if code != http.StatusOK || man == nil || man.Version == "" {
			if code != 0 && code != http.StatusOK {
				lastErr = fmt.Errorf("Updater HTTP %d (%s)", code, url)
			}
			continue
		}
		sawOK = true
		if man.Version == currentVersion || !isNewer(man.Version, currentVersion) {
			continue
		}
		if best == nil || isNewer(man.Version, best.Version) {
			cp := *man
			best = &cp
		}
	}

	if best != nil {
		emit(StatusEvent{
			Status:      "available",
			Message:     fmt.Sprintf("Actualización disponible: %s → %s", currentVersion, best.Version),
			FromVersion: currentVersion,
			ToVersion:   best.Version,
			URL:         best.URL,
		})
		return best, true, nil
	}
	if !sawOK && lastErr != nil {
		emit(StatusEvent{Status: "error", Message: fmt.Sprintf("No se pudo contactar el updater: %v", lastErr)})
		emit(StatusEvent{Status: "close", Message: lastErr.Error()})
		return nil, false, lastErr
	}
	emit(StatusEvent{Status: "upToDate", Message: "Ya estás en la última versión."})
	emit(StatusEvent{Status: "close", Message: "up to date"})
	return nil, false, nil
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
	url := DropInURL(man.URL)
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
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

	name := filepath.Base(strings.Split(url, "?")[0])
	if name == "" || name == "." || name == "/" || looksLikeInstallerName(name) {
		name = "pixelplay-launcher.exe"
		if runtime.GOOS != "windows" {
			name = "pixelplay-launcher"
		}
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

// DropInURL forces in-app updates onto the raw binary, never the NSIS setup.
// Web installs use PixelPlaySetup.exe; the launcher updater always drop-ins .exe/.bin.
func DropInURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	lower := strings.ToLower(u)
	if !strings.Contains(lower, "installer") && !strings.Contains(lower, "setup") {
		return u
	}
	// …/pixelplay-launcher-amd64-installer.exe → …/pixelplay-launcher.exe
	if i := strings.LastIndex(u, "/"); i >= 0 {
		dir := u[:i+1]
		if strings.HasSuffix(lower, ".exe") || strings.Contains(lower, "windows") {
			return dir + "pixelplay-launcher.exe"
		}
		return dir + "pixelplay-launcher"
	}
	return u
}

func looksLikeInstallerName(name string) bool {
	b := strings.ToLower(name)
	return strings.Contains(b, "installer") || strings.Contains(b, "setup")
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
