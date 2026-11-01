package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Desktop shows a native OS notification when possible.
func Desktop(title, body string) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			fmt.Fprintf(os.Stderr, "[notify] %s — %s\n", title, body)
			return
		}
		cmd := exec.Command(
			"notify-send",
			"--app-name=Pixel Play",
			"--urgency=normal",
			"--expire-time=12000",
			"--icon=software-update-available",
			title,
			body,
		)
		_ = cmd.Start()
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		_ = exec.Command("osascript", "-e", script).Start()
	case "windows":
		desktopWindows(title, body)
	default:
		fmt.Fprintf(os.Stderr, "[notify] %s — %s\n", title, body)
	}
}

// UpdateAvailable is the branded updater toast for the OS.
func UpdateAvailable(from, to string) {
	Desktop(
		"Pixel Play — actualización",
		fmt.Sprintf("Hay una nueva versión: %s → %s. Abrí el launcher para descargarla.", from, to),
	)
}

// AuthorBlocked notifies when author protection blocks an action.
func AuthorBlocked(action string) {
	Desktop(
		"Pixel Play — protección de autor",
		fmt.Sprintf("Acción bloqueada: %s", action),
	)
}
