package profiles

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"pixelplay-launcher/internal/paths"
)

// ZipHasProfileJSON reports whether the archive contains a top-level profile.json
// (launcher profile export / .pixelplaypack), as opposed to a Studio manifest.json pack.
func ZipHasProfileJSON(path string) bool {
	path = normalizeZipPath(path)
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(zipEntryBase(f.Name), "profile.json") {
			return true
		}
	}
	return false
}

// ImportFromZip imports a launcher-exported profile archive
// (.pixelplaypack / mislabeled .ppack with profile.json).
func ImportFromZip(zipPath string) (string, error) {
	zipPath = normalizeZipPath(zipPath)
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("abrir archivo: %w", err)
	}
	defer zr.Close()

	var profileRaw []byte
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.EqualFold(zipEntryBase(f.Name), "profile.json") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		profileRaw, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		break
	}
	if len(profileRaw) == 0 {
		return "", fmt.Errorf("profile.json no encontrado en %s", zipPath)
	}

	var exported map[string]any
	if err := json.Unmarshal(profileRaw, &exported); err != nil {
		return "", fmt.Errorf("profile.json inválido: %w", err)
	}

	name, _ := exported["name"].(string)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	}
	if name == "" {
		name = "perfil-importado"
	}
	gv, _ := exported["game_version"].(string)
	if gv == "" {
		gv = "1.21.1"
	}
	loader, _ := exported["loader"].(string)
	if loader == "" {
		loader = "vanilla"
	}

	id := uuid.NewString()
	pathSeg := uniqueProfilePath(name)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	exported["id"] = id
	exported["name"] = name
	exported["path"] = pathSeg
	exported["game_version"] = gv
	exported["loader"] = strings.ToLower(loader)
	exported["created"] = now
	exported["last_played"] = nil
	exported["state"] = "not_installed"
	exported["is_standard_version"] = false
	if exported["settings"] == nil {
		exported["settings"] = defaultSettings()
	}
	if exported["mods"] == nil {
		exported["mods"] = []any{}
	}
	if exported["disabled_pixelplay_mods_detailed"] == nil {
		exported["disabled_pixelplay_mods_detailed"] = []any{}
	}

	arr, err := loadAll()
	if err != nil {
		return "", err
	}
	arr = append(arr, exported)
	if err := saveAll(arr); err != nil {
		return "", err
	}

	destRoot := paths.ProfileDir(pathSeg)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return "", err
	}
	if err := extractProfileZipFiles(zr, destRoot); err != nil {
		return id, fmt.Errorf("perfil creado pero falló extraer archivos: %w", err)
	}
	return id, nil
}

func normalizeZipPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(strings.ToLower(path), "file://") {
		path = strings.TrimPrefix(path, "file://")
		path = strings.TrimPrefix(path, "FILE://")
	}
	return path
}

func zipEntryBase(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func extractProfileZipFiles(zr *zip.ReadCloser, destRoot string) error {
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := strings.ReplaceAll(f.Name, "\\", "/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.EqualFold(zipEntryBase(rel), "profile.json") {
			continue
		}
		// Tauri / Modrinth-style: files under overrides/
		if strings.HasPrefix(strings.ToLower(rel), "overrides/") {
			rel = rel[len("overrides/"):]
		}
		if rel == "" || strings.Contains(rel, "..") {
			continue
		}
		out := filepath.Join(destRoot, filepath.FromSlash(rel))
		if relCheck, err := filepath.Rel(destRoot, out); err != nil || strings.HasPrefix(relCheck, "..") {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(out)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
