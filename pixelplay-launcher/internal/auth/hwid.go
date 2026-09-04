package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// DeviceSaltedID matches the Rust launcher HWID used by Pixel Play auth:
// HMAC-SHA256("NRC", /etc/machine-id) then SHA256("pixelplay-device-salt" + hex).
func DeviceSaltedID() string {
	hwid := hmacSHA256Hex("NRC", systemIDRaw())
	sum := sha256.Sum256([]byte("pixelplay-device-salt" + hwid))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Hex(key, msg string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func systemIDRaw() string {
	if v := os.Getenv("PIXELPLAY_MACHINE_ID"); v != "" {
		return v
	}
	for _, p := range []string{"/var/lib/dbus/machine-id", "/etc/machine-id"} {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b)
		}
	}
	host, _ := os.Hostname()
	return strings.TrimSpace(host)
}
