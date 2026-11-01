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
	if err := EnsureVanillaLauncherLayout(mcVersion, emit); err != nil {
		return nil, fmt.Errorf("preparar vanilla para NeoForge: %w", err)
	}
	if err := runInstallerClient(javaBin, installer, emit); err != nil {
		return nil, fmt.Errorf("instalador NeoForge: %w", err)
	}
	_, _ = DownloadLibraries(&meta, nil, emit)
	ensureClientJar(meta.ID, mcVersion)
	if err := ensureNeoForgeGameJars(ver, mcVersion); err != nil {
		if emit != nil {
			emit("Reintentando instalador NeoForge…")
		}
		if err2 := runInstallerClient(javaBin, installer, emit); err2 != nil {
			return nil, fmt.Errorf("instalador NeoForge (retry): %w", err2)
		}
		if err := ensureNeoForgeGameJars(ver, mcVersion); err != nil {
			return nil, err
		}
	}
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

// ensureNeoForgeGameJars checks the jars FML needs as the minecraft + neoforge modules.
func ensureNeoForgeGameJars(neoVer, mcVersion string) error {
	neoClient := filepath.Join(LibrariesDir(), "net", "neoforged", "neoforge", neoVer, "neoforge-"+neoVer+"-client.jar")
	if st, err := os.Stat(neoClient); err != nil || st.Size() < 1024 {
		return fmt.Errorf("falta neoforge-client (%s) — el instalador no terminó", neoClient)
	}
	pattern := filepath.Join(LibrariesDir(), "net", "minecraft", "client", mcVersion+"-*", "client-"+mcVersion+"-*-srg.jar")
	matches, _ := filepath.Glob(pattern)
	ok := false
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.Size() > 1024 {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("falta minecraft client-srg para %s (patrón %s)", mcVersion, pattern)
	}
	return nil
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
