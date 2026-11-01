package updater

import "testing"

func TestDropInURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{
			"https://panel.pixelplay.gg/dl/pixelplay/releases-v2/windows-x86_64/5.0.23/pixelplay-launcher.exe",
			"https://panel.pixelplay.gg/dl/pixelplay/releases-v2/windows-x86_64/5.0.23/pixelplay-launcher.exe",
		},
		{
			"https://panel.pixelplay.gg/dl/pixelplay/releases-v2/windows-x86_64/5.0.23/pixelplay-launcher-amd64-installer.exe",
			"https://panel.pixelplay.gg/dl/pixelplay/releases-v2/windows-x86_64/5.0.23/pixelplay-launcher.exe",
		},
		{
			"https://launcher.pixelplay.gg/dl/PixelPlaySetup.exe",
			"https://launcher.pixelplay.gg/dl/pixelplay-launcher.exe",
		},
	}
	for _, c := range cases {
		got := DropInURL(c.in)
		if got != c.want {
			t.Fatalf("DropInURL(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
