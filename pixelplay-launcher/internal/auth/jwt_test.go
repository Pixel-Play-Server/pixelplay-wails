package auth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"pixelplay-launcher/internal/accounts"
)

func jwtToken(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	return "hdr." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func TestJWTUnixExpAndUsername(t *testing.T) {
	token := jwtToken(map[string]any{
		"exp":      time.Now().Add(time.Hour).Unix(),
		"username": "Joacoorandom",
	})
	if !jwtFresh(token, 2*time.Minute) {
		t.Fatal("expected fresh token")
	}
	name, ok := jwtClaimString(token, "username")
	if !ok || name != "Joacoorandom" {
		t.Fatalf("username=%q ok=%v", name, ok)
	}
}

func TestJWTExpired(t *testing.T) {
	token := jwtToken(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})
	if jwtFresh(token, 0) {
		t.Fatal("expected expired token")
	}
}

func TestHMACAndDeviceID(t *testing.T) {
	got := hmacSHA256Hex("NRC", "abc\n")
	if len(got) != 64 {
		t.Fatalf("hmac hex len=%d", len(got))
	}
	t.Setenv("PIXELPLAY_MACHINE_ID", "abc\n")
	id := DeviceSaltedID()
	if len(id) != 64 {
		t.Fatalf("device id len=%d", len(id))
	}
}

func TestLiveEnsurePixelplay(t *testing.T) {
	if os.Getenv("PIXELPLAY_LIVE_AUTH") == "" {
		t.Skip("set PIXELPLAY_LIVE_AUTH=1 to hit api.pixelplay.gg")
	}
	acc, err := accounts.Active()
	if err != nil || acc == nil {
		t.Fatalf("active account: %v %v", acc, err)
	}
	got, err := EnsurePixelplay(*acc)
	if err != nil {
		t.Fatal(err)
	}
	if got.PixelplayCredentials.Production == nil || got.PixelplayCredentials.Production.Value == "" {
		t.Fatal("expected production pixelplay token")
	}
}
func TestApplyPixelClaimsFromMicrosoft(t *testing.T) {
	token := jwtToken(map[string]any{
		"exp":      time.Now().Add(time.Hour).Unix(),
		"username": "Joacoorandom",
		"rank":     "pixel_plus",
		"kind":     "premium",
	})
	c := accounts.Credentials{
		Username:    "Joacoorandom",
		AccessToken: "ms-token",
		PixelplayCredentials: accounts.PixelplayCredentials{
			Production: &accounts.Token{Value: token},
		},
	}
	got := applyPixelClaims(c)
	if got.Rank != "pixel_plus" || got.Kind != "premium" {
		t.Fatalf("claims %+v", got)
	}
	if !got.PixelAccount() || got.PixelPasswordAccount() {
		t.Fatal("microsoft + jwt should be a Pixel account")
	}
}

func TestPixelplayTokenOK(t *testing.T) {
	token := jwtToken(map[string]any{
		"exp":      time.Now().Add(time.Hour).Unix(),
		"username": "Joacoorandom",
		"rank":     "pixel_plus",
	})
	c := accounts.Credentials{
		Username: "Joacoorandom",
		PixelplayCredentials: accounts.PixelplayCredentials{
			Production: &accounts.Token{Value: token},
		},
	}
	if !pixelplayTokenOK(c, false) {
		t.Fatal("expected valid production token")
	}
	c.Username = "Other"
	if pixelplayTokenOK(c, false) {
		t.Fatal("username mismatch should refresh")
	}
}
