package mrpack

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

type index struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary"`
	Files         []indexFile       `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type indexFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       map[string]string `json:"env"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

// ImportFile installs a local .mrpack as a new launcher profile.
func ImportFile(packPath string) (string, error) {
	zr, err := zip.OpenReader(packPath)
	if err != nil {
		return "", fmt.Errorf("abrir .mrpack: %w", err)
	}
	defer zr.Close()

	man, err := readIndex(&zr.Reader)
	if err != nil {
		return "", err
	}

	gv := man.Dependencies["minecraft"]
	if gv == "" {
		gv = "1.21.1"
	}
	loader, loaderVer := loaderFromDeps(man.Dependencies)
	name := strings.TrimSpace(man.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(packPath), filepath.Ext(packPath))
	}
	params := map[string]any{
		"name":         name,
		"game_version": gv,
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
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}

	if err := extractOverrides(&zr.Reader, root); err != nil {
		return "", err
	}
	if err := downloadIndexFiles(man.Files, root); err != nil {
		return "", err
	}
	return id, nil
}

// DownloadAndInstall downloads a Modrinth modpack URL and imports it.
func DownloadAndInstall(downloadURL, fileName string) (string, error) {
	downloadURL = strings.TrimSpace(downloadURL)
	if downloadURL == "" {
		return "", fmt.Errorf("no hay URL de descarga del .mrpack")
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "modpack.mrpack"
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".mrpack") {
		fileName += ".mrpack"
	}
	tmp := filepath.Join(os.TempDir(), "pixelplay-mrpack-"+filepath.Base(fileName))
	if err := download.File(downloadURL, tmp, nil); err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	return ImportFile(tmp)
}

func readIndex(zr *zip.Reader) (*index, error) {
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "modrinth.index.json" && f.Name != "modrinth.index.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		var man index
		if err := json.Unmarshal(b, &man); err != nil {
			return nil, fmt.Errorf("modrinth.index.json: %w", err)
		}
		if man.Dependencies == nil {
			man.Dependencies = map[string]string{}
		}
		return &man, nil
	}
	return nil, fmt.Errorf("el archivo no es un .mrpack válido (falta modrinth.index.json)")
}

func loaderFromDeps(deps map[string]string) (loader, version string) {
	switch {
	case deps["fabric-loader"] != "":
		return "fabric", deps["fabric-loader"]
	case deps["quilt-loader"] != "":
		return "quilt", deps["quilt-loader"]
	case deps["neoforge"] != "":
		return "neoforge", deps["neoforge"]
	case deps["forge"] != "":
		return "forge", deps["forge"]
	default:
		return "vanilla", ""
	}
}

func extractOverrides(zr *zip.Reader, root string) error {
	prefixes := []string{"overrides/", "client-overrides/"}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		low := strings.ToLower(name)
		if strings.HasPrefix(low, "server-overrides/") {
			continue
		}
		var rel string
		for _, p := range prefixes {
			if strings.HasPrefix(low, p) {
				rel = name[len(p):]
				break
			}
		}
		if rel == "" || f.FileInfo().IsDir() {
			continue
		}
		dest, ok := safeJoin(root, rel)
		if !ok {
			continue
		}
		if err := extractZipFile(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func downloadIndexFiles(files []indexFile, root string) error {
	for _, file := range files {
		if env := strings.ToLower(strings.TrimSpace(file.Env["client"])); env == "unsupported" {
			continue
		}
		if len(file.Downloads) == 0 || strings.TrimSpace(file.Path) == "" {
			continue
		}
		dest, ok := safeJoin(root, file.Path)
		if !ok {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		var lastErr error
		for _, url := range file.Downloads {
			url = strings.TrimSpace(url)
			if url == "" {
				continue
			}
			if err := download.File(url, dest, nil); err != nil {
				lastErr = err
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return fmt.Errorf("descargar %s: %w", file.Path, lastErr)
		}
	}
	return nil
}

func safeJoin(root, rel string) (string, bool) {
	rel = filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(rel), "/"))
	if rel == "" || strings.Contains(rel, "..") {
		return "", false
	}
	dest := filepath.Join(root, rel)
	absRoot, err1 := filepath.Abs(root)
	absDest, err2 := filepath.Abs(dest)
	if err1 != nil || err2 != nil {
		return "", false
	}
	if absDest != absRoot && !strings.HasPrefix(absDest, absRoot+string(os.PathSeparator)) {
		return "", false
	}
	return dest, true
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
