package horizon

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryKinds(t *testing.T) {
	for _, tc := range []struct {
		kind, name string
		want       bool
	}{
		{"mod", "x.hmod", true}, {"native-mod", "x.hnmod", true}, {"resourcepack", "x.zip", true},
		{"mod", "x.jar", false}, {"native-mod", "x.exe", false}, {"resourcepack", "x.hnmod", false},
	} {
		v := RegistryVersion{Game: "26.3", File: RegistryFile{Name: tc.name}}
		if registryCompatible(tc.kind, v) != tc.want {
			t.Errorf("%s %s", tc.kind, tc.name)
		}
		v.Game = "1.21.11"
		if registryCompatible(tc.kind, v) {
			t.Fatal("wrong game accepted")
		}
	}
}
func TestRegistryPack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.zip")
	f, _ := os.Create(src)
	z := zip.NewWriter(f)
	w, _ := z.Create("pack.mcmeta")
	w.Write([]byte(`{"pack":{"pack_format":97,"description":"Demo"}}`))
	z.Close()
	f.Close()
	if _, err := installRegistryPack(dir, src, "panorama", "1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "resourcepacks", "panorama-1.0.zip")); err != nil {
		t.Fatal(err)
	}
	if _, err := installRegistryPack(dir, src, "panorama", "../../escape"); err == nil {
		t.Fatal("unsafe version accepted")
	}
	if _, err := installRegistryPack(dir, src, "panorama", "1.0"); err == nil {
		t.Fatal("overwrote pack")
	}
}
