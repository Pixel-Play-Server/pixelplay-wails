package discordrpc

import "testing"

func TestParseConnecting(t *testing.T) {
	host, ok := ParseConnecting("[22:18:06] [Render thread/INFO]: Connecting to mc.pixelplay.gg, 25565")
	if !ok || host != "mc.pixelplay.gg" {
		t.Fatalf("got %q ok=%v", host, ok)
	}
	host, ok = ParseConnecting("Connecting to 127.0.0.1, 25565")
	if !ok || host != "127.0.0.1" {
		t.Fatalf("localhost got %q ok=%v", host, ok)
	}
	if _, ok := ParseConnecting("Connecting to the nether"); ok {
		t.Fatal("false positive")
	}
}

func TestIsDisconnect(t *testing.T) {
	if !IsDisconnect("[WARN]: Client disconnected with reason: Proxy shutting down.") {
		t.Fatal("expected disconnect")
	}
	if IsDisconnect("Connecting to mc.pixelplay.gg, 25565") {
		t.Fatal("connecting is not disconnect")
	}
}
