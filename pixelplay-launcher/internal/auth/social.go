package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
)

type PixelLookup struct {
	Exists   bool   `json:"exists"`
	Username string `json:"username"`
	Kind     string `json:"kind,omitempty"`
	Rank     string `json:"rank,omitempty"`
}

func LookupPixel(username string) (PixelLookup, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return PixelLookup{}, fmt.Errorf("faltó el usuario")
	}
	rawURL := pixelplayProdAPI + "/auth/pixel/lookup?username=" + url.QueryEscape(username)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return PixelLookup{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return PixelLookup{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return PixelLookup{}, err
	}
	if res.StatusCode >= 400 {
		return PixelLookup{}, fmt.Errorf("cuenta Pixel: %s", apiError(b, res.Status))
	}
	var out PixelLookup
	if err := json.Unmarshal(b, &out); err != nil {
		return PixelLookup{}, fmt.Errorf("respuesta Pixel inválida")
	}
	if out.Username == "" {
		out.Username = username
	}
	return out, nil
}

func ReservePixel(username, password string) (accounts.Credentials, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return accounts.Credentials{}, fmt.Errorf("faltan usuario o clave Pixel")
	}
	payload, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
		"kind":     "cracked",
	})
	req, err := http.NewRequest(http.MethodPost, pixelplayProdAPI+"/auth/pixel/reserve", strings.NewReader(string(payload)))
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
	return accounts.CreatePixel(name, id, out.Value, out.Account.Rank, firstNonEmpty(out.Account.Kind, "cracked"))
}

func requirePixelToken() (string, error) {
	token, _, err := ActivePixelSession()
	return token, err
}

// ActivePixelSession reads token and identity from the same account snapshot.
func ActivePixelSession() (token, username string, err error) {
	acc, err := accounts.Active()
	if err != nil {
		return "", "", err
	}
	if acc == nil {
		return "", "", fmt.Errorf("no hay una cuenta activa")
	}
	if !acc.PixelAccount() {
		return "", "", fmt.Errorf("las amistades son solo para cuentas Pixel")
	}
	refreshed, err := EnsurePixelplay(*acc)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo renovar la sesión Pixel: %w", err)
	}
	live, err := accounts.Active()
	if err != nil {
		return "", "", err
	}
	if live == nil || !sameAccountID(live.ID, refreshed.ID) {
		return "", "", fmt.Errorf("la cuenta activa cambió; vuelve a intentarlo")
	}
	if refreshed.PixelToken() == "" {
		return "", "", fmt.Errorf("sesión Pixel vencida — vuelve a iniciar sesión")
	}
	return refreshed.PixelToken(), strings.TrimSpace(refreshed.Username), nil
}

func RefreshPixelProfile() (map[string]any, error) {
	acc, err := accounts.Active()
	if err != nil || acc == nil {
		return nil, fmt.Errorf("no hay una cuenta activa")
	}
	if !acc.PixelAccount() || acc.PixelToken() == "" {
		return map[string]any{"changed": false, "rank": acc.Rank}, nil
	}
	var me struct {
		UUID      string   `json:"uuid"`
		Username  string   `json:"username"`
		Rank      string   `json:"rank"`
		RankLabel string   `json:"rankLabel"`
		RankColor string   `json:"rankColor"`
		Perks     []string `json:"perks"`
		Kind      string   `json:"kind"`
	}
	if err := pixelJSON(http.MethodGet, "/auth/pixel/me", acc.PixelToken(), nil, &me); err != nil {
		return nil, err
	}
	prev := strings.TrimSpace(acc.Rank)
	if prev == "" {
		prev = "usuario"
	}
	next := strings.TrimSpace(me.Rank)
	if next == "" {
		next = "usuario"
	}
	changed := !strings.EqualFold(prev, next)
	if changed || acc.Rank == "" {
		acc.Rank = next
		if me.Kind != "" {
			acc.Kind = me.Kind
		}
		if saved, err := accounts.Upsert(*acc); err == nil {
			acc = &saved
		}
	}
	label, color := accounts.RankMeta(next)
	if me.RankLabel != "" {
		label = me.RankLabel
	}
	if me.RankColor != "" {
		color = me.RankColor
	}
	return map[string]any{
		"changed":    changed,
		"previous":   prev,
		"rank":       next,
		"rank_label": label,
		"rank_color": color,
		"perks":      me.Perks,
		"username":   firstNonEmpty(me.Username, acc.Username),
		"uuid":       firstNonEmpty(me.UUID, acc.ID),
		"account":    acc.ForUI(),
	}, nil
}

func ListFriends() (map[string]any, error) {
	tok, err := requirePixelToken()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := pixelJSON(http.MethodGet, "/friends", tok, nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func AddFriend(username string) (map[string]any, error) {
	tok, err := requirePixelToken()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := pixelJSON(http.MethodPost, "/friends", tok, map[string]string{"username": strings.TrimSpace(username)}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func RemoveFriend(username string) error {
	tok, err := requirePixelToken()
	if err != nil {
		return err
	}
	return pixelJSON(http.MethodPost, "/friends/remove", tok, map[string]string{"username": strings.TrimSpace(username)}, nil)
}

func AcceptFriend(username string) (map[string]any, error) {
	tok, err := requirePixelToken()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := pixelJSON(http.MethodPost, "/friends/accept", tok, map[string]string{"username": strings.TrimSpace(username)}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func DeclineFriend(username string) error {
	tok, err := requirePixelToken()
	if err != nil {
		return err
	}
	return pixelJSON(http.MethodPost, "/friends/decline", tok, map[string]string{"username": strings.TrimSpace(username)}, nil)
}

func CancelFriend(username string) error {
	tok, err := requirePixelToken()
	if err != nil {
		return err
	}
	return pixelJSON(http.MethodPost, "/friends/cancel", tok, map[string]string{"username": strings.TrimSpace(username)}, nil)
}

func SetActivePresence(status string) (map[string]any, error) {
	acc, err := accounts.Active()
	if err != nil || acc == nil || !acc.PixelAccount() {
		return map[string]any{"ok": false, "status": "offline"}, nil
	}
	tok := acc.PixelToken()
	if tok == "" {
		return map[string]any{"ok": false, "status": "offline"}, nil
	}
	var out map[string]any
	if err := pixelJSON(http.MethodPost, "/presence", tok, map[string]string{"status": status}, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{"ok": true, "status": status}
	}
	return out, nil
}

func optionalPixelToken() string {
	acc, err := accounts.Active()
	if err != nil || acc == nil || !acc.PixelAccount() {
		return ""
	}
	return acc.PixelToken()
}

func ListSurvivalBoard(sort string) (map[string]any, error) {
	path := "/survival/leaderboard"
	if sort = strings.TrimSpace(sort); sort != "" {
		path += "?sort=" + url.QueryEscape(sort)
	}
	var out map[string]any
	if err := pixelJSON(http.MethodGet, path, optionalPixelToken(), nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func GetSurvivalPlayer(username string) (map[string]any, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("faltó el usuario")
	}
	var out map[string]any
	path := "/survival/player?username=" + url.QueryEscape(username)
	if err := pixelJSON(http.MethodGet, path, optionalPixelToken(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func pixelJSON(method, path, token string, payload any, dest any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, pixelplayProdAPI+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("%s", apiError(b, res.Status))
	}
	if dest == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dest)
}
