package ppack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	FormatName    = "ppack"
	FormatVersion = 1
	ManifestName  = "manifest.json"
	// DefaultUpdaterTemplate is hosted on the Pixel panel (same host as launcher updates).
	DefaultUpdaterTemplate = "https://panel.pixelplay.gg/dl/pixelplay/modpacks/{id}/version.json"
)

// Policy controls how the launcher treats an installed pack.
type Policy struct {
	IsProtectedByAuthor   bool     `json:"is_protected_by_author"`
	AllowModAutoUpdates   bool     `json:"allow_mod_auto_updates"`
	BlockPackModUpdates   bool     `json:"block_pack_mod_updates"`
	AllowOpenFolder       bool     `json:"allow_open_folder"`
	DefaultResourcePacks  []string `json:"default_resource_packs"`
	DefaultShaderPacks    []string `json:"default_shader_packs"`
	HideModsUI            bool     `json:"hide_mods_ui"`
}

// Updater points at a remote version.json for the pack (like launcher updates).
type Updater struct {
	URL     string `json:"url"`
	Channel string `json:"channel"`
}

// Manifest is the root metadata stored inside a .ppack zip.
type Manifest struct {
	Format        string          `json:"format"`
	FormatVersion int             `json:"format_version"`
	ID            string          `json:"id"`
	DisplayName   string          `json:"display_name"`
	Description   string          `json:"description"`
	Author        string          `json:"author"`
	Copyright     string          `json:"copyright"`
	Icon          string          `json:"icon"`
	GameVersion   string          `json:"game_version"`
	Loader        string          `json:"loader"`
	PackVersion   string          `json:"pack_version"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	Policy        Policy          `json:"policy"`
	Updater       Updater         `json:"updater"`
	Mods          json.RawMessage `json:"mods"`
	Repositories  json.RawMessage `json:"repositories"`
	Assets        []string        `json:"assets,omitempty"`
	InheritsFrom  []string        `json:"inherits_from,omitempty"`
	ExcludeMods   []string        `json:"exclude_mods,omitempty"`
	Notes         string          `json:"notes,omitempty"`
}

// VersionManifest is published next to .ppack artifacts on panel.pixelplay.gg.
type VersionManifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	PubDate string `json:"pub_date"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{
		IsProtectedByAuthor:  true,
		AllowModAutoUpdates:  false,
		BlockPackModUpdates:  true,
		AllowOpenFolder:      false,
		DefaultResourcePacks: []string{},
		DefaultShaderPacks:   []string{},
		HideModsUI:           true,
	}
}

func DefaultUpdater(packID string) Updater {
	url := strings.ReplaceAll(DefaultUpdaterTemplate, "{id}", packID)
	return Updater{URL: url, Channel: "stable"}
}

func NewManifest(id, name string) Manifest {
	now := time.Now().UTC().Format(time.RFC3339)
	return Manifest{
		Format:        FormatName,
		FormatVersion: FormatVersion,
		ID:            id,
		DisplayName:   name,
		Description:   "",
		Author:        "",
		Copyright:     "",
		Icon:          "icon.png",
		GameVersion:   "1.21.1",
		Loader:        "neoforge",
		PackVersion:   "0.1.0",
		CreatedAt:     now,
		UpdatedAt:     now,
		Policy:        DefaultPolicy(),
		Updater:       DefaultUpdater(id),
		Mods:          json.RawMessage("[]"),
		Repositories:  json.RawMessage("{}"),
	}
}

// ReadManifestFromZip reads manifest.json from a .ppack (or .zip) file.
func ReadManifestFromZip(path string) (*Manifest, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if name == ManifestName || strings.HasSuffix(name, "/"+ManifestName) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, err
			}
			if m.Format == "" {
				m.Format = FormatName
			}
			return &m, nil
		}
	}
	return nil, fmt.Errorf("manifest.json no encontrado en %s", path)
}

// WriteZip builds a .ppack from a working directory that contains manifest.json (+ optional files).
func WriteZip(srcDir, destPath string) error {
	manifestPath := filepath.Join(srcDir, ManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("leer manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	m.Format = FormatName
	m.FormatVersion = FormatVersion
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if m.Updater.URL == "" && m.ID != "" {
		m.Updater = DefaultUpdater(m.ID)
	}
	normalized, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, normalized, 0o644); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	return err
}

// ImportLegacyJSON converts a pixelplay pack definition JSON object into a Manifest.
func ImportLegacyJSON(packID string, raw json.RawMessage) (*Manifest, error) {
	var legacy struct {
		DisplayName         string          `json:"displayName"`
		Description         string          `json:"description"`
		IsProtectedByAuthor bool            `json:"isProtectedByAuthor"`
		IsExperimental      bool            `json:"isExperimental"`
		Mods                json.RawMessage `json:"mods"`
		Assets              []string        `json:"assets"`
		InheritsFrom        []string        `json:"inheritsFrom"`
		ExcludeMods         []string        `json:"excludeMods"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, err
	}
	m := NewManifest(packID, legacy.DisplayName)
	m.Description = legacy.Description
	m.Policy.IsProtectedByAuthor = legacy.IsProtectedByAuthor
	m.Policy.HideModsUI = legacy.IsProtectedByAuthor
	m.Policy.BlockPackModUpdates = legacy.IsProtectedByAuthor
	m.Policy.AllowOpenFolder = !legacy.IsProtectedByAuthor
	if len(legacy.Mods) > 0 {
		m.Mods = legacy.Mods
	}
	m.Assets = legacy.Assets
	m.InheritsFrom = legacy.InheritsFrom
	m.ExcludeMods = legacy.ExcludeMods
	_ = legacy.IsExperimental
	return &m, nil
}

// ToLauncherPackShape maps a Manifest into the launcher packs.json entry fields.
func (m *Manifest) ToLauncherPackShape() map[string]any {
	return map[string]any{
		"displayName":         m.DisplayName,
		"description":         m.Description,
		"isProtectedByAuthor": m.Policy.IsProtectedByAuthor,
		"isExperimental":      false,
		"mods":                json.RawMessage(m.Mods),
		"assets":              m.Assets,
		"inheritsFrom":        m.InheritsFrom,
		"excludeMods":         m.ExcludeMods,
		"ppack": map[string]any{
			"format":        m.Format,
			"format_version": m.FormatVersion,
			"pack_version":  m.PackVersion,
			"author":        m.Author,
			"copyright":     m.Copyright,
			"icon":          m.Icon,
			"game_version":  m.GameVersion,
			"loader":        m.Loader,
			"policy":        m.Policy,
			"updater":       m.Updater,
			"notes":         m.Notes,
		},
	}
}

// FileSHA256 returns hex sha256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BuildVersionManifest creates the remote updater document for a .ppack URL.
func BuildVersionManifest(m Manifest, downloadURL, sha string) VersionManifest {
	return VersionManifest{
		Version: m.PackVersion,
		Notes:   m.Notes,
		PubDate: time.Now().UTC().Format(time.RFC3339),
		URL:     downloadURL,
		SHA256:  sha,
	}
}

// SaveManifest writes pretty JSON to a path.
func SaveManifest(path string, m *Manifest) error {
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ExtractZip extracts a .ppack into destDir.
func ExtractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("path traversal: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// IsPPackPath reports whether the path looks like a Pixel pack archive.
func IsPPackPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".ppack" || ext == ".pixelplaypack" || ext == ".zip"
}

// EnsureWorkDir creates a scratch dir with a default manifest if empty.
func EnsureWorkDir(dir string, id, name string) (*Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	mp := filepath.Join(dir, ManifestName)
	if _, err := os.Stat(mp); err == nil {
		data, err := os.ReadFile(mp)
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return &m, nil
	}
	m := NewManifest(id, name)
	if err := SaveManifest(mp, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// BytesManifest helper for tests.
func BytesManifest(m Manifest) []byte {
	b, _ := json.Marshal(m)
	return b
}

// ZipContainsManifest quick check without full extract.
func ZipContainsManifest(r io.ReaderAt, size int64) bool {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == ManifestName {
			return true
		}
	}
	return false
}

// BufferZip helper for in-memory checks.
func BufferZip(data []byte) bool {
	return ZipContainsManifest(bytes.NewReader(data), int64(len(data)))
}
