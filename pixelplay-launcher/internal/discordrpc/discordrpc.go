package discordrpc

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	client "github.com/hugolgst/rich-go/client"
)

const (
	defaultAppID = "1423767021371261038"
	// Asset key already uploaded on the Pixel Play Discord application (same PNG as Tauri).
	defaultLargeImage = "Square512x512Logo"
	downloadPageURL   = "https://launcher.pixelplay.gg/dl/pixelplay-launcher"
	discordInviteURL  = "https://discord.pixelplay.gg"
	// Public copy of the same logo — Discord accepts https URLs as large_image.
	publicLogoURL = "https://panel.pixelplay.gg/dl/pixelplay/branding/logo.png"
)

// State mirrors the launcher presence model.
type State struct {
	Kind    string // idle | ingame | modal | launching | installing
	Profile string
	Version string
	Pack    string
	Player  string
	Server  string
}

// Manager owns Discord IPC connection + current activity.
type Manager struct {
	mu        sync.Mutex
	enabled   bool
	appID     string
	largeImg  string
	largeTxt  string
	loggedIn  bool
	started   time.Time
	sessionAt *time.Time
	state     State
	sent      bool
}

func New(enabled bool) *Manager {
	appID := os.Getenv("PIXELPLAY_DISCORD_APP_ID")
	if appID == "" {
		appID = defaultAppID
	}
	largeImg := os.Getenv("PIXELPLAY_DISCORD_LARGE_IMAGE")
	if largeImg == "" {
		largeImg = defaultLargeImage
	}
	m := &Manager{
		enabled:  enabled,
		appID:    appID,
		largeImg: largeImg,
		largeTxt: "Pixel Play Launcher",
		started:  time.Now(),
		state:    State{Kind: "idle"},
	}
	if enabled {
		_ = m.connect()
		_ = m.SetState(State{Kind: "idle"})
	}
	return m
}

func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled == enabled {
		return
	}
	m.enabled = enabled
	if enabled {
		_ = m.connectLocked()
		m.sent = false
		_ = m.applyLocked(State{Kind: "idle"})
		return
	}
	m.logoutLocked()
}

func (m *Manager) Configure(appID, largeImage, largeText string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if appID != "" {
		m.appID = appID
	}
	if largeImage != "" {
		m.largeImg = largeImage
	}
	if largeText != "" {
		m.largeTxt = largeText
	}
}

func (m *Manager) SetState(s State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyLocked(s)
}

// SessionLocked is true while Minecraft is launching or running.
// The React UI must not overwrite this with tab browsing.
func (m *Manager) SessionLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return isSessionKind(m.state.Kind)
}

func (m *Manager) HandleGameLog(line string) {
	if host, ok := ParseConnecting(line); ok {
		m.UpdateServer(host, true)
		return
	}
	if IsDisconnect(line) {
		m.UpdateServer("", false)
	}
}

func (m *Manager) UpdateServer(host string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !isSessionKind(m.state.Kind) {
		return
	}
	next := m.state
	if connected {
		next.Server = host
	} else {
		next.Server = ""
	}
	_ = m.applyLocked(next)
}

func (m *Manager) connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connectLocked()
}

func (m *Manager) connectLocked() error {
	if !m.enabled || m.loggedIn {
		return nil
	}
	if err := client.Login(m.appID); err != nil {
		log.Printf("[discord] login failed: %v", err)
		return err
	}
	m.loggedIn = true
	return nil
}

func (m *Manager) logoutLocked() {
	if !m.loggedIn {
		return
	}
	func() {
		defer func() { _ = recover() }()
		client.Logout()
	}()
	m.loggedIn = false
	m.sent = false
}

func (m *Manager) applyLocked(s State) error {
	prev := m.state
	entering := isSessionKind(s.Kind)
	if entering && (m.sessionAt == nil || !isSessionKind(prev.Kind)) {
		t := time.Now()
		m.sessionAt = &t
	}
	if !entering {
		m.sessionAt = nil
	}

	if m.sent && sameState(prev, s) {
		m.state = s
		return nil
	}
	m.state = s
	if !m.enabled {
		return nil
	}
	if err := m.connectLocked(); err != nil {
		return nil
	}

	start := m.started
	if entering && m.sessionAt != nil {
		start = *m.sessionAt
	}

	large := m.largeImg
	if large == "" || large == defaultLargeImage {
		if u := os.Getenv("PIXELPLAY_DISCORD_LOGO_URL"); u != "" {
			large = u
		} else {
			// HTTPS URL so the logo shows even if the Discord asset catalog is stale.
			large = publicLogoURL
		}
	}

	act := client.Activity{
		LargeImage: large,
		LargeText:  nonempty(m.largeTxt, "Pixel Play Launcher"),
		Buttons: []*client.Button{
			{Label: "Descargar launcher", Url: downloadPageURL},
			{Label: "Discord", Url: discordInviteURL},
		},
		Timestamps: &client.Timestamps{Start: &start},
	}

	packOrProfile := nonempty(s.Pack, nonempty(s.Profile, "Minecraft"))
	player := strings.TrimSpace(s.Player)

	switch strings.ToLower(s.Kind) {
	case "idle", "inactivo":
		act.Details = "Pixel Play Launcher"
		act.State = idleLine(player, "En el menú")
	case "ingame", "in_game", "playing", "playing_game":
		act.Details = truncate(fmt.Sprintf("Jugando · %s", packOrProfile), 128)
		act.State = truncate(ingameLine(player, s.Server), 128)
	case "launching", "starting":
		act.Details = truncate(fmt.Sprintf("Iniciando · %s", packOrProfile), 128)
		act.State = truncate(launchingLine(player, s.Version), 128)
	case "installing", "downloading":
		act.Details = truncate(fmt.Sprintf("Instalando · %s", packOrProfile), 128)
		act.State = truncate(nonempty(s.Version, nonempty(player, "Sincronizando contenido…")), 128)
	case "modal", "launcher":
		act.Details = "Pixel Play Launcher"
		section := friendlySection(s.Profile)
		act.State = truncate(idleLine(player, section), 128)
	default:
		act.Details = "Pixel Play Launcher"
		act.State = truncate(nonempty(s.Kind, "En el menú"), 128)
	}

	if err := client.SetActivity(act); err != nil {
		log.Printf("[discord] set activity: %v", err)
		m.loggedIn = false
		m.sent = false
		return err
	}
	m.sent = true
	return nil
}

func isSessionKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "ingame", "in_game", "playing", "playing_game", "launching", "starting", "installing", "downloading":
		return true
	default:
		return false
	}
}

// IsBrowseKind is a UI tab/modal update that must not clobber an active session.
func IsBrowseKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "idle", "inactivo", "modal", "launcher":
		return true
	default:
		return false
	}
}

func sameState(a, b State) bool {
	return a.Kind == b.Kind && a.Profile == b.Profile && a.Version == b.Version &&
		a.Pack == b.Pack && a.Player == b.Player && a.Server == b.Server
}

func idleLine(player, section string) string {
	section = nonempty(section, "En el menú")
	if player == "" {
		return section
	}
	return player + " · " + section
}

func ingameLine(player, server string) string {
	switch {
	case player != "" && server != "":
		return player + " @ " + server
	case player != "" && server == "":
		return player + " · en Minecraft"
	case player == "" && server != "":
		return "Conectado a " + server
	default:
		return "En partida"
	}
}

func launchingLine(player, version string) string {
	ver := version
	if ver != "" {
		ver = "Minecraft " + ver
	} else {
		ver = "Arrancando…"
	}
	if player == "" {
		return ver
	}
	return player + " · " + ver
}

func nonempty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max < 1 {
		return ""
	}
	return string(runes[:max-1]) + "…"
}

func friendlySection(raw string) string {
	key := strings.ToLower(strings.TrimSpace(strings.Split(raw, "|")[0]))
	key = strings.TrimSpace(strings.Split(key, "·")[0])
	switch key {
	case "play", "jugar":
		return "Jugar"
	case "profiles", "profilesv2", "perfiles", "instancias":
		return "Perfiles"
	case "mods":
		return "Mods"
	case "skins":
		return "Skins"
	case "capes", "store":
		return "Capes"
	case "settings", "ajustes":
		return "Ajustes"
	case "logs":
		return "Logs"
	default:
		if strings.HasPrefix(key, "modal:") {
			return "Diálogo"
		}
		if strings.TrimSpace(raw) == "" {
			return "En el menú"
		}
		return strings.TrimSpace(raw)
	}
}

// ParsePipe splits "Profile|1.21.1" style payloads from the React UI.
func ParsePipe(raw string) (profile, extra string) {
	parts := strings.SplitN(raw, "|", 2)
	profile = parts[0]
	if len(parts) > 1 {
		extra = parts[1]
	}
	return profile, extra
}