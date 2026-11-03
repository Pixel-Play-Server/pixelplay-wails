package content

import (
	"archive/zip"
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

func TestHorizonToggleAndInstalledStatus(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", root)
	dir := filepath.Join(root, "profiles", "horizon-test", "horizon-mods")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles.json"), []byte(`[{"id":"horizon-test","path":"horizon-test","loader":"horizon"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compass.hmod")
	if err := os.WriteFile(path, []byte("test package"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false, true} {
		if err := Toggle("horizon-test", path, "", "Mod", enabled); err != nil {
			t.Fatal(err)
		}
		status, err := CheckInstalled([]byte(`{"profile_id":"horizon-test","project_id":"compass","project_type":"Mod"}`), false)
		if err != nil {
			t.Fatal(err)
		}
		s := status.(map[string]any)
		if s["is_installed"] != true || s["is_enabled"] != enabled {
			t.Fatalf("wrong persisted status: %+v", s)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("toggle renamed native package")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.hmod")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall("horizon-test", outside, "", "Mod"); err == nil {
		t.Fatal("allowed deletion outside profile")
	}
	if err := Uninstall("horizon-test", path, "", "Mod"); err != nil {
		t.Fatal(err)
	}
	status, err := CheckInstalled([]byte(`{"profile_id":"horizon-test","project_id":"compass"}`), false)
	if err != nil || status.(map[string]any)["is_installed"] != false {
		t.Fatalf("removed mod still installed: %+v %v", status, err)
	}
}

func TestHorizonInstalledVersionUsesManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.hmod")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("horizon-mod.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("id = \"compass\"\nversion = \"1.2.0\"\n")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if !nativeVersionMatches(path, "compass@1.2.0") {
		t.Fatal("installed version not recognized")
	}
	if nativeVersionMatches(path, "compass@1.3.0") {
		t.Fatal("wrong version reported installed")
	}
}
