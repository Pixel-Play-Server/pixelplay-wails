package mrpack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestReadIndexAndImport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", dir)

	pack := filepath.Join(dir, "sample.mrpack")
	if err := writeSampleMrpack(pack); err != nil {
		t.Fatal(err)
	}
	id, err := ImportFile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty profile id")
	}
	mods := filepath.Join(dir, "profiles")
	entries, _ := os.ReadDir(mods)
	if len(entries) == 0 {
		t.Fatal("expected profile folder")
	}
}

func writeSampleMrpack(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("modrinth.index.json")
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(`{
  "formatVersion": 1,
  "game": "minecraft",
  "versionId": "1.0.0",
  "name": "Sample Pack",
  "files": [],
  "dependencies": {
    "minecraft": "1.21.1",
    "neoforge": "21.1.248"
  }
}`))
	if err != nil {
		return err
	}
	ow, err := zw.Create("overrides/options.txt")
	if err != nil {
		return err
	}
	_, _ = ow.Write([]byte("fov:70\n"))
	return zw.Close()
}
