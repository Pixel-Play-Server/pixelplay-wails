package horizon

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var safePackVersion = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,100}$`)

// Keep packs zipped: never extract catalog paths into the profile.
func installRegistryPack(gameDir, source, id, version string) (string, error) {
	if !safeModID.MatchString(id) || !safePackVersion.MatchString(version) {
		return "", fmt.Errorf("identidad de resource pack inválida")
	}
	z, err := zip.OpenReader(source)
	if err != nil {
		return "", err
	}
	valid := false
	for _, f := range z.File {
		if f.Name == "pack.mcmeta" && f.UncompressedSize64 <= 128<<10 {
			valid = true
		}
	}
	z.Close()
	if !valid {
		return "", fmt.Errorf("resource pack sin pack.mcmeta")
	}
	dir := filepath.Join(gameDir, "resourcepacks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(dir, id+"-"+version+".zip")
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("resource pack ya instalado")
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".pack-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, io.LimitReader(in, 64<<20+1))
	closeErr := tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	return id, nil
}
