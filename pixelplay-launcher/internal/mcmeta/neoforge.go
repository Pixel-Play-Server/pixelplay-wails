package mcmeta

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/paths"
)

const neoMaven = "https://maven.neoforged.net/releases/net/neoforged/neoforge/"

type mavenMeta struct {
	Versioning struct {
		Versions struct {
			Version []string `xml:"version"`
		} `xml:"versions"`
	} `xml:"versioning"`
}

type NeoResult struct {
	ID        string
	Version   string
	MainClass string
	Libraries []Library
	Arguments *Arguments
	ClientJar string
}

func InstallNeoForge(mcVersion, requested, javaBin string, emit Progress) (*NeoResult, error) {
	if emit != nil {
		emit("Buscando NeoForge para " + mcVersion)
	}
	ver, err := resolveNeoVersion(mcVersion, requested)
	if err != nil {
		return nil, err
	}
	if emit != nil {
		emit("Descargando instalador NeoForge " + ver)
	}
	installer := filepath.Join(paths.DataDir(), "meta", "neoforge", "neoforge-"+ver+"-installer.jar")
	url := neoMaven + ver + "/neoforge-" + ver + "-installer.jar"
	if st, err := os.Stat(installer); err != nil || st.Size() == 0 {
		if err := download.File(url, installer, nil); err != nil {
			return nil, err
		}
	}
	versionJSON, err := zipFileBytes(installer, "version.json")
	if err != nil {
		return nil, fmt.Errorf("version.json del instalador NeoForge: %w", err)
	}
	var meta PistonMeta
	if err := json.Unmarshal(versionJSON, &meta); err != nil {
		return nil, err
	}
	for i := range meta.Libraries {
		if meta.Libraries[i].URL == "" {
			meta.Libraries[i].URL = "https://maven.neoforged.net/releases/"
		}
	}
	if err := runInstallerClient(javaBin, installer, emit); err != nil && emit != nil {
		emit("aviso instalador NeoForge: " + trimMsg(err.Error(), 240))
	}
	_, _ = DownloadLibraries(&meta, nil, emit)
	ensureClientJar(meta.ID, mcVersion)
	return &NeoResult{
		ID:        meta.ID,
		Version:   ver,
		MainClass: meta.MainClass,
		Libraries: meta.Libraries,
		Arguments: meta.Arguments,
		ClientJar: ClientJar(meta.ID),
	}, nil
}

func resolveNeoVersion(mcVersion, requested string) (string, error) {
	req := strings.TrimSpace(requested)
	req = strings.TrimSuffix(req, " (stable)")
	req = strings.TrimPrefix(req, "neoforge-")
	if req != "" {
		return req, nil
	}
	b, err := httpx.Get(neoMaven + "maven-metadata.xml")
	if err != nil {
		return "", err
	}
	var meta mavenMeta
	if err := xml.Unmarshal(b, &meta); err != nil {
		return "", err
	}
	prefix := neoPrefix(mcVersion)
	var match string
	for _, v := range meta.Versioning.Versions.Version {
		if strings.HasPrefix(v, prefix) && !strings.Contains(v, "beta") {
			match = v // last matching is usually newest if list is chronological
		}
	}
	if match == "" {
		return "", fmt.Errorf("no hay NeoForge para Minecraft %s", mcVersion)
	}
	return match, nil
}

func neoPrefix(mc string) string {
	// 1.21.1 -> 21.1.
	parts := strings.Split(mc, ".")
	if len(parts) >= 3 {
		return parts[1] + "." + parts[2] + "."
	}
	if len(parts) == 2 {
		return parts[1] + ".0."
	}
	return mc
}

func zipFileBytes(zipPath, name string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.Base(f.Name) == name || f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			return b, err
		}
	}
	return nil, fmt.Errorf("%s no está en %s", name, zipPath)
}
