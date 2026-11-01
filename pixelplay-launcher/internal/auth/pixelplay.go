package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/config"
	"pixelplay-launcher/internal/httpx"
)

const (
	pixelplayProdAPI = "https://api.pixelplay.gg/api/v1"
	pixelplayExpAPI  = "https://api-staging.pixelplay.gg/api/v1"
	mojangJoinURL    = "https://sessionserver.mojang.com/session/minecraft/join"
)

var pixelplayMu sync.Mutex

// EnsurePixelplay refreshes the Minecraft token if needed and obtains a Pixel Play JWT
// when the stored one is missing, expired, or for another username.
func LoginPixel(username, password string) (accounts.Credentials, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return accounts.Credentials{}, fmt.Errorf("faltan usuario o clave Pixel")
	}
	payload, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	req, err := http.NewRequest(http.MethodPost, pixelplayProdAPI+"/auth/pixel/login", strings.NewReader(string(payload)))
	if err != nil {
		return accounts.Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return accounts.Credentials{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return accounts.Credentials{}, err
	}
	if res.StatusCode >= 400 {
		return accounts.Credentials{}, fmt.Errorf("cuenta Pixel: %s", apiError(b, res.Status))
	}
	var out struct {
		Value   string `json:"value"`
		Account struct {
			UUID     string `json:"uuid"`
			Username string `json:"username"`
			Rank     string `json:"rank"`
			Kind     string `json:"kind"`
		} `json:"account"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.Value == "" {
		return accounts.Credentials{}, fmt.Errorf("respuesta Pixel inválida")
	}
	name := out.Account.Username
	if name == "" {
		name = username
	}
	id := out.Account.UUID
	if id == "" {
		id = name
	}
	return accounts.CreatePixel(name, id, out.Value, out.Account.Rank, firstNonEmpty(out.Account.Kind, "pixel"))
}

func apiError(b []byte, status string) string {
	var parsed map[string]any
	if json.Unmarshal(b, &parsed) == nil {
		if s, ok := parsed["error"].(string); ok && s != "" {
			return s
		}
	}
	return status + " — " + truncateBody(b)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func applyPixelClaims(c accounts.Credentials) accounts.Credentials {
	tok := c.PixelToken()
	if tok == "" {
		return c
	}
	if rank, ok := jwtClaimString(tok, "rank"); ok {
		c.Rank = rank
	} else if strings.TrimSpace(c.Rank) == "" && c.PixelAccount() {
		c.Rank = "usuario"
	}
	if kind, ok := jwtClaimString(tok, "kind"); ok {
		c.Kind = kind
	}
	return c
}

func persistPixelClaims(c accounts.Credentials) (accounts.Credentials, error) {
	updated := applyPixelClaims(c)
	if updated.Rank == c.Rank && updated.Kind == c.Kind {
		return updated, nil
	}
	return accounts.Upsert(updated)
}

func EnsurePixelplay(c accounts.Credentials) (accounts.Credentials, error) {
	expMode := experimentalMode()

	// Check if existing token is valid and fresh
	if pixelplayTokenOK(c, expMode) {
		return persistPixelClaims(c)
	}

	if c.Offline() && !c.PixelPasswordAccount() {
		return c, fmt.Errorf("capas y funciones de Pixel Play requieren cuenta Microsoft o Pixel")
	}

	pixelplayMu.Lock()
	defer pixelplayMu.Unlock()

	if live, err := accounts.Active(); err == nil && live != nil && sameAccountID(live.ID, c.ID) {
		c = *live
	}

	refreshed, err := RefreshMinecraftIfNeeded(c)
	if err != nil {
		return c, err
	}
	c = refreshed

	if pixelplayTokenOK(c, expMode) {
		return persistPixelClaims(c)
	}

	value, err := refreshPixelplayToken(c, expMode)
	if err != nil {
		return c, err
	}
	tok := &accounts.Token{Value: value}
	if expMode {
		c.PixelplayCredentials.Experimental = tok
	} else {
		c.PixelplayCredentials.Production = tok
	}
	c = applyPixelClaims(c)
	return accounts.Upsert(c)
}

// AutoRefreshAllAccountTokens iterates over all accounts and silently renews any expired tokens.
func AutoRefreshAllAccountTokens() {
	list, err := accounts.List()
	if err != nil || len(list) == 0 {
		return
	}
	for _, acc := range list {
		if !acc.Offline() {
			_, _ = EnsurePixelplay(acc)
		}
	}
}

func pixelplayTokenOK(c accounts.Credentials, experimental bool) bool {
	tok := c.PixelplayCredentials.Production
	if experimental {
		tok = c.PixelplayCredentials.Experimental
		if tok == nil || tok.Value == "" {
			tok = c.PixelplayCredentials.Production
		}
	}
	if tok == nil || strings.TrimSpace(tok.Value) == "" {
		return false
	}
	if strings.Contains(tok.Value, "sig_pixelplay_auto_refreshed") {
		return false
	}
	if !jwtFresh(tok.Value, 2*time.Minute) {
		return false
	}
	if name, ok := jwtClaimString(tok.Value, "username"); ok && !strings.EqualFold(name, c.Username) {
		return false
	}
	if _, ok := jwtClaimString(tok.Value, "rank"); !ok {
		return false
	}
	return true
}

func experimentalMode() bool {
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

func pixelplayAPIBase(experimental bool) string {
	if experimental {
		return pixelplayExpAPI
	}
	return pixelplayProdAPI
}

func refreshPixelplayToken(c accounts.Credentials, experimental bool) (string, error) {
	serverID, err := requestServerID(experimental)
	if err != nil {
		return "", err
	}
	profile := strings.ReplaceAll(c.ID, "-", "")
	if err := joinMinecraftSession(c.AccessToken, profile, serverID); err != nil {
		refreshed, rerr := refreshMinecraft(c)
		if rerr != nil {
			return "", fmt.Errorf("sesión Minecraft (join): %w", err)
		}
		c = refreshed
		if err2 := joinMinecraftSession(c.AccessToken, profile, serverID); err2 != nil {
			return "", fmt.Errorf("sesión Minecraft (join): %w", err2)
		}
	}
	return validatePixelplay(c.Username, serverID, experimental)
}

func requestServerID(experimental bool) (string, error) {
	rawURL := pixelplayAPIBase(experimental) + "/launcher/auth/request-server-id"
	req, err := http.NewRequest(http.MethodPost, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("request-server-id: %s — %s", res.Status, truncateBody(b))
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		return "", fmt.Errorf("request-server-id json: %w", err)
	}
	id := mapString(parsed, "serverId", "server_id")
	if id == "" || !strings.HasPrefix(id, "nrc-") {
		return "", fmt.Errorf("server id inválido: %s", id)
	}
	return id, nil
}

func joinMinecraftSession(accessToken, selectedProfile, serverID string) error {
	payload := map[string]string{
		"accessToken":     accessToken,
		"selectedProfile": selectedProfile,
		"serverId":        serverID,
	}
	return httpx.PostJSON(mojangJoinURL, payload, nil)
}

func validatePixelplay(username, serverID string, experimental bool) (string, error) {
	u, err := url.Parse(pixelplayAPIBase(experimental) + "/launcher/auth/validate/v2")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("force", "true")
	q.Set("hwid", DeviceSaltedID())
	q.Set("username", username)
	q.Set("server_id", serverID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodPost, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("pixelplay validate/v2: %s — %s", res.Status, truncateBody(b))
	}
	var tok struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(b, &tok); err != nil {
		var asString string
		if json.Unmarshal(b, &asString) == nil && asString != "" {
			return asString, nil
		}
		return "", fmt.Errorf("pixelplay token json: %w", err)
	}
	if strings.TrimSpace(tok.Value) == "" {
		return "", fmt.Errorf("pixelplay validate/v2 no devolvió token")
	}
	return tok.Value, nil
}

func mapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func truncateBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

func sameAccountID(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}
