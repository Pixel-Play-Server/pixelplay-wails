package packs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay.gg/ppack"
)

// PackSummary is enough for the UI to show pack name/protection.
type PackSummary struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"displayName"`
	Description         string `json:"description"`
	IsProtectedByAuthor bool   `json:"isProtectedByAuthor"`
	IsExperimental      bool   `json:"isExperimental"`
	ModCount            int    `json:"modCount"`
}

type packDef struct {
	DisplayName         string          `json:"displayName"`
	Description         string          `json:"description"`
	IsProtectedByAuthor bool            `json:"isProtectedByAuthor"`
	IsExperimental      bool            `json:"isExperimental"`
	Mods                json.RawMessage `json:"mods"`
	PPack               json.RawMessage `json:"ppack"`
}

type fileShape struct {
	Packs        map[string]packDef `json:"packs"`
	Repositories json.RawMessage    `json:"repositories"`
}

func loadFile() (*fileShape, error) {
	data, err := os.ReadFile(paths.PacksFile())
	if err != nil {
		return nil, fmt.Errorf("leer pixelplay_modpacks.json: %w", err)
	}
	var cfg fileShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse packs: %w", err)
	}
	if cfg.Packs == nil {
		cfg.Packs = map[string]packDef{}
	}
	return &cfg, nil
}

func saveFile(cfg *fileShape) error {
	if cfg.Repositories == nil {
		cfg.Repositories = json.RawMessage("{}")
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.PacksFile(), data, 0o644)
}

// RawConfig returns the full modpacks JSON (same shape the Tauri UI expects).
func RawConfig() (json.RawMessage, error) {
	data, err := os.ReadFile(paths.PacksFile())
	if err != nil {
		return nil, fmt.Errorf("leer pixelplay_modpacks.json: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("pixelplay_modpacks.json inválido")
	}
	return json.RawMessage(data), nil
}

// IsProtectedByAuthor checks one pack id (legacy flag or ppack policy).
func IsProtectedByAuthor(packID string) (bool, error) {
	cfg, err := loadFile()
	if err != nil {
		return false, err
	}
	p, ok := cfg.Packs[packID]
	if !ok {
		return false, nil
	}
	if meta, ok := parsePPack(p.PPack); ok {
		return meta.Policy.IsProtectedByAuthor, nil
	}
	return p.IsProtectedByAuthor, nil
}

// PolicyFor returns ppack policy when present.
func PolicyFor(packID string) (*ppack.Policy, error) {
	cfg, err := loadFile()
	if err != nil {
		return nil, err
	}
	p, ok := cfg.Packs[packID]
	if !ok {
		return nil, nil
	}
	if meta, ok := parsePPack(p.PPack); ok {
		pol := meta.Policy
		return &pol, nil
	}
	return &ppack.Policy{IsProtectedByAuthor: p.IsProtectedByAuthor}, nil
}

// UpdaterFor returns remote updater metadata.
func UpdaterFor(packID string) (*ppack.Updater, string, error) {
	cfg, err := loadFile()
	if err != nil {
		return nil, "", err
	}
	p, ok := cfg.Packs[packID]
	if !ok {
		return nil, "", fmt.Errorf("pack desconocido: %s", packID)
	}
	meta, ok := parsePPack(p.PPack)
	if !ok {
		return nil, "", fmt.Errorf("pack sin metadata .ppack")
	}
	if !updaterTargetsPack(packID, meta.Updater.URL) {
		return &ppack.Updater{}, meta.PackVersion, nil
	}
	return &meta.Updater, meta.PackVersion, nil
}

// updaterTargetsPack rejects copies/tests that inherited another pack's updater URL.
func updaterTargetsPack(packID, rawURL string) bool {
	packID = strings.TrimSpace(packID)
	rawURL = strings.TrimSpace(rawURL)
	if packID == "" || rawURL == "" {
		return false
	}
	if !strings.Contains(rawURL, "/modpacks/") {
		return true
	}
	return strings.Contains(rawURL, "/modpacks/"+packID+"/")
}

func parsePPack(raw json.RawMessage) (*ppack.Manifest, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var partial struct {
		Format      string        `json:"format"`
		PackVersion string        `json:"pack_version"`
		Author      string        `json:"author"`
		Copyright   string        `json:"copyright"`
		Policy      ppack.Policy  `json:"policy"`
		Updater     ppack.Updater `json:"updater"`
	}
	if json.Unmarshal(raw, &partial) != nil {
		return nil, false
	}
	m := ppack.NewManifest("", "")
	m.Format = partial.Format
	m.PackVersion = partial.PackVersion
	m.Author = partial.Author
	m.Copyright = partial.Copyright
	m.Policy = partial.Policy
	m.Updater = partial.Updater
	return &m, true
}

// Upsert writes a pack definition into pixelplay_modpacks.json without touching other packs.
func Upsert(id string, m *ppack.Manifest) error {
	if id == "" {
		return fmt.Errorf("id vacío")
	}
	cfg, err := loadFile()
	if err != nil {
		cfg = &fileShape{Packs: map[string]packDef{}, Repositories: json.RawMessage("{}")}
	}
	shape := m.ToLauncherPackShape()
	b, err := json.Marshal(shape)
	if err != nil {
		return err
	}
	var def packDef
	if err := json.Unmarshal(b, &def); err != nil {
		return err
	}
	cfg.Packs[id] = def
	return saveFile(cfg)
}

// ImportPPackFile merges a .ppack into local packs config.
func ImportPPackFile(path string) (*ppack.Manifest, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(strings.ToLower(path), "file://") {
		path = strings.TrimPrefix(path, "file://")
		path = strings.TrimPrefix(path, "FILE://")
	}
	m, err := ppack.ReadManifestFromZip(path)
	if err != nil {
		return nil, err
	}
	cfg, err := loadFile()
	if err != nil {
		cfg = &fileShape{Packs: map[string]packDef{}, Repositories: json.RawMessage("{}")}
	}
	shape := m.ToLauncherPackShape()
	b, _ := json.Marshal(shape)
	var def packDef
	_ = json.Unmarshal(b, &def)
	cfg.Packs[m.ID] = def

	destDir := filepath.Join(paths.DataDir(), "ppacks", m.ID)
	_ = os.MkdirAll(destDir, 0o755)
	dest := filepath.Join(destDir, filepath.Base(path))
	if in, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(dest, in, 0o644)
	}
	_ = ppack.ExtractZip(path, destDir)
	if err := saveFile(cfg); err != nil {
		return nil, err
	}
	migrateProfilesToPack(m)
	return m, nil
}

// migrateProfilesToPack moves linked profiles to the pack's game version/loader
// (e.g. NeoForge 1.21.1 -> Fabric 1.21.11), otherwise pack mods never match.
func migrateProfilesToPack(m *ppack.Manifest) {
	gv := strings.TrimSpace(m.GameVersion)
	ld := strings.ToLower(strings.TrimSpace(m.Loader))
	if m.ID == "" || gv == "" || ld == "" {
		return
	}
	list, err := profiles.List()
	if err != nil {
		return
	}
	for _, p := range list {
		if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID != m.ID {
			continue
		}
		if p.GameVersion == gv && strings.EqualFold(p.Loader, ld) {
			continue
		}
		_ = profiles.Update(p.ID, map[string]any{"game_version": gv, "loader": ld})
		_ = profiles.ClearLoaderVersion(p.ID)
	}
}

// List reads local pixelplay_modpacks.json (written by pixel-sync / pixelmodpack).
func List() ([]PackSummary, error) {
	cfg, err := loadFile()
	if err != nil {
		return nil, err
	}
	out := make([]PackSummary, 0, len(cfg.Packs))
	for id, p := range cfg.Packs {
		modCount := 0
		var mods []json.RawMessage
		if len(p.Mods) > 0 && json.Unmarshal(p.Mods, &mods) == nil {
			modCount = len(mods)
		}
		prot := p.IsProtectedByAuthor
		if meta, ok := parsePPack(p.PPack); ok {
			prot = meta.Policy.IsProtectedByAuthor
		}
		out = append(out, PackSummary{
			ID:                  id,
			DisplayName:         p.DisplayName,
			Description:         p.Description,
			IsProtectedByAuthor: prot,
			IsExperimental:      p.IsExperimental,
			ModCount:            modCount,
		})
	}
	return out, nil
}

// DisplayNameOf returns the pack's public name, or fallback if unknown.
func DisplayNameOf(packID, fallback string) string {
	packID = strings.TrimSpace(packID)
	if packID == "" {
		return fallback
	}
	list, err := List()
	if err != nil {
		return fallback
	}
	for _, p := range list {
		if p.ID == packID && strings.TrimSpace(p.DisplayName) != "" {
			return p.DisplayName
		}
	}
	return fallback
}

func PackMods(packID string) (json.RawMessage, error) {
	cfg, err := loadFile()
	if err != nil {
		return nil, err
	}
	p, ok := cfg.Packs[packID]
	if !ok {
		return json.RawMessage("[]"), nil
	}
	if len(p.Mods) == 0 {
		return json.RawMessage("[]"), nil
	}
	if json.Valid(p.Mods) && len(p.Mods) > 0 && p.Mods[0] == '[' {
		return p.Mods, nil
	}
	return json.RawMessage("[]"), nil
}

func ProfilePackMods(profileID string) (json.RawMessage, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return nil, err
	}
	if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID == "" {
		return json.RawMessage("[]"), nil
	}
	return PackMods(*p.SelectedPixelplayPackID)
}
