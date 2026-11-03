package content

import (
	"archive/zip"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

var nativeVersionPattern = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)

func nativeVersionMatches(path, versionID string) bool {
	_, want, ok := strings.Cut(versionID, "@")
	if !ok {
		return false
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name != "horizon-mod.toml" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return false
		}
		b, err := io.ReadAll(io.LimitReader(r, 128<<10))
		r.Close()
		if err != nil {
			return false
		}
		m := nativeVersionPattern.FindSubmatch(b)
		return len(m) == 2 && string(m[1]) == want
	}
	return false
}

type checkRequest struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Hash      string `json:"file_hash_sha1"`
	Filename  string `json:"file_name"`
	Type      string `json:"project_type"`
	RequestID string `json:"request_id"`
}

func CheckInstalled(raw json.RawMessage, batch bool) (any, error) {
	var p struct {
		ProfileID string         `json:"profile_id"`
		Requests  []checkRequest `json:"requests"`
		checkRequest
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	requests := p.Requests
	if !batch {
		requests = []checkRequest{p.checkRequest}
	}
	results := []map[string]any{}
	for _, r := range requests {
		items, err := ListLocal(p.ProfileID, contentTypeOrMod(r.Type), r.Hash != "")
		if err != nil {
			return nil, err
		}
		status := map[string]any{"is_installed": false, "is_included_in_pixelplay_pack": false, "is_specific_version_in_pack": false, "is_enabled": false}
		for _, it := range items {
			matches := (r.Hash != "" && it.Sha1Hash != nil && strings.EqualFold(r.Hash, *it.Sha1Hash)) || (r.Filename != "" && it.Filename == r.Filename) || (r.ProjectID != "" && it.ID == r.ProjectID)
			if r.Hash != "" {
				matches = it.Sha1Hash != nil && strings.EqualFold(r.Hash, *it.Sha1Hash)
			}
			if matches && it.AssociatedLoad == "horizon" && r.VersionID != "" {
				matches = nativeVersionMatches(it.PathStr, r.VersionID)
			}
			if !matches {
				continue
			}
			status["is_installed"] = true
			status["is_enabled"] = !it.IsDisabled
			status["found_item_details"] = map[string]any{"item_type": it.ContentType, "item_id": it.PathStr, "file_name": it.Filename, "display_name": it.Filename}
			break
		}
		if !batch {
			return status, nil
		}
		results = append(results, map[string]any{"request_id": r.RequestID, "project_id": r.ProjectID, "version_id": r.VersionID, "file_name": r.Filename, "project_type": r.Type, "status": status})
	}
	return map[string]any{"results": results}, nil
}
