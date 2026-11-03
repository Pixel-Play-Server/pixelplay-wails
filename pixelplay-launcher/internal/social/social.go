// Package social keeps a live connection to pixel-social (chat, calls, voice relay).
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/httpx"
	"pixelplay-launcher/internal/nativertc"
	"pixelplay-launcher/internal/voice"
)

const (
	httpBase = "https://api.pixelplay.gg/api/v1/social"
	wsURL    = "wss://api.pixelplay.gg/api/v1/social/ws?device=desktop"
)

type Peer struct {
	UUID      string `json:"uuid"`
	Username  string `json:"username"`
	RankLabel string `json:"rankLabel,omitempty"`
	RankColor string `json:"rankColor,omitempty"`
}

type CallState struct {
	ID        string `json:"id"`
	Peer      Peer   `json:"peer"`
	Direction string `json:"direction"` // in | out
	State     string `json:"state"`     // ringing | active
	Since     int64  `json:"since,omitempty"`
	Muted     bool   `json:"muted"`
	Deafened  bool   `json:"deafened"`
	PeerMuted bool   `json:"peerMuted"`
	AudioErr  string `json:"audioError,omitempty"`
}

type Client struct {
	emit func(string, any)

	mu        sync.Mutex
	writeMu   sync.Mutex
	conn      *websocket.Conn
	user      string
	connected bool
	epoch     uint64
	call      *CallState
	wake      chan struct{}
	engine    voice.Engine
	stopLevel chan struct{}
	native    nativertc.Engine
}

var (
	defaultMu sync.Mutex
	current   *Client
)

// Start launches the connection loop; Default() returns it for IPC handlers.
func Start(ctx context.Context, emit func(string, any)) *Client {
	c := &Client{emit: emit, wake: make(chan struct{}, 1), engine: voice.New()}
	defaultMu.Lock()
	current = c
	defaultMu.Unlock()
	go c.loop(ctx)
	return c
}

func Default() *Client {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return current
}

// Reconnect drops the socket so the loop picks up the (new) active account.
func (c *Client) Reconnect() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.epoch++
	c.mu.Unlock()
	c.setConnected(false, "")
	if conn != nil {
		_ = conn.Close()
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) loop(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		c.mu.Lock()
		epoch := c.epoch
		c.mu.Unlock()
		token, user, err := auth.ActivePixelSession()
		if err != nil {
			c.setConnected(false, "")
			c.sleep(ctx, 15*time.Second)
			continue
		}
		hdr := http.Header{}
		hdr.Set("Authorization", "Bearer "+token)
		hdr.Set("User-Agent", httpx.UserAgent)
		dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
		conn, _, err := dialer.DialContext(ctx, wsURL, hdr)
		if err != nil {
			c.setConnected(false, "")
			c.sleep(ctx, backoff)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		c.mu.Lock()
		if epoch != c.epoch {
			c.mu.Unlock()
			_ = conn.Close()
			continue
		}
		c.conn, c.user = conn, user
		c.mu.Unlock()

		done := make(chan struct{})
		go c.watchAccount(ctx, conn, user, done)
		c.read(conn)
		_ = conn.Close()
		close(done)

		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		c.endLocal("disconnected")
		c.setConnected(false, "")
		c.sleep(ctx, backoff)
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c *Client) sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	case <-c.wake:
	}
}

func (c *Client) watchAccount(ctx context.Context, conn *websocket.Conn, user string, done chan struct{}) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			_ = conn.Close()
			return
		case <-t.C:
			_, now, err := auth.ActivePixelSession()
			if err != nil || !strings.EqualFold(now, user) {
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *Client) setConnected(v bool, user string) {
	c.mu.Lock()
	changed := c.connected != v || c.user != user
	c.connected, c.user = v, user
	c.mu.Unlock()
	if changed {
		c.emit("social_event", map[string]any{"t": "status", "connected": v, "user": user})
	}
}

func (c *Client) read(conn *websocket.Conn) {
	conn.SetReadLimit(64 * 1024)
	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if typ == websocket.BinaryMessage {
			// Calls use WebRTC only; discard obsolete PCM frames.
			continue
		}
		var ev map[string]any
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		c.mu.Lock()
		active := c.conn == conn
		c.mu.Unlock()
		if !active {
			return
		}
		c.handle(ev)
	}
}

func peerFrom(v any) Peer {
	m, _ := v.(map[string]any)
	s := func(k string) string { x, _ := m[k].(string); return x }
	return Peer{UUID: s("uuid"), Username: s("username"), RankLabel: s("rankLabel"), RankColor: s("rankColor")}
}

func (c *Client) handle(ev map[string]any) {
	t, _ := ev["t"].(string)
	switch t {
	case "hello":
		me := peerFrom(ev["me"])
		c.setConnected(true, me.Username)
	case "call":
		// Calls removed from launcher — auto-decline inbound invites.
		if action, _ := ev["action"].(string); action == "invite" {
			_ = c.writeJSON(map[string]any{"t": "call", "action": "decline", "call": ev["call"]})
			return
		}
		c.onCall(ev)
	}
	c.emit("social_event", ev)
}

func (c *Client) onCall(ev map[string]any) {
	action, _ := ev["action"].(string)
	id, _ := ev["call"].(string)
	switch action {
	case "invite":
		c.setCall(&CallState{ID: id, Peer: peerFrom(ev["from"]), Direction: "in", State: "ringing"})
	case "ringing":
		c.setCall(&CallState{ID: id, Peer: peerFrom(ev["to"]), Direction: "out", State: "ringing"})
	case "active":
		since, _ := ev["since"].(float64)
		c.mu.Lock()
		call := c.call
		if call == nil || call.ID != id {
			call = &CallState{ID: id, Peer: peerFrom(ev["peer"])}
		}
		call.State, call.Since = "active", int64(since)
		c.call = call
		c.mu.Unlock()
		c.startAudio()
	case "peer-mute":
		muted, _ := ev["muted"].(bool)
		c.mu.Lock()
		if c.call != nil {
			c.call.PeerMuted = muted
		}
		c.mu.Unlock()
		c.emitCall()
	case "ended", "answered-elsewhere", "busy-elsewhere", "busy", "unavailable", "failed":
		c.mu.Lock()
		match := c.call != nil && (id == "" || c.call.ID == id)
		c.mu.Unlock()
		if match {
			c.endLocal(action)
		}
	}
}

func (c *Client) setCall(s *CallState) {
	c.mu.Lock()
	c.call = s
	c.mu.Unlock()
	c.emitCall()
}

func (c *Client) emitCall() {
	c.emit("social_call", c.CallSnapshot())
}

func (c *Client) CallSnapshot() *CallState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.call == nil {
		return nil
	}
	cp := *c.call
	return &cp
}

func (c *Client) startAudio() {
	c.emit("social_event", map[string]any{"t": "webrtc", "action": "start", "call": c.CallSnapshot()})
	c.emitCall()
}

// PCM is disabled: calls keep the WebRTC connection when ICE needs more time.
func (c *Client) StartPCMFallback() error {
	return fmt.Errorf("el relay PCM está deshabilitado; usa WebRTC")
}

func (c *Client) endLocal(reason string) {
	c.native.Stop()
	c.mu.Lock()
	had := c.call != nil
	c.call = nil
	if c.stopLevel != nil {
		close(c.stopLevel)
		c.stopLevel = nil
	}
	c.mu.Unlock()
	c.engine.Stop()
	if had {
		c.emit("social_call", nil)
	}
}

func (c *Client) writeJSON(v any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("sin conexión con el servidor social de Pixel")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(v)
}

func (c *Client) writeBinary(b []byte) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	c.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = conn.WriteMessage(websocket.BinaryMessage, b)
	c.writeMu.Unlock()
}

// --- Actions used by IPC ---

func (c *Client) Status() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var call *CallState
	if c.call != nil {
		cp := *c.call
		call = &cp
	}
	return map[string]any{"connected": c.connected, "user": c.user, "call": call}
}

func (c *Client) SendMessage(to, text, cid string) error {
	to, text, cid = strings.TrimSpace(to), strings.TrimSpace(text), strings.TrimSpace(cid)
	if to == "" || text == "" || cid == "" {
		return fmt.Errorf("faltan destinatario, mensaje o identificador")
	}
	if len([]rune(text)) > 2000 || len(cid) > 128 {
		return fmt.Errorf("mensaje demasiado largo")
	}
	acc, err := accounts.Active()
	if err != nil {
		return err
	}
	c.mu.Lock()
	user, connected := c.user, c.connected
	c.mu.Unlock()
	if acc == nil || !acc.PixelAccount() || !connected || !strings.EqualFold(acc.Username, user) {
		return fmt.Errorf("el chat está reconectando; vuelve a intentarlo")
	}
	return c.writeJSON(map[string]any{"t": "msg", "to": to, "text": text, "cid": cid})
}

func (c *Client) MarkRead(with string) error {
	return c.writeJSON(map[string]any{"t": "read", "with": with})
}

func (c *Client) Typing(to string) error {
	return c.writeJSON(map[string]any{"t": "typing", "to": to})
}

func (c *Client) Invite(to string) error {
	if c.CallSnapshot() != nil {
		return fmt.Errorf("ya estás en una llamada")
	}
	return c.writeJSON(map[string]any{"t": "call", "action": "invite", "to": to})
}

func (c *Client) Accept() error {
	call := c.CallSnapshot()
	if call == nil || call.Direction != "in" {
		return fmt.Errorf("no hay llamada entrante")
	}
	return c.writeJSON(map[string]any{"t": "call", "action": "accept", "call": call.ID})
}

// Hangup declines, cancels or ends depending on the call state.
func (c *Client) Hangup() error {
	call := c.CallSnapshot()
	if call == nil {
		return nil
	}
	action := "end"
	if call.State == "ringing" {
		action = "cancel"
		if call.Direction == "in" {
			action = "decline"
		}
	}
	err := c.writeJSON(map[string]any{"t": "call", "action": action, "call": call.ID})
	c.endLocal(action)
	return err
}

func (c *Client) SetMuted(v bool) {
	c.engine.SetMuted(v)
	c.mu.Lock()
	var id string
	if c.call != nil {
		c.call.Muted = v
		id = c.call.ID
	}
	c.mu.Unlock()
	if id != "" {
		_ = c.writeJSON(map[string]any{"t": "call", "action": "mute", "call": id, "muted": v})
	}
	c.emitCall()
}

func (c *Client) SetDeafened(v bool) {
	c.engine.SetDeafened(v)
	c.mu.Lock()
	if c.call != nil {
		c.call.Deafened = v
	}
	c.mu.Unlock()
	c.emitCall()
}

// SendWebRTC forwards SDP/ICE signaling to the peer through the social hub.
func (c *Client) SendWebRTC(action, sdp, candidate, sdpMid string, sdpMLine *int) error {
	call := c.CallSnapshot()
	if call == nil || call.State != "active" {
		return fmt.Errorf("no hay llamada activa")
	}
	msg := map[string]any{"t": "webrtc", "action": action, "call": call.ID}
	if sdp != "" {
		msg["sdp"] = sdp
	}
	if candidate != "" {
		msg["candidate"] = candidate
	}
	if sdpMid != "" {
		msg["sdpMid"] = sdpMid
	}
	if sdpMLine != nil {
		msg["sdpMLineIndex"] = *sdpMLine
	}
	return c.writeJSON(msg)
}

// ReportMicLevel is used by the frontend WebRTC path so the overlay meter still moves.
func (c *Client) ReportMicLevel(level float32) {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	c.emit("social_voice_level", map[string]any{"mic": level})
}

func (c *Client) History(with string, before int64) (any, error) {
	q := url.Values{"with": {with}, "limit": {"80"}}
	if before > 0 {
		q.Set("before", strconv.FormatInt(before, 10))
	}
	return getJSON("/history?" + q.Encode())
}

func (c *Client) Unread() (any, error) { return getJSON("/unread") }

func getJSON(path string) (any, error) {
	token, _, err := auth.ActivePixelSession()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, httpBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	res, err := httpx.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("social: respuesta inválida (%s)", res.Status)
	}
	if res.StatusCode >= 400 {
		if m, ok := out.(map[string]any); ok {
			if e, ok := m["error"].(string); ok {
				return nil, fmt.Errorf("%s", e)
			}
		}
		return nil, fmt.Errorf("social: %s", res.Status)
	}
	return out, nil
}

func (c *Client) NativeStart(settings map[string]any) error {
	call := c.CallSnapshot()
	if call == nil || call.State != "active" {
		return fmt.Errorf("no hay llamada activa")
	}
	settings["action"] = "start"
	settings["call"] = call.ID
	settings["direction"] = call.Direction
	settings["muted"] = call.Muted
	settings["deafened"] = call.Deafened
	return c.native.Start(settings, func(event map[string]any) {
		current := c.CallSnapshot()
		if current == nil || current.ID != call.ID {
			return
		}
		event["call"] = call.ID
		if event["event"] == "signal" {
			action, _ := event["action"].(string)
			sdp, _ := event["sdp"].(string)
			candidate, _ := event["candidate"].(string)
			mid, _ := event["sdpMid"].(string)
			var line *int
			if value, ok := event["sdpMLineIndex"].(float64); ok {
				v := int(value)
				line = &v
			}
			if err := c.SendWebRTC(action, sdp, candidate, mid, line); err != nil {
				c.emit("social_native_media", map[string]any{"event": "error", "error": err.Error(), "call": call.ID})
			}
		} else {
			c.emit("social_native_media", event)
		}
	})
}
func (c *Client) NativeCommand(command map[string]any) error { return c.native.Send(command) }
func (c *Client) NativeStop()                                { c.native.Stop() }
func NativeDevices() (any, error)                            { return nativertc.Devices() }
