package featured

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
)

const (
	PackID  = "pixelplay-survival-1211"
	Address = "mc.pixelplay.gg"
	Name    = "Pixel Play"
)

type Info struct {
	ProfileID string `json:"profileId"`
	Created   bool   `json:"created"`
	Address   string `json:"address"`
	PackID    string `json:"packId"`
	Name      string `json:"name"`
}

type remoteFeatured struct {
	Address     string `json:"address"`
	PackID      string `json:"packId"`
	Name        string `json:"name"`
	DownloadURL string `json:"downloadUrl"`
	GameVersion string `json:"gameVersion"`
	Loader      string `json:"loader"`
}

func InfoDefaults() map[string]any {
	return map[string]any{
		"badge":       "Promocionado",
		"name":        Name,
		"address":     Address,
		"description": "Survival oficial NeoForge 1.21.1",
		"packId":      PackID,
		"gameVersion": "1.21.1",
		"loader":      "neoforge",
		"downloadUrl": "https://api.pixelplay.gg/api/v1/launcher/packs/pixelplay-survival-1211.ppack",
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
	for i := range list {
		p := list[i]
		n := strings.ToLower(strings.TrimSpace(p.Name))
		if n == "pixel" || n == "pixel play" || strings.Contains(n, "pixel play") {
			if p.GameVersion == "1.21.1" {
				return &p, nil
			}
		}
	}
	return nil, nil
}

func Ensure() (*Info, error) {
	if existing, err := FindExisting(); err != nil {
		return nil, err
	} else if existing != nil {
		return &Info{ProfileID: existing.ID, Created: false, Address: Address, PackID: PackID, Name: existing.Name}, nil
	}

	feat := fetchRemote()
	destDir := filepath.Join(paths.DataDir(), "ppacks", feat.PackID)
	_ = os.MkdirAll(destDir, 0o755)
	dest := filepath.Join(destDir, feat.PackID+".ppack")
	if err := downloadFile(feat.DownloadURL, dest); err != nil {
		return nil, fmt.Errorf("descargar pack: %w", err)
	}
	if _, err := packs.ImportPPackFile(dest); err != nil {
		return nil, fmt.Errorf("importar pack: %w", err)
	}
	id, err := profiles.Create(map[string]any{
		"name":                       feat.Name,
		"game_version":               feat.GameVersion,
		"loader":                     feat.Loader,
		"selected_pixelplay_pack_id": feat.PackID,
	})
	if err != nil {
		return nil, err
	}
	_ = profiles.Update(id, map[string]any{"group": "SERVER"})
	return &Info{ProfileID: id, Created: true, Address: feat.Address, PackID: feat.PackID, Name: feat.Name}, nil
}

func fetchRemote() remoteFeatured {
	out := remoteFeatured{
		Address:     Address,
		PackID:      PackID,
		Name:        Name,
		DownloadURL: "https://api.pixelplay.gg/api/v1/launcher/packs/pixelplay-survival-1211.ppack",
		GameVersion: "1.21.1",
		Loader:      "neoforge",
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
	}
	return out
}

func downloadFile(rawURL, dest string) error {
	b, err := httpx.Get(rawURL)
	if err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
