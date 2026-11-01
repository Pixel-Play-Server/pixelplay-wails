package content

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
)

type Item struct {
	Filename       string  `json:"filename"`
	PathStr        string  `json:"path_str"`
	Sha1Hash       *string `json:"sha1_hash"`
	FileSize       int64   `json:"file_size"`
	IsDisabled     bool    `json:"is_disabled"`
	IsDirectory    bool    `json:"is_directory"`
	ContentType    string  `json:"content_type"`
	ModrinthInfo   any     `json:"modrinth_info"`
	CurseforgeInfo any     `json:"curseforge_info"`
	Platform       any     `json:"platform"`
	SourceType     any     `json:"source_type"`
	PixelplayInfo  any     `json:"pixelplay_info"`
	FallbackVer    any     `json:"fallback_version"`
	ID             any     `json:"id"`
	AssociatedLoad any     `json:"associated_loader"`
	ModpackOrigin  any     `json:"modpack_origin"`
	UpdatesEnabled any     `json:"updates_enabled"`
}

func dirForType(ct string) string {
	switch strings.ToLower(ct) {
	case "mod", "mods", "pixelplaymod":
		return "mods"
	case "resourcepack", "resourcepacks":
		return "resourcepacks"
	case "shaderpack", "shaderpacks":
		return "shaderpacks"
	case "datapack", "datapacks":
		return "datapacks"
	default:
		return "mods"
	}
}

func normalizeType(ct string) string {
	switch strings.ToLower(ct) {
	case "mod", "mods":
		return "Mod"
	case "resourcepack", "resourcepacks":
		return "ResourcePack"
	case "shaderpack", "shaderpacks":
		return "ShaderPack"
	case "datapack", "datapacks":
		return "DataPack"
	case "pixelplaymod":
		return "pixelplayMod"
	default:
		return ct
	}
}

// ListLocal scans profile content folders (hashes optional).
func ListLocal(profileID, contentType string, calculateHashes bool) ([]Item, error) {
	raw, err := profiles.RawGet(profileID)
	if err != nil {
		return nil, err
	}
	var meta struct {
		Path           string `json:"path"`
		HorizonRuntime bool   `json:"horizon_runtime"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	// Author protection: hide mods listing for protected packs
	if ok, _ := protect.ProfileProtected(profileID); ok &&
		(strings.EqualFold(contentType, "Mod") || strings.EqualFold(contentType, "Mods") || strings.EqualFold(contentType, "pixelplayMod")) {
		return []Item{}, nil
	}

	root := paths.ProfileDir(meta.Path)
	sub := dirForType(contentType)
	horizonMods := meta.HorizonRuntime && (strings.EqualFold(contentType, "Mod") || strings.EqualFold(contentType, "Mods"))
	if horizonMods {
		sub = "horizon-mods"
	}
	dir := filepath.Join(root, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil
		}
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	norm := normalizeType(contentType)
	for _, e := range entries {
		name := e.Name()
		if horizonMods && !strings.HasSuffix(strings.ToLower(strings.TrimSuffix(name, ".disabled")), ".hmod") {
			continue
		}
		if name == "." || name == ".." {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		disabled := strings.HasSuffix(strings.ToLower(name), ".disabled")
		displayName := name
		if disabled {
			displayName = strings.TrimSuffix(name, ".disabled")
			if strings.HasSuffix(strings.ToLower(displayName), ".jar.disabled") {
				// already trimmed once
			}
			displayName = strings.TrimSuffix(displayName, ".disabled")
		}
		item := Item{
			Filename:    displayName,
			PathStr:     full,
			FileSize:    info.Size(),
			IsDisabled:  disabled,
			IsDirectory: e.IsDir(),
			ContentType: norm,
		}
		if calculateHashes && !e.IsDir() {
			if h, herr := fileSHA1(full); herr == nil {
				item.Sha1Hash = &h
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func fileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// InstallFile downloads a URL into the profile content folder.
func InstallFile(profileID, contentType, fileName, downloadURL string) (string, error) {
	raw, err := profiles.RawGet(profileID)
	if err != nil {
		return "", err
	}
	var meta struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	destDir := filepath.Join(paths.ProfileDir(meta.Path), dirForType(contentType))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	safe := filepath.Base(fileName)
	if safe == "" || safe == "." || safe == ".." {
		return "", fmt.Errorf("nombre de archivo inválido")
	}
	dest := filepath.Join(destDir, safe)
	return dest, nil
}

func SetEnabled(filePath string, enabled bool) error {
	in := filepath.Clean(filePath)
	parent := filepath.Dir(in)
	base := filepath.Base(in)
	trueName := strings.TrimSuffix(base, ".disabled")
	trueName = strings.TrimSuffix(trueName, ".disabled")
	enabledPath := filepath.Join(parent, trueName)
	disabledPath := filepath.Join(parent, trueName+".disabled")
	current := ""
	switch {
	case fileExists(enabledPath):
		current = enabledPath
	case fileExists(disabledPath):
		current = disabledPath
	default:
		return fmt.Errorf("archivo no encontrado: %s", in)
	}
	target := enabledPath
	if !enabled {
		target = disabledPath
	}
	if current == target {
		return nil
	}
	return os.Rename(current, target)
}

func Delete(filePath string) error {
	p := filepath.Clean(filePath)
	if err := os.RemoveAll(p); err != nil {
		alt := p + ".disabled"
		if fileExists(alt) {
			return os.RemoveAll(alt)
		}
		return err
	}
	return nil
}

func CopyInto(destDir string, srcPaths []string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, src := range srcPaths {
		if src == "" {
			continue
		}
		in, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, filepath.Base(src))
		if err := os.WriteFile(dest, in, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

type CustomMod struct {
	Filename  string `json:"filename"`
	IsEnabled bool   `json:"is_enabled"`
	Path      string `json:"path"`
}

func ListCustom(profileID string) ([]CustomMod, error) {
	dir, err := ProfileContentDir(profileID, "Mod")
	if err != nil {
		return nil, err
	}
	custom := filepath.Join(filepath.Dir(dir), "custom_mods")
	entries, err := os.ReadDir(custom)
	if err != nil {
		if os.IsNotExist(err) {
			return []CustomMod{}, nil
		}
		return nil, err
	}
	out := make([]CustomMod, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		disabled := strings.HasSuffix(strings.ToLower(name), ".disabled")
		display := strings.TrimSuffix(name, ".disabled")
		out = append(out, CustomMod{
			Filename:  display,
			IsEnabled: !disabled,
			Path:      filepath.Join(custom, name),
		})
	}
	return out, nil
}

func UpdateModrinthFile(profileID, contentType, oldName, downloadURL, newName string) error {
	dir, err := ProfileContentDir(profileID, contentType)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(dir, 0o755)
	if newName == "" {
		newName = filepath.Base(downloadURL)
	}
	dest := filepath.Join(dir, filepath.Base(newName))
	if err := downloadFile(downloadURL, dest); err != nil {
		return err
	}
	if oldName != "" && filepath.Base(oldName) != filepath.Base(newName) {
		_ = os.Remove(filepath.Join(dir, filepath.Base(oldName)))
		_ = os.Remove(filepath.Join(dir, filepath.Base(oldName)+".disabled"))
	}
	return nil
}

func downloadFile(url, dest string) error {
	return download.File(url, dest, nil)
}

func ProfileContentDir(profileID, contentType string) (string, error) {
	raw, err := profiles.RawGet(profileID)
	if err != nil {
		return "", err
	}
	var meta struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	return filepath.Join(paths.ProfileDir(meta.Path), dirForType(contentType)), nil
}
