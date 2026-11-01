package updater

import "testing"

func TestPrivateBuildDoesNotCheckPublicFeed(t *testing.T) {
	old := PrivateBuild
	PrivateBuild = "true"
	defer func() { PrivateBuild = old }()
	t.Setenv("PIXELPLAY_UPDATER_BASE_URL", "http://127.0.0.1:1")
	m, newer, e := Check("6.0.2-lunar-private.1", nil)
	if e != nil || m != nil || newer {
		t.Fatalf("private updater: %v %v %v", m, newer, e)
	}
}
