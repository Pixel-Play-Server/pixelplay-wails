// Package horizon installs and prepares the native game without Horizon Launcher.
package horizon

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"pixelplay-launcher/internal/java"
	"pixelplay-launcher/internal/mcmeta"
	"pixelplay-launcher/internal/paths"
)

const Version = "v0.2.97"
const Minecraft = "26.3"

type Runtime struct{ Root, Binary, Resources string }

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
func at(root string) Runtime {
	return Runtime{root, filepath.Join(root, exe("horizon-client")), filepath.Join(root, ".local", "horizon-client", Minecraft)}
}
func file(path string) bool { s, e := os.Stat(path); return e == nil && s.Mode().IsRegular() }
func ready(r Runtime) bool {
	return file(r.Binary) && file(filepath.Join(r.Resources, "horizon-resources.json"))
}

// Find preserves prepared installations made by the earlier bridge. It never reads accounts.
func Find(gameDir string) (Runtime, bool) {
	var roots []string
	for _, parent := range []string{filepath.Join(gameDir, ".horizon-native"), filepath.Join(gameDir, ".horizon-launcher", "versions")} {
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if e.IsDir() {
				roots = append(roots, filepath.Join(parent, e.Name()))
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		a, _ := os.Stat(roots[i])
		b, _ := os.Stat(roots[j])
		return a.ModTime().After(b.ModTime())
	})
	for _, root := range roots {
		r := at(root)
		if ready(r) {
			return r, true
		}
	}
	return Runtime{}, false
}

func Ensure(ctx context.Context, gameDir string, emit func(string)) (Runtime, error) {
	asset, digest, err := releaseAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Runtime{}, err
	}
	if r, ok := Find(gameDir); ok {
		return r, nil
	}
	r := at(filepath.Join(gameDir, ".horizon-native", Version))
	if !file(r.Binary) {
		emit("Descargando motor Horizon " + Version + "…")
		parent := filepath.Dir(r.Root)
		if err := os.MkdirAll(parent, 0700); err != nil {
			return r, err
		}
		archive := filepath.Join(parent, asset)
		if err := fetch(ctx, "https://github.com/horizon-rust/horizon/releases/download/"+Version+"/"+asset, archive, digest, false); err != nil {
			return r, err
		}
		stage, err := os.MkdirTemp(parent, ".install-")
		if err != nil {
			return r, err
		}
		defer os.RemoveAll(stage)
		if err = extract(archive, stage); err != nil {
			return r, err
		}
		payload := stage
		if !file(filepath.Join(payload, exe("horizon-client"))) {
			folder := strings.TrimSuffix(strings.TrimSuffix(asset, ".tar.gz"), ".zip")
			payload = filepath.Join(stage, folder)
		}
		if !file(filepath.Join(payload, exe("horizon-client"))) {
			return r, fmt.Errorf("paquete Horizon sin ejecutable")
		}
		if err = os.Rename(payload, r.Root); err != nil {
			return r, err
		}
		_ = os.Remove(archive)
	}
	emit("Preparando recursos de Minecraft " + Minecraft + " para Horizon…")
	if err := prepare(ctx, r, gameDir, emit); err != nil {
		return r, err
	}
	if !ready(r) {
		return r, fmt.Errorf("Horizon no generó su manifiesto de recursos")
	}
	return r, nil
}

func prepare(ctx context.Context, r Runtime, gameDir string, emit func(string)) error {
	// Reuse this computer's already downloaded Minecraft; never distribute its resources.
	mc := filepath.Join(gameDir, ".horizon-launcher", "minecraft")
	if !file(filepath.Join(mc, "versions", Minecraft, Minecraft+".json")) {
		mc = filepath.Join(paths.DataDir(), "meta")
		if err := mcmeta.EnsureVanillaLauncherLayout(Minecraft, mcmeta.Progress(emit)); err != nil {
			return err
		}
		meta, err := mcmeta.FetchPiston(Minecraft)
		if err != nil {
			return err
		}
		if err = mcmeta.DownloadAssets(meta, mcmeta.Progress(emit)); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(mc, "versions", Minecraft, Minecraft+".json"))
	if err != nil {
		return err
	}
	var meta struct {
		Downloads struct {
			Server mcmeta.Artifact `json:"server"`
		} `json:"downloads"`
	}
	if err = json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	if meta.Downloads.Server.URL == "" || len(meta.Downloads.Server.SHA1) != 40 {
		return fmt.Errorf("metadatos sin servidor Minecraft verificado")
	}
	server := filepath.Join(r.Root, ".local", "server.jar")
	if err = fetch(ctx, meta.Downloads.Server.URL, server, meta.Downloads.Server.SHA1, true); err != nil {
		return err
	}
	j, err := java.EnsureExact(25, emit)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(gameDir, "logs"), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(gameDir, "logs", "horizon-prepare.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	for _, args := range [][]string{{"--prepare", "--minecraft-dir", mc, "--server-jar", server, "--root", r.Root}, {"sounds", "--minecraft-dir", mc, "--root", r.Root}} {
		cmd := exec.CommandContext(ctx, filepath.Join(r.Root, exe("horizon-prepare")), args...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "JAVA_HOME="+filepath.Dir(filepath.Dir(j)), "PATH="+filepath.Dir(j)+string(os.PathListSeparator)+os.Getenv("PATH"))
		cmd.Stdout, cmd.Stderr = log, log
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("preparar Horizon: %w; revisa logs/horizon-prepare.log", err)
		}
	}
	return nil
}

func fetch(ctx context.Context, url, dest, digest string, shaOne bool) error {
	hasher := func() hash.Hash {
		if shaOne {
			return sha1.New()
		}
		return sha256.New()
	}
	matches := func(path string) bool {
		f, e := os.Open(path)
		if e != nil {
			return false
		}
		defer f.Close()
		h := hasher()
		_, e = io.Copy(h, f)
		return e == nil && hex.EncodeToString(h.Sum(nil)) == digest
	}
	if matches(dest) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	res, err := (&http.Client{Timeout: 20 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("descarga Horizon: HTTP %d", res.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = io.Copy(tmp, io.LimitReader(res.Body, 2<<30))
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if !matches(name) {
		return fmt.Errorf("checksum incorrecto: %s", filepath.Base(dest))
	}
	if file(dest) {
		if err = os.Remove(dest); err != nil {
			return err
		}
	}
	return os.Rename(name, dest)
}

func safePath(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(name))
	if strings.Contains(name, ":") || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ruta de archivo no permitida: %q", name)
	}
	return filepath.Join(root, clean), nil
}
func extract(archive, root string) error {
	var total int64
	write := func(name string, size int64, r io.Reader) error {
		total += size
		if size < 0 || total > 2<<30 {
			return fmt.Errorf("paquete Horizon demasiado grande")
		}
		dest, err := safePath(root, name)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
		_, err = io.CopyN(f, r, size)
		ce := f.Close()
		if err != nil {
			return err
		}
		return ce
	}
	if strings.HasSuffix(archive, ".zip") {
		z, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if _, err = safePath(root, f.Name); err != nil {
				return err
			}
			if f.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("enlace no permitido en paquete")
			}
			if f.FileInfo().IsDir() {
				continue
			}
			reader, err := f.Open()
			if err != nil {
				return err
			}
			err = write(f.Name, int64(f.UncompressedSize64), reader)
			reader.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err = safePath(root, h.Name); err != nil {
			return err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("tipo de archivo no permitido en paquete")
		}
		if err = write(h.Name, h.Size, tr); err != nil {
			return err
		}
	}
}

func SupportedPlatform() bool {
	_, _, err := releaseAsset(runtime.GOOS, runtime.GOARCH)
	return err == nil
}
func releaseAsset(osName, arch string) (string, string, error) {
	switch osName + "/" + arch {
	case "linux/amd64":
		return "horizon-linux-x86_64.tar.gz", "16e62f7a9ac95d6be788e92cd5fb50a569926d2378531a4e0ea3a07337b79c11", nil
	case "windows/amd64":
		return "horizon-windows-x64.zip", "29d3cef224aecdafea314cf6a3acb5caf8228edef9a3c5d73a0b4fe3b2696fca", nil
	case "darwin/arm64":
		return "horizon-macos-arm64.zip", "587fb8a653324e6f1b3bf59f1f2227ad99201ccaaf3f31024db25ccd4f37c63e", nil
	}
	return "", "", fmt.Errorf("Horizon está disponible en Linux/Windows x86_64 y macOS Apple Silicon; no existe un runtime oficial para macOS Intel en esta versión")
}
