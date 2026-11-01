package packs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
)

func skipUserData(rel string) bool {
	rel = filepath.ToSlash(rel)
	base := strings.ToLower(filepath.Base(rel))
	switch base {
	case "options.txt", "optionsof.txt", "servers.dat", "usercache.json", "usernamecache.json":
		return true
	}
	for _, part := range []string{"/saves/", "/playerdata/", "/stats/", "/advancements/", "/screenshots/", "/replay_recordings/"} {
		if strings.Contains("/"+rel+"/", part) {
			return true
		}
	}
	return false
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skipUserData(rel) {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// ApplyOverrides copies config/kubejs/defaultconfigs from the extracted .ppack into a profile.
func ApplyOverrides(packID, profileDir string) error {
	if packID == "" || profileDir == "" {
		return nil
	}
	srcRoot := filepath.Join(paths.DataDir(), "ppacks", packID)
	for _, folder := range []string{"config", "kubejs", "defaultconfigs"} {
		src := filepath.Join(srcRoot, "overrides", folder)
		if _, err := os.Stat(src); err != nil {
			src = filepath.Join(srcRoot, folder)
		}
		if st, err := os.Stat(src); err == nil && st.IsDir() {
			if err := copyTree(src, filepath.Join(profileDir, folder)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyOverridesToPackProfiles applies extracted overrides to every profile using packID.
func ApplyOverridesToPackProfiles(packID string) error {
	list, err := profiles.List()
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID != packID {
			continue
		}
		dir, err := profiles.OpenFolder(p.ID)
		if err != nil {
			continue
		}
		_ = ApplyDefaultPacks(packID, filepath.Dir(filepath.Join(dir, "mods")))
		_ = ApplyOverrides(packID, dir)
	}
	return nil
}
