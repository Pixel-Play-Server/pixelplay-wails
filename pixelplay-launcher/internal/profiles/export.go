package profiles

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/osopen"
	"pixelplay-launcher/internal/paths"
)

func Export(profileID, fileName string, includeFiles []string, openFolder bool) (string, error) {
	raw, err := RawGet(profileID)
	if err != nil {
		return "", err
	}
	var meta struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	safe := sanitizeFile(fileName)
	if safe == "" {
		safe = sanitizeFile(meta.Name)
	}
	if safe == "" {
		safe = "profile"
	}
	exportDir := filepath.Join(paths.DataDir(), "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(exportDir, safe+".pixelplaypack")
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("profile.json")
	if err != nil {
		zw.Close()
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		zw.Close()
		return "", err
	}
	root := paths.ProfileDir(meta.Path)
	for _, rel := range includeFiles {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
		if rel == "" || strings.Contains(rel, "..") {
			continue
		}
		src := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if err != nil {
			continue
		}
		if info.IsDir() {
			_ = filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return nil
				}
				relFile, err := filepath.Rel(root, p)
				if err != nil {
					return nil
				}
				return zipFile(zw, p, filepath.ToSlash(relFile))
			})
			continue
		}
		_ = zipFile(zw, src, rel)
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if openFolder {
		_ = osopen.Path(exportDir)
	}
	return dest, nil
}

func zipFile(zw *zip.Writer, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

func sanitizeFile(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		switch r {
		case '<', '>', ':', '"', '|', '?', '*':
			return '-'
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func SetModUpdates(profileID, modID string, enabled bool) error {
	arr, err := loadAll()
	if err != nil {
		return err
	}
	for i, p := range arr {
		pid, _ := p["id"].(string)
		if pid != profileID {
			continue
		}
		mods, _ := p["mods"].([]any)
		for j, item := range mods {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			if fmt.Sprint(m["id"]) != modID {
				continue
			}
			m["updates_enabled"] = enabled
			mods[j] = m
			arr[i]["mods"] = mods
			return saveAll(arr)
		}
		return fmt.Errorf("mod no encontrado: %s", modID)
	}
	return fmt.Errorf("perfil no encontrado: %s", profileID)
}

func BulkSetModUpdates(profileID string, entries []struct {
	ModID   string `json:"mod_id"`
	Enabled bool   `json:"updates_enabled"`
}) error {
	for _, e := range entries {
		if err := SetModUpdates(profileID, e.ModID, e.Enabled); err != nil {
			return err
		}
	}
	return nil
}
