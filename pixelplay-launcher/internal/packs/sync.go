package packs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/mcmeta"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/vault"
	"pixelplay.gg/ppack"
)

type modEntry struct {
	ID            string          `json:"id"`
	DisplayName   string          `json:"displayName"`
	Source        json.RawMessage `json:"source"`
	Compatibility json.RawMessage `json:"compatibility"`
}

type sourceObj struct {
	Type          string `json:"type"`
	ProjectID     string `json:"projectId"`
	ProjectSlug   string `json:"projectSlug"`
	RepositoryRef string `json:"repositoryRef"`
	GroupID       string `json:"groupId"`
	ArtifactID    string `json:"artifactId"`
}

type compatTarget struct {
	Identifier string `json:"identifier"`
	Filename   string `json:"filename"`
}

// SyncPackMods downloads pack mods into cache and copies them into the profile mods folder.
func SyncPackMods(packID, mcVersion, loader, profileModsDir string, skip map[string]bool, emit func(string)) error {
	cfg, err := loadFile()
	if err != nil {
		return err
	}
	pack, ok := cfg.Packs[packID]
	if !ok {
		return fmt.Errorf("pack desconocido: %s", packID)
	}
	var mods []modEntry
	if len(pack.Mods) > 0 {
		if err := json.Unmarshal(pack.Mods, &mods); err != nil {
			return err
		}
	}
	mods = append(mods, missingLibraries(mods)...)
	if skip == nil {
		skip = map[string]bool{}
	}
	if sodiumUses08(mods, mcVersion, loader) {
		// Sodium 0.8 renamed SodiumGameOptions; Leaf Culling 1.0.1 mixins crash on world join.
		skip["sodiumleafculling"] = true
	}
	repos := map[string]string{}
	if len(cfg.Repositories) > 0 {
		_ = json.Unmarshal(cfg.Repositories, &repos)
	}
	cache := mcmeta.ModCacheDir()
	_ = os.MkdirAll(cache, 0o755)
	_ = os.MkdirAll(profileModsDir, 0o755)

	loader = strings.ToLower(loader)
	copied := 0
	protected, _ := IsProtectedByAuthor(packID)
	for _, m := range mods {
		target, ok := compatFor(m.Compatibility, mcVersion, loader)
		if !ok {
			continue
		}
		var src sourceObj
		_ = json.Unmarshal(m.Source, &src)
		filename := target.Filename
		if filename == "" {
			filename = m.ID + ".jar"
		}
		out := filepath.Join(profileModsDir, filename)
		if skip[m.ID] {
			_ = os.Remove(out)
			vault.Remove(profileModsDir, filename, packID)
			continue
		}
		destCache := filepath.Join(cache, filename)
		if _, err := os.Stat(destCache); err != nil {
			if emit != nil {
				emit("Mod: " + filename)
			}
			if err := downloadMod(src, target, repos, destCache); err != nil {
				return fmt.Errorf("%s: %w", m.ID, err)
			}
		}
		in, err := os.ReadFile(destCache)
		if err != nil {
			return err
		}
		if protected {
			if err := vault.Put(profileModsDir, filename, packID, in); err != nil {
				return err
			}
		} else if err := os.WriteFile(out, in, 0o644); err != nil {
			return err
		}
		copied++
	}
	if emit != nil {
		emit(fmt.Sprintf("Mods del pack: %d", copied))
	}
	// Always prune leftovers (e.g. NeoForge jars after switching the pack to Fabric).
	keep := map[string]bool{}
	for _, m := range mods {
		if skip[m.ID] {
			continue
		}
		target, ok := compatFor(m.Compatibility, mcVersion, loader)
		if !ok {
			continue
		}
		filename := target.Filename
		if filename == "" {
			filename = m.ID + ".jar"
		}
		keep[filename] = true
		keep[filename+".disabled"] = true
	}
	pruneUnlistedJars(profileModsDir, packID, keep)
	if skip["sodiumleafculling"] {
		dropJarsMatching(profileModsDir, "sodiumleafculling")
	}
	return ApplyDefaultPacks(packID, filepath.Dir(profileModsDir))
}

func missingLibraries(mods []modEntry) []modEntry {
	have := map[string]bool{}
	for _, m := range mods {
		id := strings.ToLower(m.ID)
		have[id] = true
		have[strings.ReplaceAll(id, "_", "-")] = true
	}
	var extra []modEntry
	if (have["particle-core"] || have["particle_core"]) && !have["fzzy-config"] && !have["fzzy_config"] {
		extra = append(extra, modEntry{
			ID:          "fzzy-config",
			DisplayName: "Fzzy Config 0.7.6",
			Source:      json.RawMessage(`{"type":"modrinth","projectId":"hYykXjDp","projectSlug":"fzzy-config"}`),
			Compatibility: json.RawMessage(`{"1.21.1":{"neoforge":{"identifier":"MAPG6cXE","filename":"fzzy_config-0.7.6+1.21+neoforge.jar"}}}`),
		})
	}
	return extra
}

func sodiumUses08(mods []modEntry, mc, loader string) bool {
	for _, m := range mods {
		id := strings.ToLower(strings.ReplaceAll(m.ID, "_", "-"))
		if id != "sodium" {
			continue
		}
		t, ok := compatFor(m.Compatibility, mc, loader)
		if ok && strings.Contains(t.Filename, "0.8.") {
			return true
		}
	}
	return false
}

func dropJarsMatching(dir, needle string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	n := strings.ToLower(needle)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(e.Name()), n) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// pruneUnlistedJars drops jars (and their vault blobs) that are no longer in the pack.
// Protected packs must not keep leftovers from Mod Menu drag-drop or removed mods.
func pruneUnlistedJars(modsDir, packID string, keep map[string]bool) {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return
	}
	keepBlobs := map[string]bool{}
	for name := range keep {
		base := strings.TrimSuffix(name, ".disabled")
		keepBlobs[vault.ObjectName(packID, base)] = true
		keepBlobs[vault.ObjectName(packID, name)] = true
		// SealFrom may store under "mods/<file>" relative paths.
		keepBlobs[vault.ObjectName(packID, "mods/"+base)] = true
		keepBlobs[vault.ObjectName(packID, "mods/"+name)] = true
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		low := strings.ToLower(name)
		if strings.HasSuffix(low, vault.Ext) {
			if keepBlobs[name] {
				continue
			}
			_ = os.Remove(filepath.Join(modsDir, name))
			continue
		}
		if !strings.HasSuffix(low, ".jar") && !strings.HasSuffix(low, ".jar.disabled") {
			continue
		}
		if keep[name] {
			continue
		}
		vault.Remove(modsDir, name, packID)
		vault.Remove(filepath.Dir(modsDir), "mods/"+name, packID)
	}
	// Known filenames that may only exist as hashed vault blobs.
	for name := range extraKnownRemovals() {
		if keep[name] {
			continue
		}
		vault.Remove(modsDir, name, packID)
		vault.Remove(filepath.Dir(modsDir), "mods/"+name, packID)
	}
}

func extraKnownRemovals() map[string]bool {
	return map[string]bool{
		"toms_storage-1.21-2.4.2.jar":                    true,
		"createcontraptionterminals-1.21-1.4.0.jar":      true,
		"createaddition-1.7.0.jar":                       true,
		"create-new-age-1.2.0+neoforge-mc1.21.1.jar":     true,
	}
}

func compatFor(raw json.RawMessage, mc, loader string) (compatTarget, bool) {
	var tree map[string]map[string]compatTarget
	if json.Unmarshal(raw, &tree) != nil {
		return compatTarget{}, false
	}
	if loaders, ok := tree[mc]; ok {
		if t, ok := loaders[loader]; ok {
			return t, true
		}
	}
	return compatTarget{}, false
}

func downloadMod(src sourceObj, target compatTarget, repos map[string]string, dest string) error {
	typ := strings.ToLower(src.Type)
	switch typ {
	case "modrinth":
		slug := src.ProjectSlug
		if slug == "" {
			slug = src.ProjectID
		}
		url := fmt.Sprintf("https://api.modrinth.com/maven/maven/modrinth/%s/%s/%s", slug, target.Identifier, filepath.Base(dest))
		if err := download.File(url, dest, nil); err != nil {
			// fallback: version API
			return downloadModrinthVersion(target.Identifier, dest)
		}
		return nil
	case "url":
		return download.File(target.Identifier, dest, nil)
	case "maven":
		repo := strings.TrimRight(repos[src.RepositoryRef], "/")
		if repo == "" {
			repo = "https://maven.fabricmc.net"
		}
		g := strings.ReplaceAll(src.GroupID, ".", "/")
		name := filepath.Base(dest)
		url := fmt.Sprintf("%s/%s/%s/%s/%s", repo, g, src.ArtifactID, target.Identifier, name)
		return download.File(url, dest, nil)
	default:
		if strings.HasPrefix(target.Identifier, "http") {
			return download.File(target.Identifier, dest, nil)
		}
		return fmt.Errorf("source %s no soportado", src.Type)
	}
}

func downloadModrinthVersion(versionID, dest string) error {
	var ver struct {
		Files []struct {
			URL      string `json:"url"`
			Filename string `json:"filename"`
			Primary  bool   `json:"primary"`
		} `json:"files"`
	}
	if err := httpx.GetJSON("https://api.modrinth.com/v2/version/"+versionID, &ver); err != nil {
		return err
	}
	url := ""
	for _, f := range ver.Files {
		if f.Primary {
			url = f.URL
			break
		}
	}
	if url == "" && len(ver.Files) > 0 {
		url = ver.Files[0].URL
	}
	if url == "" {
		return fmt.Errorf("versión Modrinth %s sin archivos", versionID)
	}
	return download.File(url, dest, nil)
}

// ApplyDefaultPacks copies default resource/shader packs from extracted .ppack if present.
func ApplyDefaultPacks(packID, profileDir string) error {
	pol, err := PolicyFor(packID)
	if err != nil || pol == nil {
		return nil
	}
	srcRoot := filepath.Join(paths.DataDir(), "ppacks", packID)
	copyNamed := func(names []string, folder string) {
		if len(names) == 0 {
			return
		}
		dst := filepath.Join(profileDir, folder)
		_ = os.MkdirAll(dst, 0o755)
		for _, n := range names {
			for _, cand := range []string{
				filepath.Join(srcRoot, folder, n),
				filepath.Join(srcRoot, n),
			} {
				if st, err := os.Stat(cand); err == nil && !st.IsDir() {
					b, _ := os.ReadFile(cand)
					_ = os.WriteFile(filepath.Join(dst, filepath.Base(n)), b, 0o644)
				}
			}
		}
	}
	copyNamed(pol.DefaultResourcePacks, "resourcepacks")
	copyNamed(pol.DefaultShaderPacks, "shaderpacks")
	_ = ApplyOverrides(packID, profileDir)
	return nil
}

func ExtractImported(m *ppack.Manifest, zipPath string) error {
	dir := filepath.Join(paths.DataDir(), "ppacks", m.ID)
	_ = os.MkdirAll(dir, 0o755)
	return ppack.ExtractZip(zipPath, dir)
}
