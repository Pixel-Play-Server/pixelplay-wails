package horizon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RegistryURL = "https://horizon-rust.github.io/horizon-mods/index.json"

type RegistryFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type RegistryVersion struct {
	Version string       `json:"version"`
	Game    string       `json:"game"`
	File    RegistryFile `json:"file"`
}
type RegistryProject struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Summary   string            `json:"summary"`
	Icon      string            `json:"icon"`
	Authors   []string          `json:"authors"`
	Downloads uint64            `json:"downloads"`
	Versions  []RegistryVersion `json:"versions"`
}
type registryIndex struct {
	Schema   int               `json:"schema"`
	Projects []RegistryProject `json:"projects"`
	Revoked  []struct {
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
	} `json:"revoked"`
}

func fetchRegistry(ctx context.Context) (registryIndex, error) {
	var idx registryIndex
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", RegistryURL, nil)
	if err != nil {
		return idx, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return idx, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return idx, fmt.Errorf("Horizon Registry: HTTP %d", res.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&idx)
	if err != nil {
		return idx, err
	}
	if idx.Schema != 1 {
		return idx, fmt.Errorf("versión de índice Horizon incompatible: %d", idx.Schema)
	}
	return idx, nil
}
func RegistryMods(ctx context.Context) ([]RegistryProject, error) {
	idx, err := fetchRegistry(ctx)
	if err != nil {
		return nil, err
	}
	mods := []RegistryProject{}
	for _, p := range idx.Projects {
		if len(p.Versions) == 0 {
			continue
		}
		v := p.Versions[0]
		if !registryCompatible(p.Kind, v) {
			continue
		}
		revoked := false
		for _, r := range idx.Revoked {
			if r.ID == p.ID && (r.SHA256 == "" || r.SHA256 == v.File.SHA256) {
				revoked = true
			}
		}
		if !revoked {
			mods = append(mods, p)
		}
	}
	return mods, nil
}
func InstallRegistryMod(ctx context.Context, gameDir, id string) (string, error) {
	return InstallRegistryContent(ctx, gameDir, id, "")
}

func InstallRegistryContent(ctx context.Context, gameDir, id, versionID string) (string, error) {
	idx, err := fetchRegistry(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range idx.Projects {
		if p.ID != id || len(p.Versions) == 0 {
			continue
		}
		v := p.Versions[0]
		f := v.File
		if versionID != "" && versionID != id+"@"+v.Version {
			return "", fmt.Errorf("la versión del catálogo cambió; actualiza la lista")
		}
		if !safeModID.MatchString(id) || !registryCompatible(p.Kind, v) || f.Size <= 0 || f.Size > 64<<20 || len(f.SHA256) != 64 {
			return "", fmt.Errorf("mod no compatible con Horizon 26.3")
		}
		for _, r := range idx.Revoked {
			if r.ID == id && (r.SHA256 == "" || r.SHA256 == f.SHA256) {
				return "", fmt.Errorf("mod revocado")
			}
		}
		if p.Kind == "native-mod" {
			return "", fmt.Errorf("%s requiere compilar Horizon con sus mixins nativos (.hnmod). Abre playhorizon.dev/mods/%s para consultar el proyecto; el compilador nativo aún no está integrado en Pixel Play", p.Name, id)
		}
		u, e := url.Parse(f.URL)
		if e != nil || u.Scheme != "https" || u.User != nil || u.Hostname() != "github.com" || !strings.Contains(u.Path, "/releases/download/") {
			return "", fmt.Errorf("origen del mod no autorizado")
		}
		req, e := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
		if e != nil {
			return "", e
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			return "", e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return "", fmt.Errorf("descarga de mod HTTP %d", res.StatusCode)
		}
		tmp, e := os.CreateTemp("", "pixelplay-registry-*"+filepath.Ext(f.Name))
		if e != nil {
			return "", e
		}
		defer os.Remove(tmp.Name())
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(res.Body, f.Size+1))
		closeErr := tmp.Close()
		if e != nil {
			return "", e
		}
		if closeErr != nil {
			return "", closeErr
		}
		if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			return "", fmt.Errorf("SHA-256 o tamaño del mod incorrecto")
		}
		switch p.Kind {
		case "native-mod":
			return "", fmt.Errorf("%s requiere compilar Horizon con sus mixins nativos (.hnmod). Pixel Play todavía no incorpora ese compilador; no se ha instalado ni activado el mod", p.Name)
		case "resourcepack":
			return installRegistryPack(gameDir, tmp.Name(), id, v.Version)
		default:
			return InstallMod(filepath.Clean(gameDir), tmp.Name())
		}
	}
	return "", fmt.Errorf("mod no encontrado en Horizon Registry")
}

func registryCompatible(kind string, v RegistryVersion) bool {
	if v.Game != "26.3" {
		return false
	}
	switch kind {
	case "mod":
		return strings.HasSuffix(v.File.Name, ".hmod")
	case "native-mod":
		return strings.HasSuffix(v.File.Name, ".hnmod")
	case "resourcepack":
		return strings.HasSuffix(v.File.Name, ".zip")
	}
	return false
}

func InstalledFilename(p RegistryProject, v RegistryVersion) string {
	if p.Kind == "resourcepack" {
		return p.ID + "-" + v.Version + ".zip"
	}
	if p.Kind == "native-mod" {
		return p.ID + ".hnmod"
	}
	return p.ID + ".hmod"
}
