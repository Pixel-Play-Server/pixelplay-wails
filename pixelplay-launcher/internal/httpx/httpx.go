package httpx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const UserAgent = "PixelPlayLauncher/5.0 (wails; contact@pixelplay.gg)"

var Client = &http.Client{Timeout: 60 * time.Second}

func Get(rawURL string) ([]byte, error) {
	return GetHeader(rawURL, nil)
}

func GetHeader(rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json, */*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		msg := string(b)
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return nil, fmt.Errorf("GET %s: %s — %s", rawURL, res.Status, msg)
	}
	return b, nil
}

func GetJSON(rawURL string, dest any) error {
	return GetJSONHeader(rawURL, nil, dest)
}

func GetJSONHeader(rawURL string, headers map[string]string, dest any) error {
	b, err := GetHeader(rawURL, headers)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func PostJSONHeader(rawURL string, headers map[string]string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		msg := string(b)
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		return fmt.Errorf("POST %s: %s — %s", rawURL, res.Status, msg)
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func PostForm(rawURL string, form url.Values, dest any) error {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		msg := string(b)
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		return fmt.Errorf("POST %s: %s — %s", rawURL, res.Status, msg)
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func PostJSON(rawURL string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		msg := string(b)
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		return fmt.Errorf("POST %s: %s — %s", rawURL, res.Status, msg)
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(b, dest)
}
