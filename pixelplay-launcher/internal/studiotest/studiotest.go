package studiotest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/featured"
	"pixelplay-launcher/internal/launch"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay.gg/ppack"
)

// Run imports a Pixel Studio project directory as an isolated test pack
// and launches Minecraft against that profile (does not touch the official instance).
func Run(projectDir string) error {
	profileID, packID, err := Prepare(projectDir)
	if err != nil {
		return err
	}
	log.Printf("studio-test: pack=%s profile=%s", packID, profileID)
	emit := func(event string, payload any) {
		log.Printf("studio-test %s: %v", event, payload)
	}

	addr := ""
	if pol, err := packs.PolicyFor(packID); err == nil && pol != nil {
		addr = strings.TrimSpace(pol.AutoJoinServer)
	}
	opt := launch.Options{}
	if addr != "" {
		opt.QuickPlayMultiplayer = addr
	}
	return launch.RunBlocking(profileID, emit, nil, opt)
}

// Prepare copies the studio project into a dedicated launcher pack + profile.
func Prepare(projectDir string) (profileID, packID string, err error) {
	projectDir = filepath.Clean(projectDir)
	m, err := ppack.ReadManifestFromDir(projectDir)
	if err != nil {
		return "", "", err
	}
	if m.ID == "" {
		return "", "", fmt.Errorf("manifest sin id")
	}
	packID = "studio-test-" + m.ID
	dest := filepath.Join(paths.DataDir(), "ppacks", packID)
	if err := os.RemoveAll(dest); err != nil {
		return "", "", err
	}
	if err := copyProject(projectDir, dest); err != nil {
		return "", "", err
	}

	cloned := *m
	cloned.ID = packID
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = m.ID
	}
	cloned.DisplayName = "Test · " + name
	if strings.TrimSpace(cloned.Policy.AutoJoinServer) == "" && (m.ID == featured.PackID || strings.HasPrefix(m.ID, "pixelplay")) {
		cloned.Policy.AutoJoinServer = featured.Address
	}
	if err := packs.Upsert(packID, &cloned); err != nil {
		return "", "", err
	}

	gv := cloned.GameVersion
	if gv == "" {
		gv = "1.21.1"
	}
	loader := cloned.Loader
	if loader == "" {
		loader = "neoforge"
	}
	profilePath := "studio-test-" + m.ID
	if existing := profiles.FindByPath(profilePath); existing != nil {
		if err := profiles.Update(existing.ID, map[string]any{
			"name":                       cloned.DisplayName,
			"game_version":               gv,
			"loader":                     loader,
			"selected_pixelplay_pack_id": packID,
		}); err != nil {
			return "", "", err
		}
		_ = os.MkdirAll(paths.ProfileDir(profilePath), 0o755)
		return existing.ID, packID, nil
	}
	id, err := profiles.Create(map[string]any{
		"name":                       cloned.DisplayName,
		"path":                       profilePath,
		"game_version":               gv,
		"loader":                     loader,
		"selected_pixelplay_pack_id": packID,
	})
	if err != nil {
		return "", "", err
	}
	return id, packID, nil
}

// PublishToPixelCloud exports a studio project as .ppack, generates version.json with sha256/size, and uploads to panel.pixelplay.gg cloud.
func PublishToPixelCloud(projectDir string) (string, error) {
	projectDir = filepath.Clean(projectDir)
	m, err := ppack.ReadManifestFromDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("error leyendo manifest: %w", err)
	}
	if m.ID == "" {
		return "", fmt.Errorf("manifest sin id de modpack")
	}

	ver := m.PackVersion
	if ver == "" {
		ver = "0.1.0"
	}

	exportsDir := filepath.Join(projectDir, "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return "", err
	}

	ppackPath := filepath.Join(exportsDir, fmt.Sprintf("%s-%s.ppack", m.ID, ver))
	if err := ppack.WriteZip(projectDir, ppackPath); err != nil {
		return "", fmt.Errorf("error empacando .ppack: %w", err)
	}

	st, err := os.Stat(ppackPath)
	if err != nil {
		return "", err
	}

	f, err := os.Open(ppackPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	shaHex := hex.EncodeToString(hash.Sum(nil))

	downloadURL := fmt.Sprintf("https://panel.pixelplay.gg/dl/pixelplay/modpacks/%s/%s-%s.ppack", m.ID, m.ID, ver)
	verJSON := map[string]any{
		"packId":      m.ID,
		"version":     ver,
		"sha256":      shaHex,
		"size":        st.Size(),
		"downloadUrl": downloadURL,
	}

	verBytes, err := json.MarshalIndent(verJSON, "", "  ")
	if err != nil {
		return "", err
	}
	verPath := filepath.Join(exportsDir, "version.json")
	if err := os.WriteFile(verPath, verBytes, 0o644); err != nil {
		return "", err
	}

	return fmt.Sprintf("Pack %s v%s empaquetado con éxito (SHA256: %s, Tamaño: %d bytes). Listo en %s", m.ID, ver, shaHex[:8], st.Size(), verPath), nil
}

func copyProject(src, dst string) error {
	skip := map[string]bool{"exports": true, ".git": true, "node_modules": true}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		if skip[top] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
