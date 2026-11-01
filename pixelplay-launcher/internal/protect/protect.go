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

func blocked(msg string) error {
	return fmt.Errorf("%w: %s", ErrAuthorProtected, msg)
}

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

func folderOpenAllowed(packID string) (bool, error) {
	pol, err := packs.PolicyFor(packID)
	if err != nil {
		return false, err
	}
	if pol != nil && pol.AllowOpenFolder {
		return true, nil
	}
	ok, err := packs.IsProtectedByAuthor(packID)
	if err != nil {
		return false, err
	}
	return !ok, nil
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
	ok, err := folderOpenAllowed(*p.SelectedPixelplayPackID)
	if err != nil {
		return err
	}
	if !ok {
		return blocked("cannot open profile folder")
	}
	return nil
}

func under(candidate, root string) bool {
	if root == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	} else {
		root, _ = filepath.Abs(root)
	}
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = resolved
	}
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if candidate == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(candidate, root+sep)
}

// AssertPathOpenable blocks revealing any path that belongs to a protected client.
func AssertPathOpenable(absOrRel string) error {
	candidate, err := filepath.Abs(absOrRel)
	if err != nil {
		candidate = absOrRel
	}

	all, err := profiles.List()
	if err != nil {
		return err
	}
	for _, p := range all {
		if p.SelectedPixelplayPackID == nil || *p.SelectedPixelplayPackID == "" {
			continue
		}
		allowed, err := folderOpenAllowed(*p.SelectedPixelplayPackID)
		if err != nil || allowed {
			continue
		}
		if under(candidate, paths.ProfileDir(p.Path)) {
			return blocked("client folder access is blocked")
		}
		if under(candidate, paths.ProtectedPackDir(*p.SelectedPixelplayPackID)) {
			return blocked("protected pack storage is blocked")
		}
		if under(candidate, paths.VaultDir(*p.SelectedPixelplayPackID)) {
			return blocked("protected vault is blocked")
		}
		if under(candidate, filepath.Join(paths.DataDir(), "ppacks", *p.SelectedPixelplayPackID)) {
			return blocked("pack files access is blocked")
		}
	}
	if under(candidate, paths.ProtectedRoot()) {
		return blocked("protected storage is blocked")
	}
	return nil
}
