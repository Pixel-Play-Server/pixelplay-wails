package featured

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay.gg/ppack"
)

const (
	PackID          = "pixelplay-survival-1211"
	Address         = "mc.pixelplay.gg"
	Name            = "Pixel Play"
	NeoForgeVersion = "21.1.248"
	GameVersion     = "1.21.11"
	Loader          = "fabric"
	FabricVersion   = "0.19.5"
)

type Info struct {
	ProfileID string `json:"profileId"`
	Created   bool   `json:"created"`
	Address   string `json:"address"`
	PackID    string `json:"packId"`
	Name      string `json:"name"`
}

type remoteFeatured struct {
	Address       string `json:"address"`
	PackID        string `json:"packId"`
	Name          string `json:"name"`
	DownloadURL   string `json:"downloadUrl"`
	GameVersion   string `json:"gameVersion"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loaderVersion"`
}

func InfoDefaults() map[string]any {
	return map[string]any{
		"badge":         "Promocionado",
		"name":          Name,
		"address":       Address,
		"description":   "Survival oficial · Fabric 1.21.11",
		"packId":        PackID,
		"gameVersion":   GameVersion,
		"loader":        Loader,
		"loaderVersion": FabricVersion,
		"downloadUrl":   "https://api.pixelplay.gg/api/v1/launcher/packs/pixelplay-survival-1211.ppack",
	}
}

func FindExisting() (*profiles.Summary, error) {
	list, err := profiles.List()
	if err != nil {
		return nil, err
	}
	for i := range list {
		p := list[i]
		if p.SelectedPixelplayPackID != nil && *p.SelectedPixelplayPackID == PackID {
			return &p, nil
		}
	}
	return nil, nil
}

func Ensure() (*Info, error) {
	feat := fetchRemote()
	dest, err := ensurePackFile(feat)
	if err != nil {
		return nil, err
	}
	man, err := packs.ImportPPackFile(dest)
	if err != nil {
		return nil, fmt.Errorf("importar pack: %w", err)
	}
	applyManifest(&feat, man)

	if existing, err := FindExisting(); err != nil {
		return nil, err
	} else if existing != nil {
		updates := map[string]any{
			"name":           Name,
			"game_version":   feat.GameVersion,
			"loader":         feat.Loader,
			"loader_version": feat.LoaderVersion,
		}
		_ = profiles.Update(existing.ID, updates)
		// Drop leftover jars from previous loader (e.g. NeoForge JourneyMap on Fabric).
		// Protected packs store sealed mods under the vault, not the profile folder.
		modsDir := filepath.Join(paths.ProfileDir(existing.Path), "mods")
		if ok, _ := packs.IsProtectedByAuthor(feat.PackID); ok {
			modsDir = filepath.Join(paths.VaultDir(feat.PackID), "mods")
		}
		_ = packs.SyncPackMods(feat.PackID, feat.GameVersion, feat.Loader, modsDir, nil, nil)
		return &Info{ProfileID: existing.ID, Created: false, Address: feat.Address, PackID: feat.PackID, Name: Name}, nil
	}

	id, err := profiles.Create(map[string]any{
		"name":                       Name,
		"game_version":               feat.GameVersion,
		"loader":                     feat.Loader,
		"loader_version":             feat.LoaderVersion,
		"selected_pixelplay_pack_id": feat.PackID,
	})
	if err != nil {
		return nil, err
	}
	_ = profiles.Update(id, map[string]any{"group": "SERVER", "name": Name})
	return &Info{ProfileID: id, Created: true, Address: feat.Address, PackID: feat.PackID, Name: Name}, nil
}

// EnsureDefaultProfile creates/repairs the official Pixel Play pack profile if missing.
func EnsureDefaultProfile() (*Info, error) {
	return Ensure()
}

func ensurePackFile(feat remoteFeatured) (string, error) {
	destDir := filepath.Join(paths.DataDir(), "ppacks", feat.PackID)
	_ = os.MkdirAll(destDir, 0o755)
	dest := filepath.Join(destDir, feat.PackID+".ppack")
	needDownload := true
	if st, err := os.Stat(dest); err == nil && st.Size() > 1024 {
		if m, err := ppack.ReadManifestFromZip(dest); err == nil {
			if strings.EqualFold(m.Loader, feat.Loader) && strings.TrimSpace(m.GameVersion) == feat.GameVersion {
				needDownload = false
			}
		}
	}
	if needDownload {
		if err := download.File(feat.DownloadURL, dest, nil); err != nil {
			alt := "https://panel.pixelplay.gg/dl/pixelplay/modpacks/" + feat.PackID + "/" + feat.PackID + "-1.0.0.ppack"
			if err2 := download.File(alt, dest, nil); err2 != nil {
				alt2 := "https://panel.pixelplay.gg/dl/pixelplay/modpacks/" + feat.PackID + "/" + feat.PackID + ".ppack"
				if err3 := download.File(alt2, dest, nil); err3 != nil {
					return "", fmt.Errorf("descargar pack: %w", err)
				}
			}
		}
	}
	return dest, nil
}

func applyManifest(feat *remoteFeatured, man *ppack.Manifest) {
	if man == nil {
		return
	}
	if v := strings.TrimSpace(man.GameVersion); v != "" {
		feat.GameVersion = v
	}
	if v := strings.TrimSpace(man.Loader); v != "" {
		feat.Loader = strings.ToLower(v)
	}
	if feat.LoaderVersion == "" {
		if strings.EqualFold(feat.Loader, "fabric") {
			feat.LoaderVersion = FabricVersion
		} else if strings.EqualFold(feat.Loader, "neoforge") {
			feat.LoaderVersion = NeoForgeVersion
		}
	}
}

func fetchRemote() remoteFeatured {
	out := remoteFeatured{
		Address:       Address,
		PackID:        PackID,
		Name:          Name,
		DownloadURL:   "https://api.pixelplay.gg/api/v1/launcher/packs/pixelplay-survival-1211.ppack",
		GameVersion:   GameVersion,
		Loader:        Loader,
		LoaderVersion: FabricVersion,
	}
	b, err := httpx.Get("https://api.pixelplay.gg/api/v1/launcher/featured")
	if err != nil {
		return out
	}
	var parsed remoteFeatured
	if json.Unmarshal(b, &parsed) == nil {
		if parsed.Address != "" {
			out.Address = parsed.Address
		}
		if parsed.PackID != "" {
			out.PackID = parsed.PackID
		}
		if parsed.Name != "" {
			out.Name = parsed.Name
		}
		if parsed.DownloadURL != "" {
			out.DownloadURL = parsed.DownloadURL
		}
		if parsed.GameVersion != "" {
			out.GameVersion = parsed.GameVersion
		}
		if parsed.Loader != "" {
			out.Loader = parsed.Loader
		}
		if parsed.LoaderVersion != "" {
			out.LoaderVersion = parsed.LoaderVersion
		}
	}
	return out
}
