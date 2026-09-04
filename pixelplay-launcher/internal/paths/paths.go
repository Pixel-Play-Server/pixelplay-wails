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

func ProfileDir(profilePath string) string {
	return filepath.Join(DataDir(), "profiles", profilePath)
}
