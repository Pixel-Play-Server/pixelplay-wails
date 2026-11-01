package modrinth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.modrinth.com/v2"
const userAgent = "PixelPlayLauncher/0.2 (wails; contact@pixelplay.gg)"

var client = &http.Client{Timeout: 45 * time.Second}

func getJSON(path string, query url.Values, dest any) error {
	u := apiBase + path
	if query != nil {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("modrinth %s: %s — %s", path, res.Status, truncate(string(body), 200))
	}
	return json.Unmarshal(body, dest)
}

func postJSON(path string, payload any, dest any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, apiBase+path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("modrinth POST %s: %s — %s", path, res.Status, truncate(string(body), 200))
	}
	return json.Unmarshal(body, dest)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type SearchResponse struct {
	Hits      []json.RawMessage `json:"hits"`
	Offset    int               `json:"offset"`
	Limit     int               `json:"limit"`
	TotalHits int               `json:"total_hits"`
}

type SearchParams struct {
	Query            string
	ProjectType      string
	GameVersion      string
	Loader           string
	Limit            int
	Offset           int
	Sort             string
	Categories       []string
	ClientSideFilter string
	ServerSideFilter string
}

func SearchProjects(p SearchParams) (SearchResponse, error) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	facets := [][]string{}
	pt := strings.TrimSpace(p.ProjectType)
	if pt == "" {
		pt = "mod"
	}
	facets = append(facets, []string{"project_type:" + strings.ToLower(pt)})
	if p.GameVersion != "" {
		facets = append(facets, []string{"versions:" + p.GameVersion})
	}
	if p.Loader != "" {
		facets = append(facets, []string{"categories:" + strings.ToLower(p.Loader)})
	}
	for _, c := range p.Categories {
		if c != "" {
			facets = append(facets, []string{"categories:" + c})
		}
	}
	if p.ClientSideFilter != "" {
		facets = append(facets, []string{"client_side:" + p.ClientSideFilter})
	}
	if p.ServerSideFilter != "" {
		facets = append(facets, []string{"server_side:" + p.ServerSideFilter})
	}
	facetsJSON, _ := json.Marshal(facets)
	q := url.Values{}
	q.Set("query", p.Query)
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))
	q.Set("facets", string(facetsJSON))
	if p.Sort != "" {
		q.Set("index", sortIndex(p.Sort))
	}
	var out SearchResponse
	err := getJSON("/search", q, &out)
	return out, err
}

func sortIndex(sort string) string {
	switch strings.ToLower(sort) {
	case "relevance":
		return "relevance"
	case "downloads":
		return "downloads"
	case "follows":
		return "follows"
	case "newest", "newestpublished", "date_created":
		return "newest"
	case "updated", "date_modified":
		return "updated"
	default:
		return "relevance"
	}
}

func SearchMods(query, gameVersion, loader string, limit int) ([]json.RawMessage, error) {
	res, err := SearchProjects(SearchParams{
		Query:       query,
		ProjectType: "mod",
		GameVersion: gameVersion,
		Loader:      loader,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	return res.Hits, nil
}

func ProjectVersions(idOrSlug string, loaders, gameVersions []string) (json.RawMessage, error) {
	q := url.Values{}
	if len(loaders) > 0 {
		b, _ := json.Marshal(loaders)
		q.Set("loaders", string(b))
	}
	if len(gameVersions) > 0 {
		b, _ := json.Marshal(gameVersions)
		q.Set("game_versions", string(b))
	}
	var raw json.RawMessage
	err := getJSON("/project/"+url.PathEscape(idOrSlug)+"/version", q, &raw)
	return raw, err
}

func Projects(ids []string) (json.RawMessage, error) {
	if len(ids) == 0 {
		return json.RawMessage("[]"), nil
	}
	b, _ := json.Marshal(ids)
	q := url.Values{}
	q.Set("ids", string(b))
	var raw json.RawMessage
	err := getJSON("/projects", q, &raw)
	return raw, err
}

func VersionsByHashes(hashes []string) (json.RawMessage, error) {
	if len(hashes) == 0 {
		return json.RawMessage("{}"), nil
	}
	payload := map[string]any{
		"hashes":    hashes,
		"algorithm": "sha1",
	}
	var raw json.RawMessage
	err := postJSON("/version_files", payload, &raw)
	return raw, err
}

func CheckUpdates(body json.RawMessage) (json.RawMessage, error) {
	req, err := http.NewRequest(http.MethodPost, apiBase+"/version_files/update", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("modrinth update check: %s — %s", res.Status, truncate(string(raw), 200))
	}
	return json.RawMessage(raw), nil
}

func Categories() (json.RawMessage, error) {
	var raw json.RawMessage
	err := getJSON("/tag/category", nil, &raw)
	return raw, err
}

func Loaders() (json.RawMessage, error) {
	var raw json.RawMessage
	err := getJSON("/tag/loader", nil, &raw)
	return raw, err
}

func GameVersions() (json.RawMessage, error) {
	var raw json.RawMessage
	err := getJSON("/tag/game_version", nil, &raw)
	return raw, err
}

// Version is a subset of Modrinth's version object used for modpack installs.
type Version struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []File   `json:"files"`
}

type File struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
}

// MrpackCandidate is the downloadable .mrpack resolved for a game/loader combo.
type MrpackCandidate struct {
	ProjectSlug   string `json:"project_slug"`
	ProjectID     string `json:"project_id"`
	VersionID     string `json:"version_id"`
	VersionNumber string `json:"version_number"`
	VersionName   string `json:"version_name"`
	DownloadURL   string `json:"download_url"`
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
}

// ListVersions returns typed project versions filtered by optional loaders/game versions.
func ListVersions(idOrSlug string, loaders, gameVersions []string) ([]Version, error) {
	raw, err := ProjectVersions(idOrSlug, loaders, gameVersions)
	if err != nil {
		return nil, err
	}
	var versions []Version
	if err := json.Unmarshal(raw, &versions); err != nil {
		return nil, fmt.Errorf("parse versions: %w", err)
	}
	return versions, nil
}

func containsFold(list []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, v := range list {
		if strings.ToLower(strings.TrimSpace(v)) == want {
			return true
		}
	}
	return false
}

func primaryMrpack(files []File) (File, bool) {
	var fallback File
	found := false
	for _, f := range files {
		name := strings.ToLower(f.Filename)
		if !strings.HasSuffix(name, ".mrpack") {
			continue
		}
		if f.Primary {
			return f, true
		}
		if !found {
			fallback = f
			found = true
		}
	}
	return fallback, found
}

// FindMrpack resolves the newest version of a Modrinth modpack for gameVersion+loader
// and returns its primary .mrpack download URL.
func FindMrpack(projectSlug, gameVersion, loader string) (*MrpackCandidate, error) {
	projectSlug = strings.TrimSpace(projectSlug)
	gameVersion = strings.TrimSpace(gameVersion)
	loader = strings.TrimSpace(loader)
	if projectSlug == "" {
		return nil, fmt.Errorf("falta project slug")
	}
	if gameVersion == "" {
		return nil, fmt.Errorf("falta game version")
	}
	if loader == "" {
		loader = "fabric"
	}

	versions, err := ListVersions(projectSlug, []string{loader}, []string{gameVersion})
	if err != nil {
		return nil, err
	}
	// Defensive client-side filter (API already filters, but keep exact match).
	filtered := make([]Version, 0, len(versions))
	for _, v := range versions {
		if !containsFold(v.GameVersions, gameVersion) {
			continue
		}
		if !containsFold(v.Loaders, loader) {
			continue
		}
		filtered = append(filtered, v)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no hay versión de %s para %s/%s", projectSlug, gameVersion, loader)
	}

	// Modrinth returns newest first; take the first with a .mrpack file.
	for _, v := range filtered {
		file, ok := primaryMrpack(v.Files)
		if !ok || strings.TrimSpace(file.URL) == "" {
			continue
		}
		return &MrpackCandidate{
			ProjectSlug:   projectSlug,
			ProjectID:     v.ProjectID,
			VersionID:     v.ID,
			VersionNumber: v.VersionNumber,
			VersionName:   v.Name,
			DownloadURL:   file.URL,
			FileName:      file.Filename,
			FileSize:      file.Size,
		}, nil
	}
	return nil, fmt.Errorf("ninguna versión de %s trae .mrpack para %s/%s", projectSlug, gameVersion, loader)
}

// DefaultOptimizedPackSlugs: Fabulously Optimized first; Simply Optimized slug is gone
// on Modrinth, so "sop-continued" (Simply Optimized Continued) is the practical fallback.
var DefaultOptimizedPackSlugs = []string{
	"fabulously-optimized",
	"sop-continued",
	"simply-optimized",
}

// FindOptimizedClientMrpack tries preferred slugs until one resolves for gameVersion/loader.
func FindOptimizedClientMrpack(gameVersion, loader string, preferredSlugs ...string) (*MrpackCandidate, error) {
	slugs := preferredSlugs
	if len(slugs) == 0 {
		slugs = append([]string{}, DefaultOptimizedPackSlugs...)
	}
	var errs []string
	for _, slug := range slugs {
		cand, err := FindMrpack(slug, gameVersion, loader)
		if err == nil {
			return cand, nil
		}
		errs = append(errs, slug+": "+err.Error())
	}
	return nil, fmt.Errorf("no se encontró modpack de optimización (%s)", strings.Join(errs, "; "))
}
