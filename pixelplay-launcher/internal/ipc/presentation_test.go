package ipc

import (
	"encoding/json"
	"pixelplay-launcher/internal/profiles"
	"testing"
)

func TestEmptyLibraryDoesNotInstallPromotedProfiles(t *testing.T) {
	t.Setenv("PIXELPLAY_DATA_DIR", t.TempDir())
	out, err := Dispatch(nil, "get_all_profiles_and_last_played", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.(map[string]any)["all_profiles"].([]json.RawMessage)) != 0 {
		t.Fatal("unexpected seeded profile")
	}
	list, err := profiles.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("library was modified: %+v %v", list, err)
	}
	out, err = Dispatch(nil, "get_featured_server", "{}")
	if err != nil || out != nil {
		t.Fatal("promoted server still exposed")
	}
}
func TestCustomCapeActionsAreUnavailable(t *testing.T) {
	for _, cmd := range []string{"browse_capes", "upload_cape", "equip_cape", "delete_cape", "ensure_featured_server_profile"} {
		if _, err := Dispatch(nil, cmd, "{}"); err == nil {
			t.Fatalf("%s unexpectedly enabled", cmd)
		}
	}
}
