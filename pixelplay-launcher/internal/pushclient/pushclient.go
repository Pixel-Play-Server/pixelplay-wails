package pushclient

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Packet from the Pixel push hub.
type Packet struct {
	Type    string `json:"type"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	URL     string `json:"url,omitempty"`
	Notes   string `json:"notes,omitempty"`
	Message string `json:"message,omitempty"`
	TS      int64  `json:"ts,omitempty"`
}

// Handler is called for each push packet.
type Handler func(Packet)

func defaultURLs() []string {
	if v := strings.TrimSpace(os.Getenv("PIXELPLAY_PUSH_URL")); v != "" {
		return []string{v}
	}
	return []string{
		"wss://panel.pixelplay.gg/launcher-push/ws",
		"ws://100.117.225.55:8790/ws", // fallback LAN/Tailscale
		"ws://127.0.0.1:8790/ws",
	}
}

// Run connects (with reconnect) to the Pixel push hub.
func Run(ctx context.Context, onPacket Handler) {
	urls := defaultURLs()
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		connected := false
		for _, u := range urls {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[push] trying %s", u)
			if err := dialOnce(ctx, u, onPacket); err != nil {
				log.Printf("[push] %s: %v", u, err)
				continue
			}
			connected = true
			backoff = time.Second
			break
		}
		if !connected {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
			}
		}
	}
}

func dialOnce(ctx context.Context, url string, onPacket Handler) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 8 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	c, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return err
	}
	defer c.Close()
	log.Printf("[push] connected %s", url)

	// ping loop
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-t.C:
				_ = c.WriteJSON(Packet{Type: "ping", TS: time.Now().Unix()})
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, raw, err := c.ReadMessage()
		if err != nil {
			return err
		}
		var p Packet
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.Type == "pong" || p.Type == "hello" {
			continue
		}
		if onPacket != nil {
			onPacket(p)
		}
	}
}
