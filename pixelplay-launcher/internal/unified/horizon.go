package unified

import (
	"context"
	"pixelplay-launcher/internal/horizon"
	"sort"
	"strings"
)

func horizonSearch(p SearchParams) (SearchResponse, error) {
	mods, err := horizon.RegistryMods(context.Background())
	if err != nil {
		return SearchResponse{}, err
	}
	return horizonSearchResults(p, mods), nil
}
func horizonSearchResults(p SearchParams, mods []horizon.RegistryProject) SearchResponse {
	rows := []SearchResult{}
	if (strings.EqualFold(p.ProjectType, "mod") || strings.EqualFold(p.ProjectType, "resourcepack")) && (p.GameVersion == "" || p.GameVersion == "26.3") {
		for _, m := range mods {
			typ := "mod"
			if m.Kind == "resourcepack" {
				typ = "resourcepack"
			}
			if !strings.EqualFold(p.ProjectType, typ) {
				continue
			}
			if len(m.Versions) == 0 || !strings.Contains(strings.ToLower(m.Name+" "+m.ID+" "+m.Summary), strings.ToLower(strings.TrimSpace(p.Query))) {
				continue
			}
			latest := m.Versions[0].Version
			description := m.Summary
			if m.Kind == "native-mod" {
				description = "Requiere compilación nativa externa (.hnmod). " + description
			}
			rows = append(rows, SearchResult{ProjectID: m.ID, Source: "Horizon", Title: m.Name, Slug: m.ID, Description: description, Author: strings.Join(m.Authors, ", "), Downloads: m.Downloads, IconURL: &m.Icon, Categories: []string{"horizon"}, DisplayCategories: []string{"horizon"}, Gallery: []string{}, Versions: []string{"26.3"}, ProjectType: &typ, LatestVersion: &latest, ProjectURL: "https://playhorizon.dev/mods/" + m.ID})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	total := len(rows)
	offset, limit := 0, 20
	if p.Offset != nil {
		offset = max(0, *p.Offset)
	}
	if p.Limit != nil {
		limit = max(1, min(100, *p.Limit))
	}
	offset = min(offset, total)
	end := min(total, offset+limit)
	return SearchResponse{Results: rows[offset:end], Pagination: Pagination{Index: uint32(offset), PageSize: uint32(limit), ResultCount: uint32(end - offset), TotalCount: uint64(total)}}
}
func horizonVersions(p VersionsParams) (VersionResponse, error) {
	mods, err := horizon.RegistryMods(context.Background())
	if err != nil {
		return VersionResponse{}, err
	}
	versions := []Version{}
	for _, m := range mods {
		if m.ID != p.ProjectID || len(m.Versions) == 0 {
			continue
		}
		v := m.Versions[0]
		versions = append(versions, Version{ID: m.ID + "@" + v.Version, ProjectID: m.ID, Source: "Horizon", Name: m.Name + " " + v.Version, VersionNumber: v.Version, Dependencies: []Dependency{}, GameVersions: []string{v.Game}, Loaders: []string{"horizon"}, Files: []VersionFile{{Filename: horizon.InstalledFilename(m, v), URL: v.File.URL, Size: v.File.Size, Hashes: map[string]string{"sha256": v.File.SHA256}, Primary: true}}, ReleaseType: "release", URL: v.File.URL})
	}
	return VersionResponse{Versions: versions, TotalCount: len(versions)}, nil
}
