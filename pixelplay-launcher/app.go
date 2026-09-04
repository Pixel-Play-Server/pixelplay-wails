package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"pixelplay-launcher/internal/config"
	"pixelplay-launcher/internal/discordrpc"
	"pixelplay-launcher/internal/ipc"
	"pixelplay-launcher/internal/modpackupdate"
	"pixelplay-launcher/internal/notify"
	"pixelplay-launcher/internal/osopen"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
	"pixelplay-launcher/internal/pushclient"
	"pixelplay-launcher/internal/updater"
	"pixelplay-launcher/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails backend (thin IPC over Go packages).
type App struct {
	ctx     context.Context
	discord *discordrpc.Manager
	rt      *ipc.Runtime

	notifyMu       sync.Mutex
	lastNotifiedTo string
}

func NewApp() *App {
	enabled := true
	if raw, err := config.Raw(); err == nil {
		var cfg struct {
			EnableDiscordPresence *bool  `json:"enable_discord_presence"`
			DiscordAppID          string `json:"discord_app_id"`
			DiscordLargeImage     string `json:"discord_large_image"`
			DiscordLargeText      string `json:"discord_large_text"`
		}
		if json.Unmarshal(raw, &cfg) == nil {
			if cfg.EnableDiscordPresence != nil {
				enabled = *cfg.EnableDiscordPresence
			}
			app := &App{discord: discordrpc.New(enabled)}
			app.discord.Configure(cfg.DiscordAppID, cfg.DiscordLargeImage, cfg.DiscordLargeText)
			return app
		}
	}
	return &App{discord: discordrpc.New(enabled)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.rt = &ipc.Runtime{
		Discord: a.discord,
		Emit: func(event string, payload any) {
			runtime.EventsEmit(ctx, event, payload)
		},
		OpenURL: func(u string) {
			runtime.BrowserOpenURL(ctx, u)
		},
		PickFile: func(title, filter string) (string, error) {
			return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
				Title: title,
				Filters: []runtime.FileFilter{
					{DisplayName: title, Pattern: filter},
				},
			})
		},
		PickFiles: func(title, filter string) ([]string, error) {
			return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{
				Title: title,
				Filters: []runtime.FileFilter{
					{DisplayName: title, Pattern: filter},
				},
			})
		},
		OnAuthorBlocked: func(action string) {
			notify.AuthorBlocked(action)
			runtime.EventsEmit(ctx, "author_protection", map[string]string{
				"action":  action,
				"message": "Protección de autor: " + action,
			})
		},
	}

	// Immediate check + periodic poll (fallback) + live push from Pixel hub.
	go a.updateLoop(ctx)
	go pushclient.Run(ctx, func(p pushclient.Packet) {
		if p.Type != "update_available" && p.Type != "download_published" {
			return
		}
		to := p.To
		if to == "" {
			to = "nueva"
		}
		a.notifyMu.Lock()
		already := a.lastNotifiedTo == to
		if !already {
			a.lastNotifiedTo = to
		}
		a.notifyMu.Unlock()
		if already {
			return
		}
		msg := p.Message
		if msg == "" {
			msg = "Nueva descarga publicada: " + to
		}
		notify.UpdateAvailable(version.App, to)
		runtime.EventsEmit(ctx, "updater_available", map[string]any{
			"from":    p.From,
			"to":      to,
			"url":     p.URL,
			"notes":   p.Notes,
			"message": msg,
			"source":  "push",
		})
		runtime.EventsEmit(ctx, "updater_status", updater.StatusEvent{
			Status:      "available",
			Message:     msg,
			FromVersion: p.From,
			ToVersion:   to,
			URL:         p.URL,
		})
	})
}

func (a *App) updateLoop(ctx context.Context) {
	a.runUpdateCheck(ctx)
	a.runPackUpdateCheck(ctx)
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runUpdateCheck(ctx)
			a.runPackUpdateCheck(ctx)
		}
	}
}

func (a *App) runUpdateCheck(ctx context.Context) {
	man, newer, err := updater.Check(version.App, func(ev updater.StatusEvent) {
		runtime.EventsEmit(ctx, "updater_status", ev)
	})
	if err != nil || !newer || man == nil {
		return
	}
	a.notifyMu.Lock()
	already := a.lastNotifiedTo == man.Version
	if !already {
		a.lastNotifiedTo = man.Version
	}
	a.notifyMu.Unlock()
	if already {
		return
	}
	notify.UpdateAvailable(version.App, man.Version)
	runtime.EventsEmit(ctx, "updater_available", map[string]any{
		"from":    version.App,
		"to":      man.Version,
		"url":     man.URL,
		"notes":   man.Notes,
		"message": "Nueva versión " + man.Version + " disponible",
		"source":  "poll",
	})
}

func (a *App) runPackUpdateCheck(ctx context.Context) {
	list, err := packs.List()
	if err != nil {
		return
	}
	for _, p := range list {
		up, cur, err := packs.UpdaterFor(p.ID)
		if err != nil || up == nil || strings.TrimSpace(up.URL) == "" {
			continue
		}
		runtime.EventsEmit(ctx, "modpack_update_status", map[string]any{
			"pack_id": p.ID,
			"status":  "checking",
			"current": cur,
		})
		man, newer, err := modpackupdate.Check(cur, up.URL)
		if err != nil || !newer || man == nil {
			continue
		}
		runtime.EventsEmit(ctx, "modpack_update_available", map[string]any{
			"pack_id": p.ID,
			"from":    cur,
			"to":      man.Version,
			"url":     man.URL,
			"notes":   man.Notes,
		})
		path, err := modpackupdate.Download(p.ID, man)
		if err != nil {
			runtime.EventsEmit(ctx, "modpack_update_status", map[string]any{
				"pack_id": p.ID,
				"status":  "error",
				"message": err.Error(),
			})
			continue
		}
		if _, err := packs.ImportPPackFile(path); err != nil {
			runtime.EventsEmit(ctx, "modpack_update_status", map[string]any{
				"pack_id": p.ID,
				"status":  "error",
				"message": err.Error(),
			})
			continue
		}
		_ = packs.ApplyOverridesToPackProfiles(p.ID)
		runtime.EventsEmit(ctx, "modpack_updated", map[string]any{
			"pack_id":     p.ID,
			"displayName": p.DisplayName,
			"from":        cur,
			"to":          man.Version,
			"path":        path,
		})
	}
}

// Invoke is the Tauri-compatible command bridge used by the React shim.
func (a *App) Invoke(cmd string, argsJSON string) (any, error) {
	return ipc.Dispatch(a.rt, cmd, argsJSON)
}

// GetDataDir returns the shared launcher data directory.
func (a *App) GetDataDir() string { return paths.DataDir() }

// ListProfiles returns all profiles from profiles.json.
func (a *App) ListProfiles() ([]profiles.Summary, error) {
	return profiles.List()
}

// GetProfile returns one profile by id.
func (a *App) GetProfile(id string) (*profiles.Summary, error) {
	return profiles.Get(id)
}

// ListPacks returns local Pixel Play packs.
func (a *App) ListPacks() ([]packs.PackSummary, error) {
	return packs.List()
}

// OpenProfileFolder reveals the profile directory in the OS file manager.
func (a *App) OpenProfileFolder(id string) (string, error) {
	if err := protect.AssertProfileFolderOpenable(id); err != nil {
		notify.AuthorBlocked("abrir carpeta del perfil")
		return "", err
	}
	dir, err := profiles.OpenFolder(id)
	if err != nil {
		return "", err
	}
	return dir, osopen.Path(dir)
}

// CheckForUpdates exposes updater check to Wails bindings (and UI).
func (a *App) CheckForUpdates() (map[string]any, error) {
	v, err := a.Invoke("check_for_updates", "{}")
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(v)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	if avail, _ := out["available"].(bool); avail {
		to, _ := out["manifest"].(map[string]any)
		ver, _ := to["version"].(string)
		a.notifyMu.Lock()
		already := ver != "" && a.lastNotifiedTo == ver
		if ver != "" && !already {
			a.lastNotifiedTo = ver
		}
		a.notifyMu.Unlock()
		if !already && ver != "" {
			notify.UpdateAvailable(version.App, ver)
		}
	}
	return out, nil
}

// DemoAuthorProtect runs a quick protect check (for demos / debug).
func (a *App) DemoAuthorProtect() map[string]any {
	all, err := profiles.List()
	out := map[string]any{"ok": false, "blocked": []any{}, "allowed": []any{}}
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	blocked := []map[string]string{}
	allowed := []map[string]string{}
	for _, p := range all {
		err := protect.AssertProfileFolderOpenable(p.ID)
		row := map[string]string{"id": p.ID, "name": p.Name}
		if p.SelectedPixelplayPackID != nil {
			row["pack"] = *p.SelectedPixelplayPackID
		}
		if err != nil {
			row["reason"] = err.Error()
			blocked = append(blocked, row)
			notify.AuthorBlocked(p.Name)
		} else {
			allowed = append(allowed, row)
		}
	}
	out["ok"] = true
	out["blocked"] = blocked
	out["allowed"] = allowed
	runtime.EventsEmit(a.ctx, "author_protection_demo", out)
	return out
}

// BackendInfo is a tiny health/debug endpoint.
func (a *App) BackendInfo() map[string]string {
	return map[string]string{
		"engine":        "wails+go",
		"data_dir":      paths.DataDir(),
		"version":       version.App,
		"updater_base":  updater.DefaultBaseURL,
		"updater_check": updater.ManifestURL(version.App),
	}
}
