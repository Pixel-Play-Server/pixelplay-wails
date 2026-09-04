package capes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/config"
)

const (
	prodBase = "https://api.pixelplay.gg/api/v1/cosmetics"
	expBase  = "https://api-staging.pixelplay.gg/api/v1/cosmetics"
)

var client = &http.Client{Timeout: 40 * time.Second}

type BrowseParams struct {
	Page            *int
	PageSize        *int
	SortBy          string
	FilterHasElytra *bool
	FilterCreator   string
	TimeFrame       string
	PixelplayToken  string
	RequestUUID     string
}

func baseURL() string {
	if experimental() {
		return expBase
	}
	return prodBase
}

func experimental() bool {
	raw, err := config.Raw()
	if err != nil {
		return false
	}
	var cfg struct {
		IsExperimental *bool `json:"is_experimental"`
		Experimental   *bool `json:"experimental_mode"`
	}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.IsExperimental != nil {
		return *cfg.IsExperimental
	}
	if cfg.Experimental != nil {
		return *cfg.Experimental
	}
	return false
}

func resolveToken(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	acc, err := accounts.Active()
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", fmt.Errorf("no hay cuenta activa para capas")
	}
	if acc.Offline() {
		return "", fmt.Errorf("capas Pixel Play requieren cuenta Microsoft")
	}
	updated, err := auth.EnsurePixelplay(*acc)
	if err != nil {
		return "", err
	}
	acc = &updated
	tok := acc.PixelplayCredentials.Production
	if experimental() {
		tok = acc.PixelplayCredentials.Experimental
		if tok == nil {
			tok = acc.PixelplayCredentials.Production
		}
	}
	if tok == nil || tok.Value == "" {
		return "", fmt.Errorf("no se pudo obtener el token Pixel Play")
	}
	return tok.Value, nil
}

func Browse(p BrowseParams) (json.RawMessage, error) {
	token, err := resolveToken(p.PixelplayToken)
	if err != nil {
		return nil, err
	}
	reqUUID := p.RequestUUID
	if reqUUID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			reqUUID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", reqUUID)
	if p.Page != nil {
		q.Set("page", strconv.Itoa(*p.Page))
	}
	if p.PageSize != nil {
		q.Set("pageSize", strconv.Itoa(*p.PageSize))
	}
	if p.SortBy != "" {
		q.Set("sortBy", p.SortBy)
	}
	if p.FilterHasElytra != nil {
		q.Set("filterHasElytra", strconv.FormatBool(*p.FilterHasElytra))
	}
	if p.FilterCreator != "" {
		q.Set("filterCreator", p.FilterCreator)
	}
	if p.TimeFrame != "" {
		q.Set("timeFrame", p.TimeFrame)
	}
	return authedGET("/cape/browse", q, token)
}

func PlayerCapes(playerID, tokenOpt string) (json.RawMessage, error) {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return nil, err
	}
	if playerID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			playerID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", playerID)
	return authedGET("/cape/player/"+url.PathEscape(playerID), q, token)
}

func Equip(capeHash, tokenOpt, playerUUID string) error {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return err
	}
	if playerUUID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			playerUUID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", playerUUID)
	_, err = authedPOST("/cape/equip/"+url.PathEscape(capeHash), q, token, nil)
	return err
}

func Unequip(tokenOpt, playerUUID string) error {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return err
	}
	if playerUUID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			playerUUID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", playerUUID)
	_, err = authedPOST("/cape/unequip", q, token, nil)
	return err
}

func DeleteCape(hash, tokenOpt, playerUUID string) error {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return err
	}
	if playerUUID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			playerUUID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", playerUUID)
	_, err = authed("DELETE", "/cape/"+url.PathEscape(hash), q, token, nil, "")
	return err
}

func Favorite(hash, tokenOpt string, add bool) (json.RawMessage, error) {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return nil, err
	}
	method := http.MethodPut
	if !add {
		method = http.MethodDelete
	}
	return authed(method, "/cape/favorite/"+url.PathEscape(hash), nil, token, nil, "")
}

func ByHashes(hashes []string, tokenOpt string) (json.RawMessage, error) {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("hash", strings.Join(hashes, ","))
	return authed(http.MethodGet, "/cape/many", q, token, nil, "")
}

func UploadPNG(png []byte, tokenOpt, playerUUID string) (json.RawMessage, error) {
	token, err := resolveToken(tokenOpt)
	if err != nil {
		return nil, err
	}
	if playerUUID == "" {
		if acc, _ := accounts.Active(); acc != nil {
			playerUUID = acc.ID
		}
	}
	q := url.Values{}
	q.Set("uuid", playerUUID)
	raw, err := authed(http.MethodPost, "/cape", q, token, png, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	return normalizeUploadResponse(raw)
}

func normalizeUploadResponse(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		if _, ok := obj["capeHash"]; ok {
			return raw, nil
		}
	}
	hash := strings.TrimSpace(string(raw))
	hash = strings.Trim(hash, `"`)
	if hash == "" {
		return nil, fmt.Errorf("upload cape: respuesta vacía")
	}
	return json.Marshal(map[string]any{
		"capeHash":           hash,
		"wasResized":         false,
		"originalDimensions": nil,
	})
}

func authedGET(path string, q url.Values, token string) (json.RawMessage, error) {
	return authed(http.MethodGet, path, q, token, nil, "")
}

func authed(method, path string, q url.Values, token string, body []byte, ctype string) (json.RawMessage, error) {
	u := baseURL() + path
	if q != nil {
		u += "?" + q.Encode()
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails)")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("capes %s %s: %s — %s", method, path, res.Status, string(raw))
	}
	return json.RawMessage(raw), nil
}

func authedPOST(path string, q url.Values, token string, body any) (json.RawMessage, error) {
	u := baseURL() + path
	if q != nil {
		u += "?" + q.Encode()
	}
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PixelPlayLauncher/0.2 (wails)")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("capes POST %s: %s — %s", path, res.Status, string(raw))
	}
	return json.RawMessage(raw), nil
}
