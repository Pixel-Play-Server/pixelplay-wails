// Package host is the shared Wails backend bound by Linux and Windows shells.
// Lives outside internal/ so pixelplay-launcher-windows can import it via replace.
package host

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/config"
	"pixelplay-launcher/internal/discordrpc"
	"pixelplay-launcher/internal/filepick"
	"pixelplay-launcher/internal/ipc"
	"pixelplay-launcher/internal/notify"
	"pixelplay-launcher/internal/osopen"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/presence"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
	"pixelplay-launcher/internal/pushclient"
	"pixelplay-launcher/internal/social"
	"pixelplay-launcher/internal/targeted"
	"pixelplay-launcher/internal/updater"
	"pixelplay-launcher/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails backend (thin IPC over Go packages).
type App struct {
	ctx     context.Context
	discord *discordrpc.Manager
	rt      *ipc.Runtime

	notifyMu         sync.Mutex
	lastNotifiedTo   string
	lastPackNotified map[string]string
}

// New constructs the shared launcher host.
func New() *App {
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

// Startup wires runtime callbacks, background loops, and the IPC bridge.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	_ = paths.EnsureLayout()
	_ = profiles.EnsureStore()
	// Auto-renew tokens for all accounts silently without asking anything
	go auth.AutoRefreshAllAccountTokens()
	// Per-machine maintenance actions (e.g. re-auth support key after an OS update).
	go targeted.Run(nil)
	a.rt = &ipc.Runtime{
		Discord: a.discord,
		Emit: func(event string, payload any) {
			runtime.EventsEmit(ctx, event, payload)
		},
		OpenURL: func(u string) {
			runtime.BrowserOpenURL(ctx, u)
		},
		PickFile: func(title, filter string) (string, error) {
			if p, err := filepick.Open(title, filter); err != filepick.ErrUnsupported {
				return p, err
			}
			return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
				Title: title,
				Filters: []runtime.FileFilter{
					{DisplayName: filepickLabel(title, filter), Pattern: filter},
				},
			})
		},
		PickFiles: func(title, filter string) ([]string, error) {
			if p, err := filepick.OpenMany(title, filter); err != filepick.ErrUnsupported {
				return p, err
			}
			return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{
				Title: title,
				Filters: []runtime.FileFilter{
					{DisplayName: filepickLabel(title, filter), Pattern: filter},
				},
			})
		},
		PickDir: func(title string) (string, error) {
			if p, err := filepick.OpenDir(title); err != filepick.ErrUnsupported {
				return p, err
			}
			return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: title})
		},
		SaveFile: func(title, suggested, filter string) (string, error) {
			if p, err := filepick.Save(title, suggested, filter); err != filepick.ErrUnsupported {
				return p, err
			}
			return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
				Title:           title,
				DefaultFilename: suggested,
				Filters: []runtime.FileFilter{
					{DisplayName: filepickLabel(title, filter), Pattern: filter},
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

	social.Start(ctx, func(event string, payload any) {
		runtime.EventsEmit(ctx, event, payload)
	})

	// Immediate check + periodic poll (fallback) + live push from Pixel hub.
	go presence.Run(ctx)
	go a.updateLoop(ctx)
	go pushclient.Run(ctx, func(p pushclient.Packet) {
		switch p.Type {
		case "rank_changed":
			runtime.EventsEmit(ctx, "rank_changed", map[string]any{
				"from":     p.From,
				"to":       p.To,
				"username": p.Message,
				"uuid":     p.Notes,
			})
		case "ranks_updated":
			runtime.EventsEmit(ctx, "ranks_updated", map[string]any{
				"version": p.To,
				"updated": p.Notes,
				"message": p.Message,
			})
		case "update_available", "download_published":
			// Re-check platform-correct manifest instead of trusting push URL
			// (publish scripts historically sent the Linux artifact URL).
			go a.runUpdateCheck(ctx)
		}
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
	emit := func(event string, payload any) {
		runtime.EventsEmit(ctx, event, payload)
	}
	for _, p := range list {
		if p.IsExperimental {
			continue
		}
		up, cur, err := packs.UpdaterFor(p.ID)
		if err != nil || up == nil || strings.TrimSpace(up.URL) == "" {
			continue
		}
		runtime.EventsEmit(ctx, "modpack_update_status", map[string]any{
			"pack_id": p.ID,
			"status":  "checking",
			"current": cur,
		})
		emitOnce := func(event string, payload any) {
			if event == "modpack_update_available" {
				to, _ := payload.(map[string]any)["to"].(string)
				a.notifyMu.Lock()
				if a.lastPackNotified == nil {
					a.lastPackNotified = map[string]string{}
				}
				if a.lastPackNotified[p.ID] == to {
					a.notifyMu.Unlock()
					return
				}
				a.lastPackNotified[p.ID] = to
				a.notifyMu.Unlock()
			}
			emit(event, payload)
		}
		_, _, _, err = packs.ApplyRemoteUpdate(p.ID, emitOnce)
		if err != nil {
			runtime.EventsEmit(ctx, "modpack_update_status", map[string]any{
				"pack_id": p.ID,
				"status":  "error",
				"message": err.Error(),
			})
		}
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

func filepickLabel(title, filter string) string {
	blob := strings.ToLower(title + " " + filter)
	switch {
	case strings.Contains(blob, "ppack") || strings.Contains(blob, "perfil") || strings.Contains(blob, "modpack"):
		return "Pixel Pack (.ppack)"
	case strings.Contains(blob, "png"):
		return "Imágenes PNG"
	case strings.Contains(blob, "jar"):
		return "Mods (.jar)"
	case title != "":
		return title
	default:
		return "Archivos"
	}
}
