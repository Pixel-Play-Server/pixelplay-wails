package horizon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModStatePreservesSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "horizon-mods.txt")
	if err := os.WriteFile(path, []byte("key.zoom:67\nmod.compass:false\nmod.other:true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if ModEnabled(dir, "compass") {
		t.Fatal("disabled mod reported enabled")
	}
	for _, enabled := range []bool{true, false, true} {
		if err := SetModEnabled(dir, "compass", enabled); err != nil {
			t.Fatal(err)
		}
		if ModEnabled(dir, "compass") != enabled {
			t.Fatal("state did not persist")
		}
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "mod.compass:") != 1 || !strings.Contains(string(data), "key.zoom:67") || !ModEnabled(dir, "other") {
		t.Fatalf("damaged settings: %s", data)
	}
	if err := SetModEnabled(dir, "../bad\nmod.other", true); err == nil {
		t.Fatal("accepted invalid id")
	}
}
