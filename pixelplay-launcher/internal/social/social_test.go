package social

import (
	"testing"
)

func TestStatusClearsPreviousUser(t *testing.T) {
	events := []map[string]any{}
	c := &Client{user: "old", connected: true, emit: func(_ string, v any) { events = append(events, v.(map[string]any)) }}
	c.setConnected(false, "")
	if c.Status()["user"] != "" || c.Status()["connected"] != false {
		t.Fatal("stale identity after disconnect")
	}
	c.setConnected(true, "new")
	if c.Status()["user"] != "new" || len(events) != 2 {
		t.Fatal("new identity was not emitted")
	}
}

func TestReconnectInvalidatesSessionImmediately(t *testing.T) {
	c := &Client{user: "old", connected: true, wake: make(chan struct{}, 1), emit: func(string, any) {}}
	c.Reconnect()
	if c.Status()["connected"] != false || c.Status()["user"] != "" {
		t.Fatal("old session remained usable")
	}
	select {
	case <-c.wake:
	default:
		t.Fatal("reconnect did not wake loop")
	}
}

func TestInvalidMessagesAreRejected(t *testing.T) {
	c := &Client{}
	for _, args := range [][3]string{{"", "hello", "a"}, {"friend", " ", "b"}, {"friend", "hi", ""}} {
		if c.SendMessage(args[0], args[1], args[2]) == nil {
			t.Fatal("invalid message accepted")
		}
	}
}
