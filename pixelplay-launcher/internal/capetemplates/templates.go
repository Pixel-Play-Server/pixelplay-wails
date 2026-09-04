package capetemplates

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed files/*
var files embed.FS

func WriteTo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(files, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, filepath.Base(path)), b, 0o644)
	})
}
