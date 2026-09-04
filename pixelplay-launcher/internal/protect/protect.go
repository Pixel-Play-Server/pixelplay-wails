package protect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
)

// ErrAuthorProtected is returned when a filesystem/UI action is blocked.
var ErrAuthorProtected = fmt.Errorf("author protection: action blocked for this pack")

// ProfileProtected reports whether the profile's selected pack locks mods.
func ProfileProtected(profileID string) (bool, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return false, err
	}
	if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID == "" {
		return false, nil
	}
	return packs.IsProtectedByAuthor(*p.SelectedPixelplayPackID)
}

// AssertProfileFolderOpenable blocks opening the whole profile dir when protected.
func AssertProfileFolderOpenable(profileID string) error {
	p, err := profiles.Get(profileID)
	if err != nil {
		return err
	}
	if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID == "" {
		return nil
	}
	pol, err := packs.PolicyFor(*p.SelectedPixelplayPackID)
	if err != nil {
		return err
	}
	if pol != nil && pol.AllowOpenFolder {
		return nil
	}
	ok, err := packs.IsProtectedByAuthor(*p.SelectedPixelplayPackID)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("%w: cannot open profile folder", ErrAuthorProtected)
	}
	return nil
}

// AssertPathOpenable blocks revealing paths under a protected profile's mods/.
func AssertPathOpenable(absOrRel string) error {
	candidate, err := filepath.Abs(absOrRel)
	if err != nil {
		candidate = absOrRel
	}
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = resolved
	}

	all, err := profiles.List()
	if err != nil {
		return err
	}
	for _, p := range all {
		if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID == "" {
			continue
		}
		protected, err := packs.IsProtectedByAuthor(*p.SelectedPixelplayPackID)
		if err != nil || !protected {
			continue
		}
		modsDir := filepath.Join(paths.ProfileDir(p.Path), "mods")
		if resolved, err := filepath.EvalSymlinks(modsDir); err == nil {
			modsDir = resolved
		} else {
			modsDir, _ = filepath.Abs(modsDir)
		}
		if candidate == modsDir || strings.HasPrefix(candidate, modsDir+string(os.PathSeparator)) {
			return fmt.Errorf("%w: mods folder access is blocked", ErrAuthorProtected)
		}
	}
	return nil
}
