package skins

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/paths"
)

type Skin struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Base64Data  string  `json:"base64_data"`
	Variant     string  `json:"variant"`
	Description *string `json:"description"`
	AddedAt     string  `json:"added_at"`
}

type storeFile struct {
	Skins []Skin `json:"skins"`
}

func load() (*storeFile, error) {
	b, err := os.ReadFile(paths.SkinsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return &storeFile{Skins: []Skin{}}, nil
		}
		return nil, err
	}
	var st storeFile
	if json.Unmarshal(b, &st) != nil {
		return &storeFile{Skins: []Skin{}}, nil
	}
	if st.Skins == nil {
		st.Skins = []Skin{}
	}
	return &st, nil
}

func save(st *storeFile) error {
	_ = os.MkdirAll(filepath.Dir(paths.SkinsFile()), 0o755)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.SkinsFile(), b, 0o644)
}

func List() ([]Skin, error) {
	st, err := load()
	if err != nil {
		return nil, err
	}
	return st.Skins, nil
}

func Remove(id string) (bool, error) {
	st, err := load()
	if err != nil {
		return false, err
	}
	out := st.Skins[:0]
	found := false
	for _, s := range st.Skins {
		if s.ID == id {
			found = true
			continue
		}
		out = append(out, s)
	}
	st.Skins = out
	return found, save(st)
}

func UpdateProperties(id, name, variant string) (*Skin, error) {
	st, err := load()
	if err != nil {
		return nil, err
	}
	for i := range st.Skins {
		if st.Skins[i].ID != id {
			continue
		}
		if name != "" {
			st.Skins[i].Name = name
		}
		if variant != "" {
			st.Skins[i].Variant = variant
		}
		if err := save(st); err != nil {
			return nil, err
		}
		s := st.Skins[i]
		return &s, nil
	}
	return nil, nil
}

func AddLocal(name, variant, description, b64 string) (Skin, error) {
	st, err := load()
	if err != nil {
		return Skin{}, err
	}
	s := Skin{
		ID:         uuid.NewString(),
		Name:       name,
		Base64Data: b64,
		Variant:    variant,
		AddedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if description != "" {
		s.Description = &description
	}
	st.Skins = append(st.Skins, s)
	return s, save(st)
}

func UserProfile(accessToken, uuidStr string) (json.RawMessage, error) {
	uuidStr = strings.ReplaceAll(strings.TrimSpace(uuidStr), "-", "")
	if len(uuidStr) == 32 {
		if b, err := sessionProfile(uuidStr); err == nil && len(b) > 2 {
			return b, nil
		}
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("falta access token o UUID para la skin")
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.minecraftservices.com/minecraft/profile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", httpx.UserAgent)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("perfil Minecraft: %s — %s", res.Status, truncate(string(b)))
	}
	converted, err := servicesProfileToSession(b)
	if err != nil {
		return json.RawMessage(b), nil
	}
	return converted, nil
}

func sessionProfile(uuidStr string) (json.RawMessage, error) {
	b, err := httpx.Get("https://sessionserver.mojang.com/session/minecraft/profile/" + uuidStr)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func servicesProfileToSession(raw []byte) (json.RawMessage, error) {
	var p struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Skins []struct {
			URL     string `json:"url"`
			Variant string `json:"variant"`
			State   string `json:"state"`
		} `json:"skins"`
		Capes []struct {
			URL   string `json:"url"`
			State string `json:"state"`
		} `json:"capes"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.ID == "" {
		return nil, fmt.Errorf("perfil sin id")
	}
	tex := map[string]any{
		"timestamp":   time.Now().UnixMilli(),
		"profileId":   p.ID,
		"profileName": p.Name,
		"textures":    map[string]any{},
	}
	textures := tex["textures"].(map[string]any)
	for _, s := range p.Skins {
		if strings.EqualFold(s.State, "ACTIVE") || len(p.Skins) == 1 {
			skin := map[string]any{"url": s.URL}
			if strings.EqualFold(s.Variant, "SLIM") {
				skin["metadata"] = map[string]any{"model": "slim"}
			}
			textures["SKIN"] = skin
			break
		}
	}
	for _, c := range p.Capes {
		if strings.EqualFold(c.State, "ACTIVE") {
			textures["CAPE"] = map[string]any{"url": c.URL}
			break
		}
	}
	tb, _ := json.Marshal(tex)
	out := map[string]any{
		"id":   p.ID,
		"name": p.Name,
		"properties": []map[string]any{
			{"name": "textures", "value": base64.StdEncoding.EncodeToString(tb)},
		},
		"skins": p.Skins,
		"capes": p.Capes,
	}
	b, err := json.Marshal(out)
	return json.RawMessage(b), err
}

func ApplyPNG(accessToken, variant string, png []byte) error {
	if variant == "" {
		variant = "classic"
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("variant", variant)
	fw, err := w.CreateFormFile("file", "skin.png")
	if err != nil {
		return err
	}
	if _, err := fw.Write(png); err != nil {
		return err
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, "https://api.minecraftservices.com/minecraft/profile/skins", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return fmt.Errorf("subir skin: %s — %s", res.Status, truncate(string(b)))
	}
	return nil
}

func ApplyBase64(accessToken, variant, b64 string) error {
	b64 = strings.TrimPrefix(b64, "data:image/png;base64,")
	png, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return ApplyPNG(accessToken, variant, png)
}

func Reset(accessToken string) error {
	req, err := http.NewRequest(http.MethodDelete, "https://api.minecraftservices.com/minecraft/profile/skins/active", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 && res.StatusCode != 404 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("reset skin: %s — %s", res.Status, truncate(string(b)))
	}
	return nil
}

func MojangProfile(query string) (json.RawMessage, error) {
	query = strings.TrimSpace(query)
	id := strings.ReplaceAll(query, "-", "")
	if len(id) == 32 {
		return sessionProfile(id)
	}
	uuidStr, err := lookupUUID(query)
	if err != nil {
		return nil, err
	}
	return sessionProfile(strings.ReplaceAll(uuidStr, "-", ""))
}

func lookupUUID(name string) (string, error) {
	var ms struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	u := "https://api.minecraftservices.com/minecraft/profile/lookup/name/" + url.PathEscape(name)
	if err := httpx.GetJSON(u, &ms); err == nil && ms.ID != "" {
		return ms.ID, nil
	}
	var pdb struct {
		Success bool `json:"success"`
		Data    struct {
			Player struct {
				RawID string `json:"raw_id"`
				ID    string `json:"id"`
			} `json:"player"`
		} `json:"data"`
	}
	if err := httpx.GetJSON("https://playerdb.co/api/player/minecraft/"+url.PathEscape(name), &pdb); err == nil {
		if pdb.Data.Player.RawID != "" {
			return pdb.Data.Player.RawID, nil
		}
		if pdb.Data.Player.ID != "" {
			return pdb.Data.Player.ID, nil
		}
	}
	var legacy struct {
		ID string `json:"id"`
	}
	if err := httpx.GetJSON("https://api.mojang.com/users/profiles/minecraft/"+url.PathEscape(name), &legacy); err == nil && legacy.ID != "" {
		return legacy.ID, nil
	}
	return "", fmt.Errorf("no encontré el jugador %s", name)
}

func PNGFromSource(source map[string]any) (string, error) {
	typ, _ := source["type"].(string)
	details, _ := source["details"].(map[string]any)
	if details == nil {
		details = source
	}
	switch strings.ToLower(typ) {
	case "base64":
		s, _ := details["base64_content"].(string)
		if s == "" {
			s, _ = details["base64"].(string)
		}
		return strings.TrimPrefix(s, "data:image/png;base64,"), nil
	case "url":
		u, _ := details["url"].(string)
		return fetchB64(u)
	case "filepath", "file_path":
		p, _ := details["path"].(string)
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(b), nil
	case "profile":
		q, _ := details["query"].(string)
		return fetchProfileSkin(q)
	default:
		if u, _ := details["url"].(string); u != "" {
			return fetchB64(u)
		}
		return "", fmt.Errorf("fuente de skin desconocida")
	}
}

func fetchB64(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails)")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("skin %s: %s", rawURL, res.Status)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func fetchProfileSkin(query string) (string, error) {
	query = strings.TrimSpace(query)
	uuidStr := strings.ReplaceAll(query, "-", "")
	if len(uuidStr) != 32 {
		id, err := lookupUUID(query)
		if err != nil {
			return "", err
		}
		uuidStr = strings.ReplaceAll(id, "-", "")
	}
	var sess struct {
		Properties []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"properties"`
	}
	if err := httpx.GetJSON("https://sessionserver.mojang.com/session/minecraft/profile/"+uuidStr, &sess); err != nil {
		return "", err
	}
	for _, p := range sess.Properties {
		if p.Name != "textures" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p.Value)
		if err != nil {
			return "", err
		}
		var tex struct {
			Textures struct {
				Skin struct {
					URL string `json:"url"`
				} `json:"SKIN"`
			} `json:"textures"`
		}
		if err := json.Unmarshal(raw, &tex); err != nil {
			return "", err
		}
		if tex.Textures.Skin.URL == "" {
			return "", fmt.Errorf("el perfil no tiene skin")
		}
		return fetchB64(tex.Textures.Skin.URL)
	}
	return "", fmt.Errorf("sin texturas para %s", query)
}

func StarlightRender(player, renderType, view, b64 string) (string, error) {
	if renderType == "" {
		renderType = "default"
	}
	if view == "" {
		view = "full"
	}
	b64 = strings.TrimPrefix(strings.TrimSpace(b64), "data:image/png;base64,")
	sum := sha256.Sum256([]byte(player + "|" + renderType + "|" + view + "|" + b64))
	key := hex.EncodeToString(sum[:16])
	dir := filepath.Join(paths.DataDir(), "meta", "starlight_cache")
	_ = os.MkdirAll(dir, 0o755)
	dest := filepath.Join(dir, key+".png")
	if isPNGFile(dest) {
		return fileToDataURL(dest)
	}

	var urls []string
	if b64 != "" {
		urls = append(urls, fmt.Sprintf(
			"https://starlightskins.lunareclipse.studio/render/%s/%s/%s?skinUrl=%s",
			url.PathEscape(renderType), url.PathEscape(orSkin(player)), url.PathEscape(view),
			url.QueryEscape("data:image/png;base64,"+b64),
		))
	} else if player != "" {
		urls = append(urls,
			fmt.Sprintf("https://starlightskins.lunareclipse.studio/render/%s/%s/%s",
				url.PathEscape(renderType), url.PathEscape(player), url.PathEscape(view)),
			"https://mc-heads.net/body/"+url.PathEscape(player),
			"https://vzge.me/full/"+url.PathEscape(player),
			"https://crafatar.com/renders/body/"+url.PathEscape(strings.ReplaceAll(player, "-", ""))+"?overlay=true",
		)
	}
	var last error
	for _, u := range urls {
		png, err := fetchPNG(u)
		if err != nil {
			last = err
			continue
		}
		_ = os.WriteFile(dest, png, 0o644)
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
	}
	if b64 != "" {
		return "data:image/png;base64," + b64, nil
	}
	if last != nil {
		return "", last
	}
	return "", fmt.Errorf("no pude renderizar la skin de %s", player)
}

func orSkin(player string) string {
	if strings.TrimSpace(player) == "" {
		return "skin"
	}
	return player
}

func isPNGFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [8]byte
	n, _ := f.Read(hdr[:])
	return n >= 8 && hdr[0] == 0x89 && hdr[1] == 'P' && hdr[2] == 'N' && hdr[3] == 'G'
}

func fileToDataURL(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b), nil
}

func fetchPNG(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Accept", "image/png,image/*,*/*")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("render %s: %s", rawURL, res.Status)
	}
	if len(b) < 8 || b[0] != 0x89 || b[1] != 'P' || b[2] != 'N' || b[3] != 'G' {
		return nil, fmt.Errorf("respuesta no es PNG: %s", rawURL)
	}
	return b, nil
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
	if s == "" {
		return "x"
	}
	return s
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
