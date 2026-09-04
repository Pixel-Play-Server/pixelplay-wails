package protect_test

import (
	"path/filepath"
	"testing"

	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
)

func TestAuthorProtectedProfileFolderBlocked(t *testing.T) {
	all, err := profiles.List()
	if err != nil {
		t.Skip("no profiles.json:", err)
	}
	var protectedID string
	for _, p := range all {
		ok, err := protect.ProfileProtected(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			protectedID = p.ID
			break
		}
	}
	if protectedID == "" {
		t.Skip("no author-protected profile in data dir")
	}
	if err := protect.AssertProfileFolderOpenable(protectedID); err == nil {
		t.Fatal("expected folder open to be blocked")
	}
	p, _ := profiles.Get(protectedID)
	mods := filepath.Join(paths.ProfileDir(p.Path), "mods")
	if err := protect.AssertPathOpenable(mods); err == nil {
		t.Fatal("expected mods path to be blocked")
	}
}
