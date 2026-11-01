package horizon

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createMod(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.hmod")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestInstallValidMod(t *testing.T) {
	dir := t.TempDir()
	src := createMod(t, map[string]string{"horizon-mod.toml": "id = \"test-mod\"\nname = \"Test\"\n", "mod.wasm": "\\x00asm"})
	id, err := InstallMod(dir, src)
	if err != nil {
		t.Fatal(err)
	}
	if id != "test-mod" {
		t.Fatalf("unexpected id: %s", id)
	}
	if _, err := os.Stat(filepath.Join(dir, "horizon-mods", "test-mod.hmod")); err != nil {
		t.Fatal(err)
	}
	if _, err = InstallMod(dir, src); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestInstallRejectsMissingManifest(t *testing.T) {
	src := createMod(t, map[string]string{"mod.wasm": "some bytes"})
	if _, err := InstallMod(t.TempDir(), src); err == nil {
		t.Fatal("missing manifest accepted")
	}
}
func TestInstallRejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{"../escape", "/absolute", "C:/drive", "dir/../../escape"} {
		t.Run(path, func(t *testing.T) {
			src := createMod(t, map[string]string{"horizon-mod.toml": "id = \"mod\"\n", "mod.wasm": "wasm", path: "bad"})
			if _, err := InstallMod(t.TempDir(), src); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
