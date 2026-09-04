package mcmeta

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"pixelplay-launcher/internal/download"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/paths"
)

const forgeMaven = "https://maven.minecraftforge.net/net/minecraftforge/forge/"

type LoaderInstall struct {
	ID        string
	Version   string
	MainClass string
	Libraries []Library
	Arguments *Arguments
}

func InstallForge(mcVersion, requested, javaBin string, emit Progress) (*LoaderInstall, error) {
	if emit != nil {
		emit("Buscando Forge para " + mcVersion)
	}
	ver, err := resolveForgeVersion(mcVersion, requested)
	if err != nil {
		return nil, err
	}
	if emit != nil {
		emit("Descargando instalador Forge " + ver)
	}
	installer := filepath.Join(paths.DataDir(), "meta", "forge", "forge-"+ver+"-installer.jar")
	url := forgeMaven + ver + "/forge-" + ver + "-installer.jar"
	if st, err := os.Stat(installer); err != nil || st.Size() == 0 {
		if err := download.File(url, installer, nil); err != nil {
			return nil, err
		}
	}
	meta, err := readInstallerVersionJSON(installer)
	if err != nil {
		return nil, err
	}
	for i := range meta.Libraries {
		if meta.Libraries[i].URL == "" {
			meta.Libraries[i].URL = "https://maven.minecraftforge.net/"
		}
	}
	if err := runInstallerClient(javaBin, installer, emit); err != nil && emit != nil {
		emit("aviso instalador Forge: " + trimMsg(err.Error(), 240))
	}
	_, _ = DownloadLibraries(meta, nil, emit)
	ensureClientJar(meta.ID, mcVersion)
	return &LoaderInstall{
		ID:        meta.ID,
		Version:   ver,
		MainClass: meta.MainClass,
		Libraries: meta.Libraries,
		Arguments: meta.Arguments,
	}, nil
}

func resolveForgeVersion(mcVersion, requested string) (string, error) {
	req := strings.TrimSpace(requested)
	req = strings.TrimSuffix(req, " (stable)")
	req = strings.TrimSuffix(req, " (latest)")
	req = strings.TrimPrefix(req, "forge-")
	b, err := httpx.Get(forgeMaven + "maven-metadata.xml")
	if err != nil {
		if req != "" {
			if strings.HasPrefix(req, mcVersion+"-") {
				return req, nil
			}
			return mcVersion + "-" + req, nil
		}
		return "", err
	}
	var meta mavenMeta
	if err := xml.Unmarshal(b, &meta); err != nil {
		return "", err
	}
	vers := meta.Versioning.Versions.Version
	if req != "" {
		if found := matchForgeVersion(vers, mcVersion, req); found != "" {
			return found, nil
		}
	}
	prefix := mcVersion + "-"
	var match string
	for _, v := range vers {
		if strings.HasPrefix(v, prefix) && !strings.Contains(strings.ToLower(v), "beta") {
			match = v
		}
	}
	if match == "" {
		return "", fmt.Errorf("no hay Forge para Minecraft %s", mcVersion)
	}
	return match, nil
}

func matchForgeVersion(vers []string, mc, req string) string {
	if req == "" {
		return ""
	}
	for _, v := range vers {
		if v == req {
			return v
		}
	}
	full := mc + "-" + req
	for _, v := range vers {
		if v == full {
			return v
		}
	}
	for _, v := range vers {
		if strings.HasSuffix(v, "-"+req) {
			return v
		}
	}
	if i := strings.LastIndex(req, "-"); i >= 0 {
		suf := req[i+1:]
		for _, v := range vers {
			if strings.HasSuffix(v, "-"+suf) && strings.HasPrefix(v, mc+"-") {
				return v
			}
		}
	}
	return ""
}

func readInstallerVersionJSON(installer string) (*PistonMeta, error) {
	b, err := zipFileBytes(installer, "version.json")
	if err == nil {
		var meta PistonMeta
		if err := json.Unmarshal(b, &meta); err != nil {
			return nil, err
		}
		return &meta, nil
	}
	// Older installers nest versionInfo inside install_profile.json
	raw, err2 := zipFileBytes(installer, "install_profile.json")
	if err2 != nil {
		return nil, err
	}
	var prof struct {
		VersionInfo json.RawMessage `json:"versionInfo"`
		JSON        json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal(raw, &prof); err != nil {
		return nil, err
	}
	payload := prof.VersionInfo
	if len(payload) == 0 {
		payload = prof.JSON
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("instalador sin version.json")
	}
	var meta PistonMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func runInstallerClient(javaBin, installer string, emit Progress) error {
	mcRoot := filepath.Join(paths.DataDir(), "meta")
	_ = os.MkdirAll(filepath.Join(mcRoot, "libraries"), 0o755)
	if emit != nil {
		emit("Ejecutando instalador del loader…")
	}
	cmd := exec.Command(javaBin, "-jar", installer, "--installClient", mcRoot)
	cmd.Dir = filepath.Dir(installer)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func ensureClientJar(id, vanillaID string) {
	if id == "" {
		return
	}
	client := ClientJar(id)
	if _, err := os.Stat(client); err == nil {
		return
	}
	vanilla := ClientJar(vanillaID)
	if _, err := os.Stat(vanilla); err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(client), 0o755)
	in, err := os.ReadFile(vanilla)
	if err != nil {
		return
	}
	_ = os.WriteFile(client, in, 0o644)
}

func trimMsg(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > n {
		return s[:n]
	}
	return s
}
