package vanillacapes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
)

type Cape struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	URL         string  `json:"url"`
	Equipped    bool    `json:"equipped"`
	ObtainedAt  *int64  `json:"obtained_at"`
	Category    string  `json:"category"`
	Active      bool    `json:"active"`
}

type Info struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	PreviewURL   string  `json:"preview_url"`
	Category     string  `json:"category"`
	Obtainable   bool    `json:"obtainable"`
	ObtainMethod *string `json:"obtain_method"`
}

func activeToken() (string, error) {
	acc, err := accounts.Active()
	if err != nil {
		return "", err
	}
	if acc == nil || strings.TrimSpace(acc.AccessToken) == "" {
		return "", fmt.Errorf("no hay cuenta de Minecraft activa")
	}
	return acc.AccessToken, nil
}

func Owned() ([]Cape, error) {
	tok, err := activeToken()
	if err != nil {
		return []Cape{}, err
	}
	return ownedWith(tok)
}

func Equipped() (*Cape, error) {
	list, err := Owned()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Equipped {
			c := list[i]
			return &c, nil
		}
	}
	return nil, nil
}

func Equip(capeID string) error {
	tok, err := activeToken()
	if err != nil {
		return err
	}
	capeID = strings.TrimSpace(capeID)
	var body []byte
	if capeID == "" {
		req, err := http.NewRequest(http.MethodDelete, "https://api.minecraftservices.com/minecraft/profile/capes/active", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", httpx.UserAgent)
		res, err := httpx.Client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 400 {
			return fmt.Errorf("quitar capa: %s — %s", res.Status, trunc(string(b)))
		}
		return nil
	}
	body, _ = json.Marshal(map[string]any{"capeId": capeID})
	req, err := http.NewRequest(http.MethodPut, "https://api.minecraftservices.com/minecraft/profile/capes/active", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return fmt.Errorf("equipar capa: %s — %s", res.Status, trunc(string(b)))
	}
	return nil
}

func Catalog() []Info {
	return []Info{}
}

func ownedWith(tok string) ([]Cape, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.minecraftservices.com/minecraft/profile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
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
		return nil, fmt.Errorf("perfil Minecraft: %s — %s", res.Status, trunc(string(b)))
	}
	var prof struct {
		Capes []struct {
			ID      string `json:"id"`
			State   string `json:"state"`
			URL     string `json:"url"`
			Alias   string `json:"alias"`
		} `json:"capes"`
	}
	if err := json.Unmarshal(b, &prof); err != nil {
		return nil, err
	}
	out := make([]Cape, 0, len(prof.Capes))
	for _, c := range prof.Capes {
		active := strings.EqualFold(c.State, "ACTIVE")
		name := c.Alias
		if name == "" {
			name = c.ID
		}
		out = append(out, Cape{
			ID:       c.ID,
			Name:     name,
			URL:      c.URL,
			Equipped: active,
			Category: "vanilla",
			Active:   active,
		})
	}
	return out, nil
}

func trunc(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
