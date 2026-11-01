package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pixelplay-launcher/internal/lunar"
)

func TestNextUniquePath(t *testing.T) {
	taken := map[string]bool{
		"NeoForge 1.21.1":     true,
		"NeoForge 1.21.1 (1)": true,
	}
	got := nextUniquePath("NeoForge 1.21.1", func(s string) bool { return taken[s] })
	if got != "NeoForge 1.21.1 (2)" {
		t.Fatalf("got %q, want %q", got, "NeoForge 1.21.1 (2)")
	}

	fresh := nextUniquePath("Fresh Profile", func(string) bool { return false })
	if fresh != "Fresh Profile" {
		t.Fatalf("got %q, want unused name", fresh)
	}

	blank := nextUniquePath("  ", func(string) bool { return false })
	if blank != "profile" {
		t.Fatalf("got %q, want fallback slug", blank)
	}
}

func TestRawListMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", dir)
	if _, err := os.Stat(filepath.Join(dir, "profiles.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no profiles.json yet, err=%v", err)
	}
	raw, err := RawList()
	if err != nil {
		t.Fatalf("RawList: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 0 {
		t.Fatalf("want empty array, got %d", len(arr))
	}
	if _, err := os.Stat(filepath.Join(dir, "profiles.json")); err != nil {
		t.Fatalf("EnsureStore should create profiles.json: %v", err)
	}
}

func TestLunarCatalogUsesFabricPreservingRuntime(t *testing.T) {
	if lunar.Enabled != "private" {
		t.Skip("private Lunar integration disabled in public build")
	}
	dir := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", dir)
	input := `[{"id":"lunar","loader":"vanilla","lunar_runtime":true,"settings":{"memory":4096}},{"id":"plain","loader":"vanilla"}]`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := RawGet("lunar")
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p["loader"] != "fabric" || p["lunar_runtime"] != true || p["settings"] == nil {
		t.Fatal(p)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Loader != "fabric" || list[1].Loader != "vanilla" {
		t.Fatal(list)
	}
}

func TestLegacyLunarCatalogDoesNotOfferFabric(t *testing.T) {
	if lunar.Enabled != "private" {
		t.Skip("private Lunar integration disabled in public build")
	}
	dir := t.TempDir()
	t.Setenv("PIXELPLAY_DATA_DIR", dir)
	data := `[{"id":"legacy","game_version":"1.8.9","loader":"fabric","lunar_runtime":true}]`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Loader != "vanilla" {
		t.Fatal("legacy catalog offered external Fabric mods")
	}
}
