//go:build !windows

package packs

import (
	"os"
	"path/filepath"
)

func replaceWithDirLink(link, target string) error {
	_ = os.RemoveAll(link)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, link)
}

func hideProtectedDir(path string) {
	_ = os.Chmod(path, 0o700)
}
