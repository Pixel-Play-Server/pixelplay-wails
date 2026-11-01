package process

import "testing"

func TestTakeStoppedAfterKillWithoutProcess(t *testing.T) {
	if TakeStopped("missing-profile") {
		t.Fatal("no process should not be marked stopped")
	}
}
