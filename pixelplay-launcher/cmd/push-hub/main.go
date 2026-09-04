package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Packet struct {
	Type    string `json:"type"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	URL     string `json:"url,omitempty"`
	Notes   string `json:"notes,omitempty"`
	Message string `json:"message,omitempty"`
	TS      int64  `json:"ts,omitempty"`
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: map[*websocket.Conn]struct{}{}}
}

func (h *Hub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n := len(h.clients)
	h.mu.Unlock()
	log.Printf("client +1 (total=%d)", n)
}

func (h *Hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	n := len(h.clients)
	h.mu.Unlock()
	_ = c.Close()
	log.Printf("client -1 (total=%d)", n)
}

func (h *Hub) broadcast(p Packet) int {
	if p.TS == 0 {
		p.TS = time.Now().Unix()
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	sent := 0
	for c := range h.clients {
		if err := c.WriteMessage(websocket.TextMessage, raw); err != nil {
			log.Printf("write failed, dropping client: %v", err)
			delete(h.clients, c)
			_ = c.Close()
			continue
		}
		sent++
	}
	return sent
}

func (h *Hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	addr := flag.String("addr", ":8790", "listen address")
	token := flag.String("token", "pixelplay-push", "shared token for /broadcast")
	flag.Parse()

	hub := NewHub()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"clients": hub.count(),
			"service": "pixelplay-launcher-push",
		})
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade: %v", err)
			return
		}
		hub.add(c)
		defer hub.remove(c)
		// hello
		_ = c.WriteJSON(Packet{
			Type:    "hello",
			Message: "connected to pixel play push hub",
			TS:      time.Now().Unix(),
		})
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			var p Packet
			if json.Unmarshal(msg, &p) == nil && p.Type == "ping" {
				_ = c.WriteJSON(Packet{Type: "pong", TS: time.Now().Unix()})
			}
		}
	})

	mux.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if t := r.Header.Get("X-Pixel-Token"); t != *token && r.URL.Query().Get("token") != *token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var p Packet
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if p.Type == "" {
			p.Type = "update_available"
		}
		n := hub.broadcast(p)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "sent": n, "packet": p})
		log.Printf("broadcast type=%s to=%s sent=%d", p.Type, p.To, n)
	})

	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"clients": hub.count()})
	})

	log.Printf("pixelplay push hub on %s (ws=/ws broadcast=/broadcast)", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Println(err)
	}
}
