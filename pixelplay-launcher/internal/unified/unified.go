package unified

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"pixelplay-launcher/internal/curseforge"
	"pixelplay-launcher/internal/modrinth"
)

type SearchParams struct {
	Query            string   `json:"query"`
	Source           string   `json:"source"`
	ProjectType      string   `json:"project_type"`
	GameVersion      string   `json:"game_version"`
	Categories       []string `json:"categories"`
	ModLoaders       []string `json:"mod_loaders"`
	Limit            *int     `json:"limit"`
	Offset           *int     `json:"offset"`
	Sort             string   `json:"sort"`
	ClientSideFilter string   `json:"client_side_filter"`
	ServerSideFilter string   `json:"server_side_filter"`
}

type SearchResult struct {
	ProjectID         string   `json:"project_id"`
	Source            string   `json:"source"`
	Title             string   `json:"title"`
	Slug              string   `json:"slug"`
	Description       string   `json:"description"`
	Author            string   `json:"author"`
	Categories        []string `json:"categories"`
	DisplayCategories []string `json:"display_categories"`
	ClientSide        *string  `json:"client_side"`
	ServerSide        *string  `json:"server_side"`
	Downloads         uint64   `json:"downloads"`
	Follows           *uint64  `json:"follows"`
	IconURL           *string  `json:"icon_url"`
	ProjectURL        string   `json:"project_url"`
	ProjectType       *string  `json:"project_type"`
	LatestVersion     *string  `json:"latest_version"`
	DateCreated       *string  `json:"date_created"`
	DateModified      *string  `json:"date_modified"`
	License           *string  `json:"license"`
	Gallery           []string `json:"gallery"`
	Versions          []string `json:"versions"`
}

type Pagination struct {
	Index       uint32 `json:"index"`
	PageSize    uint32 `json:"page_size"`
	ResultCount uint32 `json:"result_count"`
	TotalCount  uint64 `json:"total_count"`
}

type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Pagination Pagination     `json:"pagination"`
}

type VersionsParams struct {
	Source       string   `json:"source"`
	ProjectID    string   `json:"project_id"`
	Loaders      []string `json:"loaders"`
	GameVersions []string `json:"game_versions"`
	Limit        *int     `json:"limit"`
	Offset       *int     `json:"offset"`
}

type VersionFile struct {
	Filename    string            `json:"filename"`
	URL         string            `json:"url"`
	Size        int64             `json:"size"`
	Hashes      map[string]string `json:"hashes"`
	Primary     bool              `json:"primary"`
	Fingerprint *int64            `json:"fingerprint,omitempty"`
}

type Dependency struct {
	ProjectID      *string `json:"project_id"`
	VersionID      *string `json:"version_id"`
	FileName       *string `json:"file_name"`
	DependencyType string  `json:"dependency_type"`
}

type Version struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	Source        string        `json:"source"`
	Name          string        `json:"name"`
	VersionNumber string        `json:"version_number"`
	Changelog     *string       `json:"changelog"`
	Dependencies  []Dependency  `json:"dependencies"`
	GameVersions  []string      `json:"game_versions"`
	Loaders       []string      `json:"loaders"`
	Files         []VersionFile `json:"files"`
	DatePublished string        `json:"date_published"`
	Downloads     uint64        `json:"downloads"`
	ReleaseType   string        `json:"release_type"`
	URL           string        `json:"url"`
}

type VersionResponse struct {
	Versions   []Version `json:"versions"`
	TotalCount int       `json:"total_count"`
}

type ModpackVersionsResponse struct {
	InstalledVersion *Version  `json:"installed_version"`
	AllVersions      []Version `json:"all_versions"`
	UpdatesAvailable *bool     `json:"updates_available"`
}

func Search(raw json.RawMessage) (SearchResponse, error) {
	var p SearchParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return SearchResponse{}, fmt.Errorf("params de búsqueda: %w", err)
	}
	if strings.EqualFold(p.Source, "Horizon") {
		return horizonSearch(p)
	}
	limit := 20
	offset := 0
	if p.Limit != nil && *p.Limit > 0 {
		limit = *p.Limit
	}
	if p.Offset != nil && *p.Offset >= 0 {
		offset = *p.Offset
	}
	pt := projectTypeSlug(p.ProjectType)
	src := strings.ToLower(p.Source)

	if src == "curseforge" || src == "curse_forge" {
		return searchCurseForge(p, pt, limit, offset)
	}
	return searchModrinth(p, pt, limit, offset)
}

func searchModrinth(p SearchParams, pt string, limit, offset int) (SearchResponse, error) {
	loader := ""
	if len(p.ModLoaders) > 0 {
		loader = p.ModLoaders[0]
	}
	res, err := modrinth.SearchProjects(modrinth.SearchParams{
		Query:            p.Query,
		ProjectType:      pt,
		GameVersion:      p.GameVersion,
		Loader:           loader,
		Limit:            limit,
		Offset:           offset,
		Sort:             p.Sort,
		Categories:       p.Categories,
		ClientSideFilter: p.ClientSideFilter,
		ServerSideFilter: p.ServerSideFilter,
	})
	if err != nil {
		return SearchResponse{}, err
	}
	out := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		mapped, err := mapModrinthHit(hit)
		if err != nil {
			continue
		}
		out = append(out, mapped)
	}
	return SearchResponse{
		Results: out,
		Pagination: Pagination{
			Index:       uint32(offset),
			PageSize:    uint32(limit),
			ResultCount: uint32(len(out)),
			TotalCount:  uint64(res.TotalHits),
		},
	}, nil
}

func searchCurseForge(p SearchParams, pt string, limit, offset int) (SearchResponse, error) {
	loaders := make([]int, 0, len(p.ModLoaders))
	for _, l := range p.ModLoaders {
		if id := curseforge.LoaderID(l); id > 0 {
			loaders = append(loaders, id)
		}
	}
	sortField, sortOrder := curseforge.SortField(p.Sort)
	res, err := curseforge.Search(curseforge.SearchOpts{
		Query:       p.Query,
		ClassID:     curseforge.ClassID(pt),
		GameVersion: p.GameVersion,
		Loaders:     loaders,
		SortField:   sortField,
		SortOrder:   sortOrder,
		Index:       offset,
		PageSize:    limit,
	})
	if err != nil {
		return SearchResponse{}, err
	}
	out := make([]SearchResult, 0, len(res.Data))
	for _, m := range res.Data {
		out = append(out, mapCurseForgeMod(m, pt))
	}
	return SearchResponse{
		Results: out,
		Pagination: Pagination{
			Index:       uint32(offset),
			PageSize:    uint32(limit),
			ResultCount: uint32(len(out)),
			TotalCount:  uint64(res.Pagination.TotalCount),
		},
	}, nil
}

func Versions(raw json.RawMessage) (VersionResponse, error) {
	var p VersionsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return VersionResponse{}, fmt.Errorf("params de versiones: %w", err)
	}
	if strings.EqualFold(p.Source, "Horizon") {
		return horizonVersions(p)
	}
	if strings.TrimSpace(p.ProjectID) == "" {
		return VersionResponse{}, fmt.Errorf("falta project_id")
	}
	src := strings.ToLower(p.Source)
	if src == "curseforge" || src == "curse_forge" {
		return versionsCurseForge(p)
	}
	return versionsModrinth(p)
}

func versionsModrinth(p VersionsParams) (VersionResponse, error) {
	raw, err := modrinth.ProjectVersions(p.ProjectID, p.Loaders, p.GameVersions)
	if err != nil {
		return VersionResponse{}, err
	}
	var list []modrinthVersion
	if err := json.Unmarshal(raw, &list); err != nil {
		return VersionResponse{}, err
	}
	out := make([]Version, 0, len(list))
	for _, v := range list {
		out = append(out, mapModrinthVersion(v))
	}
	return VersionResponse{Versions: out, TotalCount: len(out)}, nil
}

func versionsCurseForge(p VersionsParams) (VersionResponse, error) {
	gv := ""
	if len(p.GameVersions) > 0 {
		gv = p.GameVersions[0]
	}
	loader := 0
	if len(p.Loaders) > 0 {
		loader = curseforge.LoaderID(p.Loaders[0])
	}
	limit := 50
	offset := 0
	if p.Limit != nil && *p.Limit > 0 {
		limit = *p.Limit
	}
	if p.Offset != nil && *p.Offset >= 0 {
		offset = *p.Offset
	}
	res, err := curseforge.GetFiles(p.ProjectID, gv, loader, offset, limit)
	if err != nil {
		return VersionResponse{}, err
	}
	out := make([]Version, 0, len(res.Data))
	for _, f := range res.Data {
		out = append(out, mapCurseForgeFile(f))
	}
	return VersionResponse{Versions: out, TotalCount: len(out)}, nil
}

func ModpackVersions(raw json.RawMessage) (ModpackVersionsResponse, error) {
	var src struct {
		Source    string      `json:"source"`
		ProjectID json.Number `json:"project_id"`
		VersionID string      `json:"version_id"`
		FileID    json.Number `json:"file_id"`
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		return ModpackVersionsResponse{}, err
	}
	projectID := src.ProjectID.String()
	platform := "Modrinth"
	if strings.EqualFold(src.Source, "curse_forge") || strings.EqualFold(src.Source, "curseforge") {
		platform = "CurseForge"
	}
	vers, err := Versions(mustJSON(VersionsParams{
		Source:    platform,
		ProjectID: projectID,
	}))
	if err != nil {
		return ModpackVersionsResponse{}, err
	}
	var installed *Version
	want := src.VersionID
	if want == "" {
		want = src.FileID.String()
	}
	if want != "" && want != "0" {
		for i := range vers.Versions {
			if vers.Versions[i].ID == want {
				cp := vers.Versions[i]
				installed = &cp
				break
			}
		}
	}
	updates := false
	if installed != nil && len(vers.Versions) > 0 && vers.Versions[0].ID != installed.ID {
		updates = true
	}
	return ModpackVersionsResponse{
		InstalledVersion: installed,
		AllVersions:      vers.Versions,
		UpdatesAvailable: &updates,
	}, nil
}

func CheckUpdates(raw json.RawMessage) (map[string]any, error) {
	mapped, err := modrinth.CheckUpdates(raw)
	if err != nil {
		return nil, err
	}
	var versions map[string]modrinthVersion
	if err := json.Unmarshal(mapped, &versions); err != nil {
		return map[string]any{"updates": map[string]any{}}, nil
	}
	out := map[string]Version{}
	for hash, v := range versions {
		out[hash] = mapModrinthVersion(v)
	}
	return map[string]any{"updates": out}, nil
}

type modrinthHit struct {
	ProjectID         string   `json:"project_id"`
	ProjectType       string   `json:"project_type"`
	Slug              string   `json:"slug"`
	Author            string   `json:"author"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Categories        []string `json:"categories"`
	DisplayCategories []string `json:"display_categories"`
	Downloads         uint64   `json:"downloads"`
	Follows           uint64   `json:"follows"`
	IconURL           *string  `json:"icon_url"`
	DateCreated       string   `json:"date_created"`
	DateModified      string   `json:"date_modified"`
	LatestVersion     *string  `json:"latest_version"`
	License           any      `json:"license"`
	ClientSide        string   `json:"client_side"`
	ServerSide        string   `json:"server_side"`
	Gallery           []string `json:"gallery"`
}

type modrinthVersion struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
	Name          string  `json:"name"`
	VersionNumber string  `json:"version_number"`
	Changelog     *string `json:"changelog"`
	Dependencies  []struct {
		ProjectID      *string `json:"project_id"`
		VersionID      *string `json:"version_id"`
		FileName       *string `json:"file_name"`
		DependencyType string  `json:"dependency_type"`
	} `json:"dependencies"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	DatePublished string   `json:"date_published"`
	Downloads     uint64   `json:"downloads"`
	VersionType   string   `json:"version_type"`
	Files         []struct {
		Filename string `json:"filename"`
		URL      string `json:"url"`
		Size     int64  `json:"size"`
		Primary  bool   `json:"primary"`
		Hashes   struct {
			SHA1   string `json:"sha1"`
			SHA512 string `json:"sha512"`
		} `json:"hashes"`
	} `json:"files"`
}

func mapModrinthHit(raw json.RawMessage) (SearchResult, error) {
	var h modrinthHit
	if err := json.Unmarshal(raw, &h); err != nil {
		return SearchResult{}, err
	}
	pt := h.ProjectType
	author := h.Author
	if author == "" {
		author = "Unknown"
	}
	cats := h.Categories
	if cats == nil {
		cats = []string{}
	}
	disp := h.DisplayCategories
	if disp == nil {
		disp = cats
	}
	gallery := h.Gallery
	if gallery == nil {
		gallery = []string{}
	}
	follows := h.Follows
	cs := h.ClientSide
	ss := h.ServerSide
	created := h.DateCreated
	modified := h.DateModified
	lic := licenseString(h.License)
	return SearchResult{
		ProjectID:         h.ProjectID,
		Source:            "Modrinth",
		Title:             h.Title,
		Slug:              h.Slug,
		Description:       h.Description,
		Author:            author,
		Categories:        cats,
		DisplayCategories: disp,
		ClientSide:        strPtr(cs),
		ServerSide:        strPtr(ss),
		Downloads:         h.Downloads,
		Follows:           &follows,
		IconURL:           h.IconURL,
		ProjectURL:        fmt.Sprintf("https://modrinth.com/%s/%s", pt, h.Slug),
		ProjectType:       strPtr(pt),
		LatestVersion:     h.LatestVersion,
		DateCreated:       strPtr(created),
		DateModified:      strPtr(modified),
		License:           strPtr(lic),
		Gallery:           gallery,
	}, nil
}

func mapCurseForgeMod(m curseforge.Mod, pt string) SearchResult {
	author := "Unknown"
	if len(m.Authors) > 0 && m.Authors[0].Name != "" {
		author = m.Authors[0].Name
	}
	cats := make([]string, 0, len(m.Categories))
	for _, c := range m.Categories {
		if c.Slug != "" {
			cats = append(cats, c.Slug)
		} else {
			cats = append(cats, c.Name)
		}
	}
	var icon *string
	if m.Logo != nil && m.Logo.URL != "" {
		u := m.Logo.URL
		icon = &u
	}
	url := m.Links.WebsiteURL
	if url == "" && m.Slug != "" {
		url = "https://www.curseforge.com/minecraft/mc-mods/" + m.Slug
	}
	created := m.DateCreated
	modified := m.DateModified
	return SearchResult{
		ProjectID:         strconv.FormatUint(m.ID, 10),
		Source:            "CurseForge",
		Title:             m.Name,
		Slug:              m.Slug,
		Description:       m.Summary,
		Author:            author,
		Categories:        cats,
		DisplayCategories: cats,
		Downloads:         m.DownloadCount,
		IconURL:           icon,
		ProjectURL:        url,
		ProjectType:       strPtr(pt),
		DateCreated:       strPtr(created),
		DateModified:      strPtr(modified),
		Gallery:           []string{},
	}
}

func mapModrinthVersion(v modrinthVersion) Version {
	files := make([]VersionFile, 0, len(v.Files))
	for _, f := range v.Files {
		hashes := map[string]string{}
		if f.Hashes.SHA1 != "" {
			hashes["sha1"] = f.Hashes.SHA1
		}
		if f.Hashes.SHA512 != "" {
			hashes["sha512"] = f.Hashes.SHA512
		}
		files = append(files, VersionFile{
			Filename: f.Filename,
			URL:      f.URL,
			Size:     f.Size,
			Hashes:   hashes,
			Primary:  f.Primary,
		})
	}
	deps := make([]Dependency, 0, len(v.Dependencies))
	for _, d := range v.Dependencies {
		deps = append(deps, Dependency{
			ProjectID:      d.ProjectID,
			VersionID:      d.VersionID,
			FileName:       d.FileName,
			DependencyType: strings.ToLower(d.DependencyType),
		})
	}
	rt := strings.ToLower(v.VersionType)
	if rt == "" {
		rt = "release"
	}
	return Version{
		ID:            v.ID,
		ProjectID:     v.ProjectID,
		Source:        "Modrinth",
		Name:          v.Name,
		VersionNumber: v.VersionNumber,
		Changelog:     v.Changelog,
		Dependencies:  deps,
		GameVersions:  v.GameVersions,
		Loaders:       v.Loaders,
		Files:         files,
		DatePublished: v.DatePublished,
		Downloads:     v.Downloads,
		ReleaseType:   rt,
		URL:           fmt.Sprintf("https://modrinth.com/mod/%s/version/%s", v.ProjectID, v.ID),
	}
}

func mapCurseForgeFile(f curseforge.File) Version {
	hashes := map[string]string{}
	for _, h := range f.Hashes {
		switch h.Algo {
		case 1:
			hashes["sha1"] = h.Value
		case 2:
			hashes["md5"] = h.Value
		}
	}
	loaders, versions := splitGameVersions(f.GameVersions)
	deps := make([]Dependency, 0, len(f.Dependencies))
	for _, d := range f.Dependencies {
		id := strconv.FormatUint(d.ModID, 10)
		deps = append(deps, Dependency{
			ProjectID:      &id,
			DependencyType: cfDepType(d.RelationType),
		})
	}
	rt := "release"
	switch f.ReleaseType {
	case 2:
		rt = "beta"
	case 3:
		rt = "alpha"
	}
	id := strconv.FormatUint(f.ID, 10)
	pid := strconv.FormatUint(f.ModID, 10)
	return Version{
		ID:            id,
		ProjectID:     pid,
		Source:        "CurseForge",
		Name:          f.DisplayName,
		VersionNumber: f.DisplayName,
		Dependencies:  deps,
		GameVersions:  versions,
		Loaders:       loaders,
		Files: []VersionFile{{
			Filename: f.FileName,
			URL:      f.DownloadURL,
			Size:     f.FileLength,
			Hashes:   hashes,
			Primary:  true,
		}},
		DatePublished: f.FileDate,
		Downloads:     f.DownloadCount,
		ReleaseType:   rt,
		URL:           f.DownloadURL,
	}
}

func splitGameVersions(in []string) (loaders, versions []string) {
	for _, v := range in {
		low := strings.ToLower(v)
		switch low {
		case "forge", "fabric", "quilt", "neoforge", "liteloader", "rift":
			loaders = append(loaders, low)
		default:
			versions = append(versions, v)
		}
	}
	if loaders == nil {
		loaders = []string{}
	}
	if versions == nil {
		versions = []string{}
	}
	return loaders, versions
}

func cfDepType(rel int) string {
	switch rel {
	case 3:
		return "required"
	case 2:
		return "optional"
	case 5:
		return "incompatible"
	case 1:
		return "embedded"
	default:
		return "optional"
	}
}

func projectTypeSlug(pt string) string {
	switch strings.ToLower(pt) {
	case "modpack":
		return "modpack"
	case "resourcepack", "resource_pack":
		return "resourcepack"
	case "shader":
		return "shader"
	case "datapack", "data_pack":
		return "datapack"
	default:
		return "mod"
	}
}

func licenseString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["id"].(string); ok {
			return s
		}
		if s, ok := t["name"].(string); ok {
			return s
		}
	}
	return ""
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
