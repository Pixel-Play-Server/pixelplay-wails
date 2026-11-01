package profiles

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestZipHasProfileJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.ppack")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("profile.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`{"name":"test","game_version":"1.21.1","loader":"fabric"}`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if !ZipHasProfileJSON(path) {
		t.Fatal("expected profile.json detection")
	}
	if ZipHasProfileJSON(filepath.Join(dir, "missing.zip")) {
		t.Fatal("missing zip should be false")
	}
}
