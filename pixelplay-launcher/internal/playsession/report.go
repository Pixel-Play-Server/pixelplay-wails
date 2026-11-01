package playsession

import (
	"strings"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
)

const playAPI = "https://api.pixelplay.gg/api/v1/launcher/play-session"

type Event struct {
	Type     string
	Host     string
	Pack     string
	Profile  string
	Player   string
	Seconds  int64
	Sessions int64
}

func Report(ev Event) {
	go func() {
		_ = send(ev)
	}()
}

func send(ev Event) error {
	player := strings.TrimSpace(ev.Player)
	headers := map[string]string{}
	if acc, err := accounts.Active(); err == nil && acc != nil {
		if player == "" {
			player = acc.Username
		}
		if tok := acc.PixelToken(); tok != "" {
			headers["Authorization"] = "Bearer " + tok
		}
	}
	if player == "" && ev.Host == "" {
		return nil
	}
	body := map[string]any{
		"event":    ev.Type,
		"host":     ev.Host,
		"pack":     ev.Pack,
		"profile":  ev.Profile,
		"player":   player,
		"seconds":  ev.Seconds,
		"sessions": ev.Sessions,
	}
	return httpx.PostJSONHeader(playAPI, headers, body, nil)
}
