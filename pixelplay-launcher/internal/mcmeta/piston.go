package mcmeta

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"pixelplay-launcher/internal/httpx"
)

const ManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type VersionManifest struct {
	Latest   LatestVersions   `json:"latest"`
	Versions []ManifestVersion `json:"versions"`
}

type LatestVersions struct {
	Release  string `json:"release"`
	Snapshot string `json:"snapshot"`
}

type ManifestVersion struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Time        string `json:"time"`
	ReleaseTime string `json:"releaseTime"`
}

type PistonMeta struct {
	ID                  string          `json:"id"`
	MainClass           string          `json:"mainClass"`
	Type                string          `json:"type"`
	Libraries           []Library       `json:"libraries"`
	AssetIndex          AssetIndex      `json:"assetIndex"`
	Assets              string          `json:"assets"`
	Downloads           Downloads       `json:"downloads"`
	JavaVersion         JavaVersion     `json:"javaVersion"`
	Arguments           *Arguments      `json:"arguments"`
	MinecraftArguments  string          `json:"minecraftArguments"`
	Logging             json.RawMessage `json:"logging"`
	InheritsFrom        string          `json:"inheritsFrom"`
}

type AssetIndex struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	TotalSize int64  `json:"totalSize"`
	URL       string `json:"url"`
}

type Downloads struct {
	Client Artifact `json:"client"`
}

type Artifact struct {
	Path string `json:"path"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type JavaVersion struct {
	Component     string `json:"component"`
	MajorVersion  int    `json:"majorVersion"`
}

type Library struct {
	Name      string          `json:"name"`
	Downloads *LibDownloads   `json:"downloads"`
	Natives   map[string]string `json:"natives"`
	Rules     []Rule          `json:"rules"`
	URL       string          `json:"url"`
}

type LibDownloads struct {
	Artifact    *Artifact            `json:"artifact"`
	Classifiers map[string]Artifact  `json:"classifiers"`
}

type Rule struct {
	Action string `json:"action"`
	OS     *struct {
		Name string `json:"name"`
		Arch string `json:"arch"`
	} `json:"os"`
	Features map[string]bool `json:"features"`
}

type Arguments struct {
	Game []json.RawMessage `json:"game"`
	JVM  []json.RawMessage `json:"jvm"`
}

func FetchManifest() (*VersionManifest, error) {
	var m VersionManifest
	if err := httpx.GetJSON(ManifestURL, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func FetchPiston(versionID string) (*PistonMeta, error) {
	man, err := FetchManifest()
	if err != nil {
		return nil, err
	}
	var url string
	for _, v := range man.Versions {
		if v.ID == versionID {
			url = v.URL
			break
		}
	}
	if url == "" {
		return nil, fmt.Errorf("versión Minecraft no encontrada: %s", versionID)
	}
	var meta PistonMeta
	if err := httpx.GetJSON(url, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func CurrentOSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "osx"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

func CurrentArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x86"
}

func RulesAllow(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	osName := CurrentOSName()
	arch := CurrentArch()
	for _, r := range rules {
		match := true
		if r.OS != nil {
			if r.OS.Name != "" && r.OS.Name != osName {
				match = false
			}
			if r.OS.Arch != "" && r.OS.Arch != arch {
				match = false
			}
		}
		if r.Features != nil {
			// skip feature-gated args/libs (demo, quickplay, custom res)
			match = false
		}
		if !match {
			continue
		}
		if strings.EqualFold(r.Action, "allow") {
			allowed = true
		} else {
			allowed = false
		}
	}
	return allowed
}

func MavenPath(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return ""
	}
	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact := parts[1]
	version := parts[2]
	classifier := ""
	if len(parts) >= 4 {
		classifier = parts[3]
	}
	file := artifact + "-" + version
	if classifier != "" {
		file += "-" + classifier
	}
	return group + "/" + artifact + "/" + version + "/" + file + ".jar"
}
