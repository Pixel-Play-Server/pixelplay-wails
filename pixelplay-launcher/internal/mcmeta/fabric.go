package mcmeta

import (
	"fmt"
	"strings"

	"pixelplay-launcher/internal/httpx"
)

type FabricProfile struct {
	ID        string    `json:"id"`
	MainClass string    `json:"mainClass"`
	Libraries []Library `json:"libraries"`
	Arguments *Arguments `json:"arguments"`
}

func InstallFabric(mcVersion, requested string, emit Progress) (*FabricProfile, []Library, error) {
	if emit != nil {
		emit("Instalando Fabric…")
	}
	loader := strings.TrimSpace(strings.TrimSuffix(requested, " (stable)"))
	if loader == "" {
		var loaders []struct {
			Loader struct {
				Version string `json:"version"`
				Stable  bool   `json:"stable"`
			} `json:"loader"`
		}
		url := "https://meta.fabricmc.net/v2/versions/loader/" + mcVersion
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
			return nil, nil, fmt.Errorf("no hay Fabric para %s", mcVersion)
		}
	}
	url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/profile/json", mcVersion, loader)
	var prof FabricProfile
	if err := httpx.GetJSON(url, &prof); err != nil {
		return nil, nil, err
	}
	for i := range prof.Libraries {
		if prof.Libraries[i].URL == "" {
			prof.Libraries[i].URL = "https://maven.fabricmc.net/"
		}
	}
	if emit != nil {
		emit("Fabric " + loader)
	}
	return &prof, prof.Libraries, nil
}
