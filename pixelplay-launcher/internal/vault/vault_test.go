package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripHidesNameAndZipMagic(t *testing.T) {
	dir := t.TempDir()
	plain := []byte("PK\x03\x04FAKEJAR-Mekanism-secret")
	if err := Put(dir, "Mekanism-1.21.1.jar", "pack-a", plain); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, Ext) {
		t.Fatalf("blob name %s", name)
	}
	if strings.Contains(strings.ToLower(name), "mekanism") {
		t.Fatalf("original name leaked in filename: %s", name)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, name))
	if bytes.Contains(raw, []byte("Mekanism")) || bytes.Contains(raw, []byte("PK\x03\x04")) {
		t.Fatal("plaintext leaked inside sealed blob")
	}
	if err := UnsealTree(dir, "pack-a"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "Mekanism-1.21.1.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("roundtrip mismatch")
	}
	if err := UnsealTree(dir, "other-pack"); err != nil {
		t.Fatal(err)
	}
	// wrong key must not overwrite with garbage; original still present
	got2, _ := os.ReadFile(filepath.Join(dir, "Mekanism-1.21.1.jar"))
	if !bytes.Equal(got2, plain) {
		t.Fatal("wrong key should not replace plaintext")
	}
}

func TestUnsealToOtherDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mods := filepath.Join(src, "mods")
	plain := []byte("PK\x03\x04secret-jar")
	if err := Put(mods, "Foo.jar", "pack-c", plain); err != nil {
		t.Fatal(err)
	}
	if err := UnsealTo(src, dst, "pack-c"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "mods", "Foo.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("unseal-to mismatch")
	}
	if _, err := os.Stat(filepath.Join(src, "mods", "Foo.jar")); !os.IsNotExist(err) {
		t.Fatal("vault should stay sealed (no plaintext jar)")
	}
}

func TestSealTreeNestedConfig(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "Mekanism", "common.toml")
	_ = os.MkdirAll(filepath.Dir(nested), 0o700)
	if err := os.WriteFile(nested, []byte("secret-config-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SealTree(dir, "pack-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatal("plaintext should be gone after seal")
	}
	if err := UnsealTree(dir, "pack-b"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(nested)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret-config-value" {
		t.Fatalf("got %q", got)
	}
}

func TestRuntimeDirShred(t *testing.T) {
	dir, err := NewRuntimeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.bin"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ShredDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("runtime dir should be gone")
	}
}
