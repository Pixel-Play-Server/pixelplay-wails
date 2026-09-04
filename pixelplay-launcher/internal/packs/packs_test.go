package packs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSodiumUses08(t *testing.T) {
	mods := []modEntry{{
		ID:            "sodium",
		Compatibility: json.RawMessage(`{"1.21.1":{"neoforge":{"filename":"sodium-neoforge-0.8.13+mc1.21.1.jar"}}}`),
	}}
	if !sodiumUses08(mods, "1.21.1", "neoforge") {
		t.Fatal("expected Sodium 0.8")
	}
	mods[0].Compatibility = json.RawMessage(`{"1.21.1":{"neoforge":{"filename":"sodium-neoforge-0.6.13+mc1.21.1.jar"}}}`)
	if sodiumUses08(mods, "1.21.1", "neoforge") {
		t.Fatal("did not expect Sodium 0.6 to match 0.8")
	}
}

func TestDropJarsMatching(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "sodium-neoforge-0.8.13+mc1.21.1.jar")
	drop := filepath.Join(dir, "sodiumleafculling-neoforge-1.0.1-1.21.1.jar")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(drop, []byte("drop"), 0o644); err != nil {
		t.Fatal(err)
	}
	dropJarsMatching(dir, "sodiumleafculling")
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("leaf culling jar still present: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("sodium jar should remain: %v", err)
	}
}

func TestList(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("expected packs")
	}
	t.Logf("packs=%d first=%s mods=%d", len(list), list[0].DisplayName, list[0].ModCount)
}
