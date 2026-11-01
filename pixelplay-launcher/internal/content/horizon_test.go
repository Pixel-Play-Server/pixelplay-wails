package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListHorizonModsRealFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", root)
	profileDir := filepath.Join(root, "profiles", "horizon-test")
	modDir := filepath.Join(profileDir, "horizon-mods")
	if err := os.MkdirAll(modDir, 0700); err != nil {
		t.Fatal(err)
	}
	profileData := `[{"id":"horizon-test","name":"Horizon 26.3","path":"horizon-test","loader":"horizon","horizon_runtime":true,"game_version":"26.3"}]`
	if err := os.WriteFile(filepath.Join(root, "profiles.json"), []byte(profileData), 0600); err != nil {
		t.Fatal(err)
	}
	for name, bytes := range map[string]string{"compass_bar.hmod": "real package bytes", "off.hmod.disabled": "disabled package", "fabric-api.jar": "must not appear"} {
		if err := os.WriteFile(filepath.Join(modDir, name), []byte(bytes), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mods, err := ListLocal("horizon-test", "Mod", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("expected 2 Horizon packages, got %+v", mods)
	}
	found := map[string]bool{}
	for _, mod := range mods {
		found[mod.Filename] = true
		if mod.ContentType != "Mod" || filepath.Dir(mod.PathStr) != modDir {
			t.Fatalf("bad item: %+v", mod)
		}
	}
	if !found["compass_bar.hmod"] || !found["off.hmod"] {
		t.Fatalf("missing packages: %+v", mods)
	}
}
