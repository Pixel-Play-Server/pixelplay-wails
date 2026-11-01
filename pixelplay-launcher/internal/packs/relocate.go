package packs

import (
	"os"
	"path/filepath"

	"pixelplay-launcher/internal/paths"
)

// RelocateProtectedMedia moves resourcepacks/shaderpacks out of the visible
// profile folder into DataDir/protected/<packID>/ and puts a link back so
// vanilla Minecraft still reads them from gameDir/<folder>.
func RelocateProtectedMedia(packID, profileDir string) error {
	if packID == "" || profileDir == "" {
		return nil
	}
	ok, err := IsProtectedByAuthor(packID)
	if err != nil || !ok {
		return err
	}
	store := paths.ProtectedPackDir(packID)
	if err := os.MkdirAll(store, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(store, 0o700)
	for _, folder := range []string{"resourcepacks", "shaderpacks"} {
		if err := relocateFolder(profileDir, store, folder); err != nil {
			return err
		}
	}
	hideProtectedDir(store)
	return nil
}

func relocateFolder(profileDir, store, folder string) error {
	src := filepath.Join(profileDir, folder)
	dst := filepath.Join(store, folder)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	st, err := os.Lstat(src)
	if err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			cur, _ := os.Readlink(src)
			if filepath.Clean(cur) == filepath.Clean(dst) {
				return nil
			}
			_ = os.Remove(src)
		} else if st.IsDir() {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			if err := os.RemoveAll(src); err != nil {
				return err
			}
		} else {
			_ = os.Remove(src)
		}
	}
	return replaceWithDirLink(src, dst)
}
