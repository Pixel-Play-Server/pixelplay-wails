package mcmeta

import (
	"fmt"
	"strings"

	"pixelplay-launcher/internal/httpx"
)

func InstallQuilt(mcVersion, requested string, emit Progress) (*FabricProfile, []Library, error) {
	if emit != nil {
		emit("Instalando Quilt…")
	}
	loader := strings.TrimSpace(strings.TrimSuffix(requested, " (stable)"))
	loader = strings.TrimPrefix(loader, "quilt-")
	if loader == "" {
		var loaders []struct {
			Loader struct {
				Version string `json:"version"`
				Stable  bool   `json:"stable"`
			} `json:"loader"`
		}
		url := "https://meta.quiltmc.org/v3/versions/loader/" + mcVersion
		if err := httpx.GetJSON(url, &loaders); err != nil {
			return nil, nil, err
		}
		for _, l := range loaders {
			if l.Loader.Stable {
				loader = l.Loader.Version
				break
			}
		}
		if loader == "" && len(loaders) > 0 {
			loader = loaders[0].Loader.Version
		}
		if loader == "" {
			return nil, nil, fmt.Errorf("no hay Quilt para %s", mcVersion)
		}
	}
	url := fmt.Sprintf("https://meta.quiltmc.org/v3/versions/loader/%s/%s/profile/json", mcVersion, loader)
	var prof FabricProfile
	if err := httpx.GetJSON(url, &prof); err != nil {
		return nil, nil, err
	}
	for i := range prof.Libraries {
		if prof.Libraries[i].URL == "" {
			prof.Libraries[i].URL = "https://maven.quiltmc.org/repository/release/"
		}
	}
	if emit != nil {
		emit("Quilt " + loader)
	}
	return &prof, prof.Libraries, nil
}
