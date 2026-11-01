package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir is the shared launcher data root (same as Tauri pixelplayclientv3).
func DataDir() string {
	if override := os.Getenv("PIXELPLAY_DATA_DIR"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "pixelplayclientv3")
	}
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "pixelplay", "pixelplayClientV3")
		}
		return filepath.Join(home, "AppData", "Roaming", "pixelplay", "pixelplayClientV3")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "gg.pixelplay.pixelplayClientV3")
	default:
		return filepath.Join(home, ".local", "share", "pixelplayclientv3")
	}
}

func ProfilesFile() string { return filepath.Join(DataDir(), "profiles.json") }
func PacksFile() string    { return filepath.Join(DataDir(), "pixelplay_modpacks.json") }
func ConfigFile() string   { return filepath.Join(DataDir(), "launcher_config.json") }
func AccountsFile() string { return filepath.Join(DataDir(), "accounts.json") }
func SkinsFile() string    { return filepath.Join(DataDir(), "skins.json") }

// EnsureLayout creates the shared data directory tree used by a fresh install.
func EnsureLayout() error {
	root := DataDir()
	for _, d := range []string{
		root,
		filepath.Join(root, "profiles"),
		filepath.Join(root, "meta"),
		filepath.Join(root, "ppacks"),
		filepath.Join(root, "updates"),
		filepath.Join(root, "protected"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	defaults := map[string]string{
		ProfilesFile(): "[]\n",
		ConfigFile():   "{}\n",
		AccountsFile(): "{\n  \"accounts\": [],\n  \"token\": null\n}\n",
		SkinsFile():    "[]\n",
		PacksFile():    "{\n  \"packs\": {},\n  \"repositories\": {}\n}\n",
	}
	for path, body := range defaults {
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ProfileDir(profilePath string) string {
	return filepath.Join(DataDir(), "profiles", profilePath)
}

// ProtectedRoot holds author-protected client data (resource packs, capture agent).
func ProtectedRoot() string {
	return filepath.Join(DataDir(), "protected")
}

func ProtectedPackDir(packID string) string {
	return filepath.Join(ProtectedRoot(), packID)
}

// VaultDir is the encrypted-at-rest store for a protected pack.
func VaultDir(packID string) string {
	return filepath.Join(ProtectedPackDir(packID), "v")
}
