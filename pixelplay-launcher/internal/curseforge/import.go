package curseforge

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
)

type cfManifest struct {
	Name      string `json:"name"`
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	Files []struct {
		ProjectID uint64 `json:"projectID"`
		FileID    uint64 `json:"fileID"`
		Required  bool   `json:"required"`
	} `json:"files"`
	Overrides string `json:"overrides"`
}

func ImportPack(zipPath string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("abrir pack CurseForge: %w", err)
	}
	defer zr.Close()

	var man cfManifest
	found := false
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), "manifest.json") {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
			if err := json.Unmarshal(b, &man); err != nil {
				return "", fmt.Errorf("manifest.json: %w", err)
			}
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("el zip no es un modpack de CurseForge (falta manifest.json)")
	}

	loader, loaderVer := "forge", ""
	for _, l := range man.Minecraft.ModLoaders {
		if !l.Primary && loaderVer != "" {
			continue
		}
		id := strings.ToLower(l.ID)
		switch {
		case strings.HasPrefix(id, "forge-"):
			loader, loaderVer = "forge", strings.TrimPrefix(l.ID, "forge-")
		case strings.HasPrefix(id, "neoforge-"):
			loader, loaderVer = "neoforge", strings.TrimPrefix(l.ID, "neoforge-")
		case strings.HasPrefix(id, "fabric-"):
			loader, loaderVer = "fabric", strings.TrimPrefix(l.ID, "fabric-")
		case strings.HasPrefix(id, "quilt-"):
			loader, loaderVer = "quilt", strings.TrimPrefix(l.ID, "quilt-")
		}
		if l.Primary {
			break
		}
	}
	name := man.Name
	if name == "" {
		name = filepath.Base(zipPath)
	}
	params := map[string]any{
		"name":         name,
		"game_version": man.Minecraft.Version,
		"loader":       loader,
	}
	if loaderVer != "" {
		params["loader_version"] = loaderVer
	}
	id, err := profiles.Create(params)
	if err != nil {
		return "", err
	}
	p, err := profiles.Get(id)
	if err != nil {
		return "", err
	}
	root := paths.ProfileDir(p.Path)
	modsDir := filepath.Join(root, "mods")
	_ = os.MkdirAll(modsDir, 0o755)

	ovr := man.Overrides
	if ovr == "" {
		ovr = "overrides"
	}
	ovr = strings.TrimSuffix(ovr, "/") + "/"
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(ovr)) {
			continue
		}
		rel := name[len(ovr):]
		if rel == "" || f.FileInfo().IsDir() {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(rel))
		if err := extractZipFile(f, dest); err != nil {
			return "", err
		}
	}

	for _, file := range man.Files {
		if file.ProjectID == 0 || file.FileID == 0 {
			continue
		}
		cf, err := GetFile(file.ProjectID, file.FileID)
		if err != nil {
			continue
		}
		dl := cf.DownloadURL
		if dl == "" {
			dl = fmt.Sprintf("https://www.curseforge.com/api/v1/mods/%d/files/%d/download", file.ProjectID, file.FileID)
		}
		fname := cf.FileName
		if fname == "" {
			fname = fmt.Sprintf("%d-%d.jar", file.ProjectID, file.FileID)
		}
		_ = download.File(dl, filepath.Join(modsDir, filepath.Base(fname)), nil)
	}
	return id, nil
}

func DownloadAndInstall(projectID, fileID uint64, fileName, downloadURL string) (string, error) {
	if downloadURL == "" && projectID != 0 && fileID != 0 {
		cf, err := GetFile(projectID, fileID)
		if err != nil {
			return "", err
		}
		downloadURL = cf.DownloadURL
		if fileName == "" {
			fileName = cf.FileName
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("no hay URL de descarga del modpack")
	}
	if fileName == "" {
		fileName = fmt.Sprintf("cf-%d-%d.zip", projectID, fileID)
	}
	tmp := filepath.Join(os.TempDir(), filepath.Base(fileName))
	if err := download.File(downloadURL, tmp, nil); err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	return ImportPack(tmp)
}

func extractZipFile(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
