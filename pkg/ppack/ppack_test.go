package ppack

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifestFromZipWindowsBackslash(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(`folder\manifest.json`) // literal backslash entry name
	if err != nil {
		t.Fatal(err)
	}
	m := NewManifest("winpack", "Win Pack")
	data := BytesManifest(m)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "win.ppack")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifestFromZip(path)
	if err != nil {
		t.Fatalf("ReadManifestFromZip: %v", err)
	}
	if got.ID != "winpack" {
		t.Fatalf("id=%q", got.ID)
	}
}

func TestReadManifestFromZipCaseInsensitive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("Manifest.JSON")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManifest("casepack", "Case")
	if _, err := w.Write(BytesManifest(m)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "case.ppack")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifestFromZip(path); err != nil {
		t.Fatal(err)
	}
}

func TestWriteZipSkipsExports(t *testing.T) {
	dir := t.TempDir()
	m := NewManifest("tpack", "Test")
	if err := SaveManifest(filepath.Join(dir, ManifestName), &m); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "overrides", "config"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "overrides", "config", "a.toml"), []byte("x=1\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "exports"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "exports", "old.ppack"), []byte("not-a-zip"), 0o644)

	dest := filepath.Join(dir, "exports", "out.ppack")
	if err := WriteZip(dir, dest); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.ToSlash(f.Name) == "exports/old.ppack" || filepath.ToSlash(f.Name) == "exports/out.ppack" {
			t.Fatalf("exports leaked into zip: %s", f.Name)
		}
	}
}
