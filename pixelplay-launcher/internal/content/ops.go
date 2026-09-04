package content

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/protect"
)

func Uninstall(profileID, filePath, sha1, contentType string) error {
	if ok, _ := protect.ProfileProtected(profileID); ok && isModType(contentType) {
		return protect.ErrAuthorProtected
	}
	if filePath != "" {
		return Delete(filePath)
	}
	if sha1 == "" {
		return fmt.Errorf("hace falta file_path o sha1_hash")
	}
	items, err := ListLocal(profileID, contentTypeOrMod(contentType), true)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Sha1Hash != nil && strings.EqualFold(*it.Sha1Hash, sha1) {
			return Delete(it.PathStr)
		}
	}
	return fmt.Errorf("no encontré el archivo con sha1 %s", sha1)
}

func Toggle(profileID, filePath, sha1, contentType string, enabled bool) error {
	if ok, _ := protect.ProfileProtected(profileID); ok && isModType(contentType) {
		return protect.ErrAuthorProtected
	}
	if filePath != "" {
		return SetEnabled(filePath, enabled)
	}
	if sha1 == "" {
		return fmt.Errorf("hace falta file_path o sha1_hash")
	}
	items, err := ListLocal(profileID, contentTypeOrMod(contentType), true)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Sha1Hash != nil && strings.EqualFold(*it.Sha1Hash, sha1) {
			return SetEnabled(it.PathStr, enabled)
		}
	}
	return fmt.Errorf("no encontré el archivo con sha1 %s", sha1)
}

func InstallRemote(profileID, contentType, fileName, downloadURL string) error {
	if ok, _ := protect.ProfileProtected(profileID); ok && isModType(contentType) {
		return protect.ErrAuthorProtected
	}
	if downloadURL == "" {
		return fmt.Errorf("falta download_url")
	}
	dir, err := ProfileContentDir(profileID, contentType)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if fileName == "" {
		fileName = filepath.Base(downloadURL)
	}
	safe := filepath.Base(fileName)
	dest := filepath.Join(dir, safe)
	return download.File(downloadURL, dest, nil)
}

func isModType(ct string) bool {
	switch strings.ToLower(ct) {
	case "", "mod", "mods", "pixelplaymod":
		return true
	default:
		return false
	}
}

func contentTypeOrMod(ct string) string {
	if strings.TrimSpace(ct) == "" {
		return "Mod"
	}
	return ct
}

func PreviewImage(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	w, h := 0, 0
	if len(b) >= 24 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		w = int(b[16])<<24 | int(b[17])<<16 | int(b[18])<<8 | int(b[19])
		h = int(b[20])<<24 | int(b[21])<<16 | int(b[22])<<8 | int(b[23])
	}
	enc := base64.StdEncoding.EncodeToString(b)
	return map[string]any{
		"base64_image":    enc,
		"original_width":  w,
		"original_height": h,
		"preview_width":   w,
		"preview_height":  h,
	}, nil
}
