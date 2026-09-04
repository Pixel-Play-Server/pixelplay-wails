package mcmeta

import (
	"encoding/json"
	"encoding/xml"
	"net/url"
	"sort"
	"strings"

	"pixelplay-launcher/internal/httpx"
)

func FabricLoaderVersions(mcVersion string) (json.RawMessage, error) {
	u := "https://meta.fabricmc.net/v2/versions/loader/" + url.PathEscape(mcVersion)
	b, err := httpx.Get(u)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func QuiltLoaderVersions(mcVersion string) (json.RawMessage, error) {
	u := "https://meta.quiltmc.org/v3/versions/loader/" + url.PathEscape(mcVersion)
	b, err := httpx.Get(u)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func ForgeVersionsFor(mcVersion string) ([]string, error) {
	b, err := httpx.Get(forgeMaven + "maven-metadata.xml")
	if err != nil {
		return nil, err
	}
	var meta mavenMeta
	if err := xml.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	prefix := mcVersion + "-"
	out := make([]string, 0)
	for _, v := range meta.Versioning.Versions.Version {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return compareLoose(out[i], out[j]) > 0 })
	return out, nil
}

func NeoForgeVersionsFor(mcVersion string) ([]string, error) {
	b, err := httpx.Get(neoMaven + "maven-metadata.xml")
	if err != nil {
		return nil, err
	}
	var meta mavenMeta
	if err := xml.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	prefix := neoPrefix(mcVersion)
	out := make([]string, 0)
	for _, v := range meta.Versioning.Versions.Version {
		if strings.HasPrefix(v, prefix) && !strings.Contains(strings.ToLower(v), "beta") {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return compareLoose(out[i], out[j]) > 0 })
	return out, nil
}

type ResolvedLoader struct {
	Version *string `json:"version"`
	Reason  string  `json:"reason"`
}

func ResolveLoaderVersion(loader, requested, mcVersion string) ResolvedLoader {
	req := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(requested, " (stable)"), " (latest)"))
	if req != "" {
		v := req
		return ResolvedLoader{Version: &v, Reason: "user_overwrite"}
	}
	var ver string
	var err error
	switch strings.ToLower(loader) {
	case "fabric":
		raw, e := FabricLoaderVersions(mcVersion)
		err = e
		if e == nil {
			var loaders []struct {
				Loader struct {
					Version string `json:"version"`
					Stable  bool   `json:"stable"`
				} `json:"loader"`
			}
			_ = json.Unmarshal(raw, &loaders)
			for _, l := range loaders {
				if l.Loader.Stable {
					ver = l.Loader.Version
					break
				}
			}
			if ver == "" && len(loaders) > 0 {
				ver = loaders[0].Loader.Version
			}
		}
	case "quilt":
		raw, e := QuiltLoaderVersions(mcVersion)
		err = e
		if e == nil {
			var loaders []struct {
				Loader struct {
					Version string `json:"version"`
				} `json:"loader"`
			}
			_ = json.Unmarshal(raw, &loaders)
			if len(loaders) > 0 {
				ver = loaders[0].Loader.Version
			}
		}
	case "forge":
		list, e := ForgeVersionsFor(mcVersion)
		err = e
		if e == nil && len(list) > 0 {
			ver = list[0]
		}
	case "neoforge":
		list, e := NeoForgeVersionsFor(mcVersion)
		err = e
		if e == nil && len(list) > 0 {
			ver = list[0]
		}
	default:
		return ResolvedLoader{Reason: "not_resolved"}
	}
	if err != nil || ver == "" {
		return ResolvedLoader{Reason: "not_resolved"}
	}
	return ResolvedLoader{Version: &ver, Reason: "profile_default"}
}

func compareLoose(a, b string) int {
	as := strings.FieldsFunc(a, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	bs := strings.FieldsFunc(b, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi string
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
		}
		an, aok := atoi(ai)
		bn, bok := atoi(bi)
		if aok && bok {
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
			continue
		}
		if ai != bi {
			return strings.Compare(ai, bi)
		}
	}
	return 0
}

func atoi(s string) (int, bool) {
	n := 0
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
