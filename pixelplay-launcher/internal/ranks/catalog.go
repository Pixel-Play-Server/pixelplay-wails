package ranks

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const catalogURL = "https://api.pixelplay.gg/api/v1/ranks"

type Def struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Color     string   `json:"color"`
	Gem       string   `json:"gem,omitempty"`
	Exclusive bool     `json:"exclusive,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Perks     []string `json:"perks"`
}

type Catalog struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Default   string `json:"default"`
	Ranks     []Def  `json:"ranks"`
}

var (
	live    atomic.Pointer[Catalog]
	lastAt  atomic.Int64
	fetchMu sync.Mutex
)

func init() {
	live.Store(&Catalog{
		Default: "usuario",
		Ranks: []Def{
			{Key: "usuario", Label: "Usuario", Color: "#059669"},
			{Key: "mini_pixel", Label: "Mini Pixel", Color: "#d97706", Aliases: []string{"mini"}},
			{Key: "pixel", Label: "Pixel", Color: "#34d399"},
			{Key: "pixel_plus", Label: "Pixel+", Color: "#c084fc", Aliases: []string{"pixel+"}},
			{Key: "fundador", Label: "Fundador", Color: "#ef4444", Aliases: []string{"founder"}, Exclusive: true},
		},
	})
}

// Refresh fetches the remote catalog at most once per minute unless force.
func Refresh(force bool) {
	now := time.Now().Unix()
	if !force && now-lastAt.Load() < 60 {
		return
	}
	fetchMu.Lock()
	defer fetchMu.Unlock()
	if !force && time.Now().Unix()-lastAt.Load() < 60 {
		return
	}
	client := &http.Client{Timeout: 4 * time.Second}
	res, err := client.Get(catalogURL)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return
	}
	var c Catalog
	if json.NewDecoder(res.Body).Decode(&c) != nil || len(c.Ranks) == 0 {
		return
	}
	if strings.TrimSpace(c.Default) == "" {
		c.Default = "usuario"
	}
	live.Store(&c)
	lastAt.Store(time.Now().Unix())
}

func Meta(rank string) (label, color string) {
	Refresh(false)
	c := live.Load()
	key := strings.ToLower(strings.TrimSpace(rank))
	if key == "" {
		key = c.Default
	}
	for _, r := range c.Ranks {
		if r.Key == key {
			return r.Label, r.Color
		}
		for _, a := range r.Aliases {
			if strings.EqualFold(a, key) {
				return r.Label, r.Color
			}
		}
	}
	if key == "" || key == "usuario" || key == "user" {
		return "Usuario", "#059669"
	}
	// Unknown remote rank: still show something readable without forcing a launcher update.
	return strings.ToUpper(key[:1]) + key[1:], "#94a3b8"
}
