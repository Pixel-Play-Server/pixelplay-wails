package mcmeta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/paths"
)

// EnsureVanillaLauncherLayout prepares meta/ like the official Minecraft launcher
// so NeoForge/Forge `--installClient` succeeds. Without launcher_profiles.json the
// installer fails with: "There is no minecraft launcher profile in …".
func EnsureVanillaLauncherLayout(mcVersion string, emit Progress) error {
	mcVersion = stringsTrim(mcVersion)
	if mcVersion == "" {
		return fmt.Errorf("versión Minecraft vacía")
	}
	mcRoot := filepath.Join(paths.DataDir(), "meta")
	verDir := filepath.Join(mcRoot, "versions", mcVersion)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Join(mcRoot, "libraries"), 0o755)

	if err := ensureLauncherProfiles(mcRoot, mcVersion); err != nil {
		return err
	}

	man, err := FetchManifest()
	if err != nil {
		return err
	}
	var verURL string
	for _, v := range man.Versions {
		if v.ID == mcVersion {
			verURL = v.URL
			break
		}
	}
	if verURL == "" {
		return fmt.Errorf("versión Minecraft no encontrada: %s", mcVersion)
	}

	vjsonPath := filepath.Join(verDir, mcVersion+".json")
	if st, err := os.Stat(vjsonPath); err != nil || st.Size() < 64 {
		if emit != nil {
			emit("Preparando versión vanilla " + mcVersion + "…")
		}
		raw, err := httpx.Get(verURL)
		if err != nil {
			return fmt.Errorf("descargar %s.json: %w", mcVersion, err)
		}
		if !json.Valid(raw) {
			return fmt.Errorf("%s.json inválido desde Mojang", mcVersion)
		}
		if err := os.WriteFile(vjsonPath, raw, 0o644); err != nil {
			return err
		}
	}

	var meta PistonMeta
	b, err := os.ReadFile(vjsonPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return err
	}
	if emit != nil {
		emit("Descargando cliente vanilla " + mcVersion + "…")
	}
	if err := DownloadClient(&meta); err != nil {
		return err
	}
	// Libraries needed by the installer processors (bundler extract, etc.).
	if _, err := DownloadLibraries(&meta, nil, emit); err != nil {
		return fmt.Errorf("libraries vanilla: %w", err)
	}
	return nil
}

func ensureLauncherProfiles(mcRoot, mcVersion string) error {
	path := filepath.Join(mcRoot, "launcher_profiles.json")
	if st, err := os.Stat(path); err == nil && st.Size() > 8 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	doc := map[string]any{
		"profiles": map[string]any{
			"pixelplay": map[string]any{
				"created":       now,
				"icon":          "Furnace",
				"lastUsed":      now,
				"lastVersionId": mcVersion,
				"name":          "Pixel Play",
				"type":          "custom",
			},
		},
		"settings": map[string]any{
			"enableSnapshots":   false,
			"keepLauncherOpen":  false,
			"showGameLog":       false,
			"showMenu":          true,
			"soundOn":           false,
		},
		"version": 3,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
