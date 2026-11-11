package horizon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const nightlyAPI = "https://api.github.com/repos/horizon-rust/horizon/releases/tags/nightly"
const releaseBase = "https://github.com/horizon-rust/horizon/releases/download/"
const updateInterval = time.Hour

// UpdateStatus is profile-local metadata; it never contains account or session data.
type UpdateStatus struct {
	Version          string    `json:"version"`
	Channel          string    `json:"channel"`
	MinecraftVersion string    `json:"minecraft_version"`
	CheckedAt        time.Time `json:"checked_at"`
	LastError        string    `json:"last_error,omitempty"`
	Automatic        bool      `json:"automatic"`
	Installed        bool      `json:"installed"`
}
type updateState struct {
	Active string `json:"active"`
	Digest string `json:"digest"`
	UpdateStatus
}
type runtimeRelease struct{ Version, Asset, Digest, URL string }

var updateLocks sync.Map
var managedDir = regexp.MustCompile(`^(?:nightly-[a-f0-9]{64}|v[0-9]+\.[0-9]+\.[0-9]+)$`)

// Status reports the engine selected for this profile without initiating network requests.
func Status(gameDir string) UpdateStatus {
	state := readUpdateState(gameDir)
	state.Automatic = true
	state.MinecraftVersion = Minecraft
	r, ok := Find(gameDir)
	state.Installed = ok
	if ok && state.Version == "" {
		state.Version = filepath.Base(r.Root)
		state.Channel = "legacy"
	}
	return state.UpdateStatus
}
func statePath(gameDir string) string {
	return filepath.Join(gameDir, ".horizon-native", "update.json")
}
func readUpdateState(gameDir string) updateState {
	var state updateState
	b, err := os.ReadFile(statePath(gameDir))
	if err == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}
func writeUpdateState(gameDir string, state updateState) error {
	path := statePath(gameDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = json.NewEncoder(tmp).Encode(state)
	ce := tmp.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp.Name(), path)
}

// Find selects only the committed update, then the known legacy release. An unfinished
// download, a directory mtime or a native-mod build must never silently change the runtime.
func Find(gameDir string) (Runtime, bool) {
	state := readUpdateState(gameDir)
	if managedDir.MatchString(state.Active) {
		r := at(filepath.Join(gameDir, ".horizon-native", state.Active))
		if ready(r) {
			return r, true
		}
	}
	for _, parent := range []string{".horizon-native", filepath.Join(".horizon-launcher", "versions")} {
		r := at(filepath.Join(gameDir, parent, Version))
		if ready(r) {
			return r, true
		}
	}
	return Runtime{}, false
}
func assetName(osName, arch string) (string, error) {
	switch osName + "/" + arch {
	case "linux/amd64":
		return "horizon-linux-x86_64.tar.gz", nil
	case "windows/amd64":
		return "horizon-windows-x64.zip", nil
	case "windows/arm64":
		return "horizon-windows-arm64.zip", nil
	case "darwin/arm64":
		return "horizon-macos-arm64.zip", nil
	case "darwin/amd64":
		return "horizon-macos-x64.zip", nil
	}
	return "", fmt.Errorf("Horizon requiere Linux x86_64, Windows x86_64/ARM64 o macOS Intel/Apple Silicon")
}
func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}

func discoverRelease(ctx context.Context, asset string) (runtimeRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nightlyAPI, nil)
	if err != nil {
		return runtimeRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PixelPlay-Horizon-Updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return runtimeRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return runtimeRelease{}, fmt.Errorf("consulta Horizon: HTTP %d", resp.StatusCode)
	}
	return decodeRelease(io.LimitReader(resp.Body, 2<<20), asset)
}
func decodeRelease(reader io.Reader, asset string) (runtimeRelease, error) {
	var release struct {
		Tag    string `json:"tag_name"`
		Draft  bool   `json:"draft"`
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(reader).Decode(&release); err != nil {
		return runtimeRelease{}, err
	}
	if release.Draft || release.Tag != "nightly" {
		return runtimeRelease{}, fmt.Errorf("canal Horizon inesperado")
	}
	for _, a := range release.Assets {
		if a.Name != asset {
			continue
		}
		digest := strings.TrimPrefix(a.Digest, "sha256:")
		if a.Digest != "sha256:"+digest || !validDigest(digest) || a.Size <= 0 || a.Size > 2<<30 || a.URL != releaseBase+"nightly/"+asset {
			return runtimeRelease{}, fmt.Errorf("release Horizon sin origen y SHA-256 verificables")
		}
		return runtimeRelease{"nightly-" + digest, asset, digest, a.URL}, nil
	}
	return runtimeRelease{}, fmt.Errorf("Nightly no publica %s", asset)
}

type updateOperations struct {
	discover func(context.Context, string) (runtimeRelease, error)
	install  func(context.Context, string, runtimeRelease, func(string)) (Runtime, error)
	now      func() time.Time
}

func Ensure(ctx context.Context, gameDir string, emit func(string)) (Runtime, error) {
	return ensureUpdated(ctx, gameDir, emit, updateOperations{discoverRelease, installRelease, time.Now})
}
func ensureUpdated(ctx context.Context, gameDir string, emit func(string), ops updateOperations) (Runtime, error) {
	if emit == nil {
		emit = func(string) {}
	}
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	key, err := filepath.Abs(gameDir)
	if err != nil {
		return Runtime{}, err
	}
	raw, _ := updateLocks.LoadOrStore(key, make(chan struct{}, 1))
	lock := raw.(chan struct{})
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return Runtime{}, ctx.Err()
	}
	asset, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Runtime{}, err
	}
	old, haveOld := Find(gameDir)
	state := readUpdateState(gameDir)
	age := ops.now().Sub(state.CheckedAt)
	if haveOld && !state.CheckedAt.IsZero() && age >= 0 && age < updateInterval && state.LastError == "" {
		return old, nil
	}
	emit("Buscando actualizaciones de Horizon…")
	latest, err := ops.discover(ctx, asset)
	if ctx.Err() != nil {
		return Runtime{}, ctx.Err()
	}
	state.CheckedAt = ops.now().UTC()
	state.Automatic = true
	state.MinecraftVersion = Minecraft
	fallback := func(cause error) (Runtime, error) {
		state.LastError = cause.Error()
		_ = writeUpdateState(gameDir, state)
		if haveOld {
			emit("Horizon: no se pudo actualizar; se conserva el motor instalado. " + cause.Error())
			return old, nil
		}
		return Runtime{}, cause
	}
	if err != nil {
		if haveOld {
			return fallback(err)
		}
		// A first install can still use the checksum pinned by this Pixel Play build.
		name, digest, pinnedErr := releaseAsset(runtime.GOOS, runtime.GOARCH)
		if pinnedErr != nil {
			return fallback(err)
		}
		emit("No se pudo consultar Nightly; instalando la versión verificada disponible")
		latest = runtimeRelease{Version, name, digest, releaseBase + Version + "/" + name}
	}
	if haveOld && state.Digest == latest.Digest && filepath.Base(old.Root) == latest.Version {
		state.LastError = ""
		if err := writeUpdateState(gameDir, state); err != nil {
			return old, nil
		}
		return old, nil
	}
	label := latest.Version
	if len(label) > 20 {
		label = label[:20]
	}
	emit("Actualizando motor Horizon (" + label + ")…")
	installed, err := ops.install(ctx, gameDir, latest, emit)
	if ctx.Err() != nil {
		return Runtime{}, ctx.Err()
	}
	if err != nil {
		return fallback(err)
	}
	if !ready(installed) {
		return fallback(fmt.Errorf("Horizon no terminó de preparar sus recursos"))
	}
	state.Active = latest.Version
	state.Digest = latest.Digest
	state.Version = label
	state.Channel = "nightly"
	if latest.Version == Version {
		state.Channel = "stable-fallback"
	}
	state.LastError = ""
	state.Installed = true
	if err := writeUpdateState(gameDir, state); err != nil {
		return Runtime{}, fmt.Errorf("activar Horizon: %w", err)
	}
	emit("Horizon actualizado; mundos, mods y opciones del perfil conservados")
	return installed, nil
}

func checkRuntimeCompatibility(root string) error {
	b, err := os.ReadFile(filepath.Join(root, "research-lock.json"))
	if err != nil {
		return fmt.Errorf("Horizon sin manifiesto de compatibilidad: %w", err)
	}
	var lock struct {
		Minecraft struct {
			Version string `json:"version"`
			Java    int    `json:"java_major"`
		} `json:"minecraft"`
	}
	if err = json.Unmarshal(b, &lock); err != nil {
		return err
	}
	if lock.Minecraft.Version != Minecraft || lock.Minecraft.Java != 25 {
		return fmt.Errorf("este motor requiere Minecraft %s / Java %d; el perfil usa %s / Java 25", lock.Minecraft.Version, lock.Minecraft.Java, Minecraft)
	}
	return nil
}
func installRelease(ctx context.Context, gameDir string, release runtimeRelease, emit func(string)) (Runtime, error) {
	return installReleaseWith(ctx, gameDir, release, emit, prepare)
}
func installReleaseWith(ctx context.Context, gameDir string, release runtimeRelease, emit func(string), prepareFn func(context.Context, Runtime, string, func(string)) error) (Runtime, error) {
	if !managedDir.MatchString(release.Version) || !validDigest(release.Digest) {
		return Runtime{}, fmt.Errorf("release Horizon inválida")
	}
	parent := filepath.Join(gameDir, ".horizon-native")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return Runtime{}, err
	}
	target := at(filepath.Join(parent, release.Version))
	// Only completed installations carry this marker. It also avoids reusing a partial runtime.
	marker := filepath.Join(target.Root, "pixelplay-runtime.sha256")
	if b, e := os.ReadFile(marker); e == nil && string(b) == release.Digest && ready(target) {
		return target, nil
	}
	stage, err := os.MkdirTemp(parent, ".update-*")
	if err != nil {
		return Runtime{}, err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, release.Asset)
	if err = fetch(ctx, release.URL, archive, release.Digest, false); err != nil {
		return Runtime{}, err
	}
	unpack := filepath.Join(stage, "unpacked")
	if err = extract(archive, unpack); err != nil {
		return Runtime{}, err
	}
	if err = ctx.Err(); err != nil {
		return Runtime{}, err
	}
	payload := unpack
	if !file(filepath.Join(payload, exe("horizon-client"))) {
		payload = filepath.Join(unpack, strings.TrimSuffix(strings.TrimSuffix(release.Asset, ".tar.gz"), ".zip"))
	}
	if !file(filepath.Join(payload, exe("horizon-client"))) || !file(filepath.Join(payload, exe("horizon-prepare"))) {
		return Runtime{}, fmt.Errorf("paquete Horizon sin cliente o preparador")
	}
	if err = checkRuntimeCompatibility(payload); err != nil {
		return Runtime{}, err
	}
	// Never overwrite the active installation, even if it has become damaged.
	if active := readUpdateState(gameDir).Active; active == release.Version {
		return Runtime{}, fmt.Errorf("el runtime activo necesita reparación; se conserva para evitar sobrescribirlo")
	}
	if _, err = os.Stat(target.Root); err == nil {
		if err = os.RemoveAll(target.Root); err != nil {
			return Runtime{}, err
		}
	}
	if err = os.Rename(payload, target.Root); err != nil {
		return Runtime{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(target.Root)
		}
	}()
	emit("Preparando recursos de Minecraft " + Minecraft + " para el nuevo motor…")
	if err = prepareFn(ctx, target, gameDir, emit); err != nil {
		return Runtime{}, err
	}
	if err = ctx.Err(); err != nil {
		return Runtime{}, err
	}
	if !ready(target) {
		return Runtime{}, fmt.Errorf("Horizon no generó su manifiesto de recursos")
	}
	if err = os.WriteFile(marker, []byte(release.Digest), 0600); err != nil {
		return Runtime{}, err
	}
	complete = true
	return target, nil
}
