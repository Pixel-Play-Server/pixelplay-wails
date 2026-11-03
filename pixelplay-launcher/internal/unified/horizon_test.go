package unified

import (
	"pixelplay-launcher/internal/horizon"
	"testing"
)

func TestHorizonUsesUnifiedSearch(t *testing.T) {
	mods := []horizon.RegistryProject{
		{ID: "zoom", Name: "Zoom", Versions: []horizon.RegistryVersion{{Version: "1.0"}}},
		{ID: "compass", Name: "Compass", Summary: "HUD", Versions: []horizon.RegistryVersion{{Version: "1.1"}}},
	}
	limit := 1
	p := SearchParams{ProjectType: "mod", GameVersion: "26.3", Limit: &limit}
	r := horizonSearchResults(p, mods)
	if r.Pagination.TotalCount != 2 || len(r.Results) != 1 || r.Results[0].ProjectID != "compass" || r.Results[0].Source != "Horizon" {
		t.Fatalf("bad unified results: %+v", r)
	}
	p.Query = "HUD"
	if r = horizonSearchResults(p, mods); r.Pagination.TotalCount != 1 {
		t.Fatal("query not filtered")
	}
	p.ProjectType = "shader"
	if r = horizonSearchResults(p, mods); r.Pagination.TotalCount != 0 {
		t.Fatal("mods exposed as shaders")
	}
	p.ProjectType = "mod"
	p.GameVersion = "1.21.11"
	if r = horizonSearchResults(p, mods); r.Pagination.TotalCount != 0 {
		t.Fatal("incompatible game version accepted")
	}
}

func TestHorizonNativeAndPacksCatalog(t *testing.T) {
	projects := []horizon.RegistryProject{
		{ID: "zoom", Kind: "native-mod", Name: "Zoom", Versions: []horizon.RegistryVersion{{Version: "2"}}},
		{ID: "panorama", Kind: "resourcepack", Name: "Panorama", Versions: []horizon.RegistryVersion{{Version: "1"}}},
	}
	mods := horizonSearchResults(SearchParams{ProjectType: "mod"}, projects)
	packs := horizonSearchResults(SearchParams{ProjectType: "resourcepack"}, projects)
	if len(mods.Results) != 1 || len(packs.Results) != 1 || packs.Results[0].ProjectID != "panorama" {
		t.Fatal("catalog types mixed")
	}
	if mods.Results[0].ProjectURL != "https://playhorizon.dev/mods/zoom" {
		t.Fatal("incorrect source link")
	}
}
