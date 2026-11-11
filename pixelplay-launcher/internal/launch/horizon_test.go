package launch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/horizon"
	"pixelplay-launcher/internal/process"
)

func nativeFixture(t *testing.T) (profileJSON, horizon.Runtime) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PIXELPLAY_DATA_DIR", filepath.Join(home, "data"))
	p := profileJSON{ID: "native-test", Name: "Horizon", Path: "Horizon-Test"}
	root := filepath.Join(home, "data/profiles", p.Path, ".horizon-native/"+horizon.Version)
	r := horizon.Runtime{Root: root, Binary: filepath.Join(root, "horizon-client"), Resources: filepath.Join(root, ".local/horizon-client/26.3")}
	if err := os.MkdirAll(r.Resources, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Resources, "horizon-resources.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]any{"active": horizon.Version, "checked_at": time.Now().UTC(), "version": horizon.Version})
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "update.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	return p, r
}
func TestHorizonGuestAndMicrosoft(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux fixture")
	}
	p, r := nativeFixture(t)
	t.Setenv("HORIZON_HOME", "/wrong/profile")
	t.Setenv("HORIZON_SESSION_FILE", "/wrong/session")
	guest := &accounts.Credentials{Username: "PixelGuest"}
	cmd, err := horizonCommand(context.Background(), p, Options{QuickPlayMultiplayer: "localhost:25565"}, r, guest)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != r.Binary || strings.Contains(strings.Join(cmd.Args, " "), "launcher") {
		t.Fatal("must launch engine directly")
	}
	for _, entry := range cmd.Env {
		if strings.Contains(entry, "/wrong/") {
			t.Fatal("inherited session")
		}
	}
	cleanup, err := horizonSession(cmd, guest)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if strings.Contains(strings.Join(cmd.Args, " "), "session-file") {
		t.Fatal("guest has session")
	}
	premium := &accounts.Credentials{Username: "TestOwner", ID: "12345678-1234-1234-1234-123456789abc", AccessToken: "test-token-not-real"}
	cleanup, err = horizonSession(cmd, premium)
	if err != nil {
		t.Fatal(err)
	}
	session := cmd.Args[len(cmd.Args)-1]
	if strings.Contains(strings.Join(cmd.Args, " "), premium.AccessToken) {
		t.Fatal("token in arguments")
	}
	st, err := os.Stat(session)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("session not private", err)
	}
	cleanup()
	if _, err = os.Stat(session); !os.IsNotExist(err) {
		t.Fatal("session not removed")
	}
	for _, path := range []string{"", "../outside", "/absolute"} {
		bad := p
		bad.Path = path
		if _, err = horizonCommand(context.Background(), bad, Options{}, r, guest); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}
func TestHorizonHeadlessLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux fixture")
	}
	p, r := nativeFixture(t)
	guest := &accounts.Credentials{Username: "PixelGuest"}
	exited := false
	emit := func(event string, payload any) {
		if event == "minecraft_process_exited" {
			exited = payload.(map[string]any)["success"] == true
		}
	}
	if err := runHorizon(context.Background(), p.ID, p, guest, emit, Options{QuickPlayMultiplayer: "localhost:25565"}); err != nil {
		t.Fatal(err)
	}
	if !exited || process.Any() {
		t.Fatal("native lifecycle incomplete")
	}
	if err := os.WriteFile(r.Binary, []byte("#!/bin/sh\nsleep 30 &\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := runHorizon(ctx, p.ID, p, guest, nil, Options{}); err == nil {
		t.Fatal("expected cancellation")
	}
	if process.Any() {
		t.Fatal("cancel left tracked process")
	}
}
