//go:build !windows

package protect

import "context"

func excludeFromCapture(ctx context.Context, pid int) {
	// Linux/macOS have no SetWindowDisplayAffinity. The Java agent is a no-op
	// here; screenshots are still wiped by GuardSession.
	<-ctx.Done()
}
