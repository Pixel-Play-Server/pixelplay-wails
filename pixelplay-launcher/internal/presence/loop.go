package presence

import (
	"context"
	"time"

	"pixelplay-launcher/internal/auth"
	"pixelplay-launcher/internal/process"
)

func Run(ctx context.Context) {
	report := func(status string) {
		_, _ = auth.SetActivePresence(status)
	}
	current := func() string {
		if process.Any() {
			return "ingame"
		}
		return "online"
	}
	report(current())
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			report("offline")
			return
		case <-tick.C:
			report(current())
		}
	}
}
