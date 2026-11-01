package redact

import (
	"strings"
	"testing"
)

func TestLineRedactsHomeAndMods(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home")
	}
	game := home + "/.local/share/pixelplayclientv3/profiles/demo"
	line := "Loading file " + game + "/mods/Mekanism-1.21.1.jar from " + home
	got := Line(line, game)
	if strings.Contains(got, home) {
		t.Fatalf("home leaked: %s", got)
	}
	if strings.Contains(got, "Mekanism") {
		t.Fatalf("mod filename leaked: %s", got)
	}
	if !strings.Contains(got, "[path]") && !strings.Contains(got, "/mods/[mod]") {
		t.Fatalf("expected redaction tokens, got %s", got)
	}
}

func TestMaskSameLength(t *testing.T) {
	p := "/home/tester/.local/share/pixelplayclientv3"
	in := "x=" + p + "/mods/foo.jar"
	out := MaskSameLength(in, p)
	if len(out) != len(in) {
		t.Fatalf("length changed %d -> %d", len(in), len(out))
	}
	if strings.Contains(out, "pixelplayclientv3") {
		t.Fatalf("path remained: %s", out)
	}
	jar := "/mods/Mekanism-1.21.1.jar"
	in2 := "load " + jar
	out2 := MaskSameLength(in2)
	if len(out2) != len(in2) {
		t.Fatalf("mod mask length %d -> %d", len(in2), len(out2))
	}
	if strings.Contains(out2, "Mekanism") {
		t.Fatalf("mod name remained: %s", out2)
	}
}
