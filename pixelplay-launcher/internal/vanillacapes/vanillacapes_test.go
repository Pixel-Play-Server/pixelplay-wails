package vanillacapes

import (
	"io"
	"net/http"
	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOwnedUsesMinecraftProfileAndActiveState(t *testing.T) {
	previous := httpx.Client
	t.Cleanup(func() { httpx.Client = previous })
	httpx.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.minecraftservices.com" || r.URL.Path != "/minecraft/profile" {
			t.Fatalf("wrong endpoint: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Fatal("missing authorization")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"capes":[{"id":"a","alias":"Migrator","state":"ACTIVE","url":"https://textures.minecraft.net/texture/a"},{"id":"b","alias":"Vanilla","state":"INACTIVE","url":"https://textures.minecraft.net/texture/b"}]}`))}, nil
	})}
	capes, err := ownedWith("fixture")
	if err != nil || len(capes) != 2 || !capes[0].Equipped || capes[1].Equipped || capes[0].Category != "vanilla" {
		t.Fatalf("unexpected capes: %+v %v", capes, err)
	}
}
func TestLocalAccountCannotFetchOfficialCapes(t *testing.T) {
	t.Setenv("PIXELPLAY_DATA_DIR", t.TempDir())
	if _, err := accounts.CreateOffline("Fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := activeToken(); err == nil {
		t.Fatal("local account accepted as a Minecraft session")
	}
}
