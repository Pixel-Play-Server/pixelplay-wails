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

// zipEntryBase returns the file basename of a zip entry, normalizing Windows '\' paths.
// On Linux filepath.ToSlash / filepath.Base do NOT treat '\' as a separator, so packs
// zipped on Windows (or some GUI tools) would fail to find manifest.json otherwise.
func zipEntryBase(name string) string {
	n := strings.ReplaceAll(name, "\\", "/")
	n = strings.TrimSuffix(n, "/")
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:]
	}
	return n
}

func isManifestFileName(base string) bool {
	switch strings.ToLower(strings.TrimSpace(base)) {
	case ManifestName, "manifesto.json": // alias tipográfico / docs viejos
		return true
	default:
		return false
	}
}

// Policy controls how the launcher treats an installed pack.
type Policy struct {
	IsProtectedByAuthor  bool     `json:"is_protected_by_author"`
	AllowModAutoUpdates  bool     `json:"allow_mod_auto_updates"`
	BlockPackModUpdates  bool     `json:"block_pack_mod_updates"`
	AllowOpenFolder      bool     `json:"allow_open_folder"`
	DefaultResourcePacks []string `json:"default_resource_packs"`
	DefaultShaderPacks   []string `json:"default_shader_packs"`
	HideModsUI           bool     `json:"hide_mods_ui"`
	// AutoJoinServer, if set, is passed as --quickPlayMultiplayer on launch (and Test).
	AutoJoinServer string `json:"autojoin_server,omitempty"`
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
	// ConfigSource is the last folder imported into overrides/ (Studio metadata only).
	ConfigSource string `json:"config_source,omitempty"`
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
	path = strings.TrimSpace(path)
	if strings.HasPrefix(strings.ToLower(path), "file://") {
		path = strings.TrimPrefix(path, "file://")
		path = strings.TrimPrefix(path, "file:")
		if u, err := decodeFileURI(path); err == nil {
			path = u
		}
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !isManifestFileName(zipEntryBase(f.Name)) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.Name, err)
		}
		if m.Format == "" {
			m.Format = FormatName
		}
		return &m, nil
	}
	return nil, fmt.Errorf("manifest.json no encontrado en %s", path)
}

func decodeFileURI(p string) (string, error) {
	// path may already be decoded; best-effort unescape (%20 etc.)
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '%' && i+2 < len(p) {
			var v byte
			ok := true
			for _, c := range []byte{p[i+1], p[i+2]} {
				v <<= 4
				switch {
				case c >= '0' && c <= '9':
					v |= c - '0'
				case c >= 'a' && c <= 'f':
					v |= c - 'a' + 10
				case c >= 'A' && c <= 'F':
					v |= c - 'A' + 10
				default:
					ok = false
				}
			}
			if ok {
				b.WriteByte(v)
				i += 2
				continue
			}
		}
		b.WriteByte(p[i])
	}
	out := b.String()
	if out == "" {
		return p, fmt.Errorf("empty")
	}
	return out, nil
}

// ReadManifestFromDir reads manifest.json from an extracted project folder.
func ReadManifestFromDir(dir string) (*Manifest, error) {
	candidates := []string{
		filepath.Join(dir, ManifestName),
		filepath.Join(dir, "manifesto.json"),
	}
	// case-insensitive scan of top-level (Linux / Mint)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if isManifestFileName(e.Name()) {
				candidates = append([]string{filepath.Join(dir, e.Name())}, candidates...)
				break
			}
		}
	}
	var lastErr error
	for _, mp := range candidates {
		data, err := os.ReadFile(mp)
		if err != nil {
			lastErr = err
			continue
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
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("manifest.json no encontrado en %s", dir)
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
	destAbs, _ := filepath.Abs(destPath)
	srcAbs, _ := filepath.Abs(srcDir)
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()

	added := 0
	skipped := 0
	err = filepath.Walk(srcAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "ppack walk err %s: %v\n", path, err)
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if path != srcAbs && (base == "exports" || base == ".git") {
				fmt.Fprintf(os.Stderr, "ppack skip dir %s\n", path)
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			skipped++
			return nil
		}
		if destAbs != "" && filepath.Clean(path) == destAbs {
			skipped++
			return nil
		}
		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "exports/") {
			skipped++
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		n, err := io.Copy(w, f)
		if err != nil {
			return err
		}
		added++
		fmt.Fprintf(os.Stderr, "ppack + %s (%d bytes)\n", rel, n)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppack WriteZip FAIL dest=%s: %v\n", destPath, err)
		return err
	}
	fmt.Fprintf(os.Stderr, "ppack WriteZip OK dest=%s files=%d skipped=%d\n", destPath, added, skipped)
	return nil
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
		_ = SeedDefaultFolders(dir)
		return &m, nil
	}
	m := NewManifest(id, name)
	if err := SaveManifest(mp, &m); err != nil {
		return nil, err
	}
	_ = SeedDefaultFolders(dir)
	return &m, nil
}

// DefaultProjectFolders is the skeleton every Pixel Studio pack gets.
var DefaultProjectFolders = []string{
	"overrides/config",
	"overrides/kubejs",
	"overrides/defaultconfigs",
	"resourcepacks",
	"shaderpacks",
}

// SeedDefaultFolders creates the default pack layout (config/kubejs/packs).
func SeedDefaultFolders(dir string) error {
	if dir == "" {
		return fmt.Errorf("dir vacío")
	}
	for _, rel := range DefaultProjectFolders {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
		keep := filepath.Join(p, ".keep")
		if _, err := os.Stat(keep); err == nil {
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			_ = os.WriteFile(keep, []byte{}, 0o644)
		}
	}
	readme := filepath.Join(dir, "overrides", "README.txt")
	if _, err := os.Stat(readme); err != nil {
		_ = os.WriteFile(readme, []byte("Carpeta default del pack.\n\nconfig/  kubejs/  defaultconfigs/  (se copian al cliente)\nEn la raíz del proyecto: resourcepacks/ y shaderpacks/\n"), 0o644)
	}
	return nil
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
		if !f.FileInfo().IsDir() && isManifestFileName(zipEntryBase(f.Name)) {
			return true
		}
	}
	return false
}

// BufferZip helper for in-memory checks.
func BufferZip(data []byte) bool {
	return ZipContainsManifest(bytes.NewReader(data), int64(len(data)))
}
