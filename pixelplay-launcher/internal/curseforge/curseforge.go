package curseforge

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"pixelplay-launcher/internal/httpx"
)

const (
	apiBase = "https://api.curseforge.com/v1"
	// Same public key used by the Tauri launcher / Prism-class clients.
	apiKey = "$2a$10$bL4bIL5pUWqfcO7KQtnMReakwtfHbNKh6v1uTpKlzhwoueEJQnPnm"
	mcGame = 432
)

func headers() map[string]string {
	return map[string]string{"x-api-key": apiKey}
}

type Pagination struct {
	Index       int `json:"index"`
	PageSize    int `json:"pageSize"`
	ResultCount int `json:"resultCount"`
	TotalCount  int `json:"totalCount"`
}

type Mod struct {
	ID            uint64      `json:"id"`
	Name          string      `json:"name"`
	Slug          string      `json:"slug"`
	Summary       string      `json:"summary"`
	DownloadCount uint64      `json:"downloadCount"`
	ClassID       *uint32     `json:"classId"`
	Authors       []Author    `json:"authors"`
	Logo          *Attachment `json:"logo"`
	Links         Links       `json:"links"`
	Categories    []Category  `json:"categories"`
	DateCreated   string      `json:"dateCreated"`
	DateModified  string      `json:"dateModified"`
}

type Author struct {
	Name string `json:"name"`
}

type Attachment struct {
	URL string `json:"url"`
}

type Links struct {
	WebsiteURL string `json:"websiteUrl"`
}

type Category struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type SearchResponse struct {
	Data       []Mod      `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type File struct {
	ID            uint64     `json:"id"`
	ModID         uint64     `json:"modId"`
	DisplayName   string     `json:"displayName"`
	FileName      string     `json:"fileName"`
	ReleaseType   int        `json:"releaseType"`
	FileDate      string     `json:"fileDate"`
	FileLength    int64      `json:"fileLength"`
	DownloadCount uint64     `json:"downloadCount"`
	DownloadURL   string     `json:"downloadUrl"`
	GameVersions  []string   `json:"gameVersions"`
	Hashes        []FileHash `json:"hashes"`
	Dependencies  []FileDep  `json:"dependencies"`
}

type FileHash struct {
	Value string `json:"value"`
	Algo  int    `json:"algo"`
}

type FileDep struct {
	ModID        uint64 `json:"modId"`
	RelationType int    `json:"relationType"`
}

type FilesResponse struct {
	Data       []File     `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type SearchOpts struct {
	Query       string
	ClassID     int
	GameVersion string
	Loaders     []int
	SortField   int
	SortOrder   string
	Index       int
	PageSize    int
}

func Search(opts SearchOpts) (SearchResponse, error) {
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}
	if opts.PageSize > 50 {
		opts.PageSize = 50
	}
	if opts.SortField <= 0 {
		opts.SortField = 2 // popularity
	}
	if opts.SortOrder == "" {
		opts.SortOrder = "desc"
	}
	q := url.Values{}
	q.Set("gameId", strconv.Itoa(mcGame))
	if opts.Query != "" {
		q.Set("searchFilter", opts.Query)
	}
	if opts.ClassID > 0 {
		q.Set("classId", strconv.Itoa(opts.ClassID))
	}
	if opts.GameVersion != "" {
		q.Set("gameVersion", opts.GameVersion)
	}
	q.Set("sortField", strconv.Itoa(opts.SortField))
	q.Set("sortOrder", opts.SortOrder)
	if len(opts.Loaders) > 0 {
		ids := make([]string, 0, len(opts.Loaders))
		for _, id := range opts.Loaders {
			ids = append(ids, strconv.Itoa(id))
		}
		q.Set("modLoaderTypes", strings.Join(ids, ","))
	}
	q.Set("index", strconv.Itoa(opts.Index))
	q.Set("pageSize", strconv.Itoa(opts.PageSize))
	var out SearchResponse
	err := httpx.GetJSONHeader(apiBase+"/mods/search?"+q.Encode(), headers(), &out)
	return out, err
}

func GetFiles(modID string, gameVersion string, loader int, index, pageSize int) (FilesResponse, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 50 {
		pageSize = 50
	}
	q := url.Values{}
	if gameVersion != "" {
		q.Set("gameVersion", gameVersion)
	}
	if loader > 0 {
		q.Set("modLoaderType", strconv.Itoa(loader))
	}
	q.Set("index", strconv.Itoa(index))
	q.Set("pageSize", strconv.Itoa(pageSize))
	u := apiBase + "/mods/" + url.PathEscape(modID) + "/files"
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	var out FilesResponse
	err := httpx.GetJSONHeader(u, headers(), &out)
	return out, err
}

func Changelog(modID, fileID string) (string, error) {
	var out struct {
		Data string `json:"data"`
	}
	u := fmt.Sprintf("%s/mods/%s/files/%s/changelog", apiBase, url.PathEscape(modID), url.PathEscape(fileID))
	if err := httpx.GetJSONHeader(u, headers(), &out); err != nil {
		return "", err
	}
	return out.Data, nil
}

func ModsByIDs(ids []uint64) ([]Mod, error) {
	if len(ids) == 0 {
		return []Mod{}, nil
	}
	var out struct {
		Data []Mod `json:"data"`
	}
	if err := httpx.PostJSONHeader(apiBase+"/mods", headers(), map[string]any{"modIds": ids}, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func GetFile(modID, fileID uint64) (*File, error) {
	var out struct {
		Data File `json:"data"`
	}
	u := fmt.Sprintf("%s/mods/%d/files/%d", apiBase, modID, fileID)
	if err := httpx.GetJSONHeader(u, headers(), &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func ClassID(projectType string) int {
	switch strings.ToLower(projectType) {
	case "mod":
		return 6
	case "modpack":
		return 4471
	case "resourcepack", "resource_pack":
		return 12
	case "shader":
		return 6552
	case "datapack", "data_pack":
		return 119
	default:
		return 6
	}
}

func LoaderID(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "forge":
		return 1
	case "cauldron":
		return 2
	case "liteloader":
		return 3
	case "fabric":
		return 4
	case "quilt":
		return 5
	case "neoforge":
		return 6
	default:
		return 0
	}
}

func SortField(sort string) (int, string) {
	switch strings.ToLower(sort) {
	case "downloads":
		return 6, "desc"
	case "newest", "updated":
		return 3, "desc"
	case "name":
		return 4, "asc"
	case "author":
		return 5, "asc"
	case "featured":
		return 1, "desc"
	case "category":
		return 7, "asc"
	case "gameversion", "game_version":
		return 8, "desc"
	default:
		return 2, "desc" // popularity
	}
}
