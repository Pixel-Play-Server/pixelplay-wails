package ipc

import (
	"encoding/json"
	"fmt"

	"pixelplay-launcher/internal/social"
)

func socialCommand(cmd string, args map[string]json.RawMessage) (any, error) {
	c := social.Default()
	if c == nil {
		return nil, fmt.Errorf("servicio social no iniciado")
	}
	ok := map[string]any{"ok": true}
	switch cmd {
	case "social_call", "social_accept", "social_hangup", "social_mute", "social_deafen",
		"social_webrtc", "social_pcm_fallback", "social_mic_level",
		"social_native_start", "social_native_command", "social_native_stop", "social_native_devices":
		return nil, fmt.Errorf("las llamadas están deshabilitadas en este launcher")
	case "social_status":
		return c.Status(), nil
	case "social_send":
		to, err := argString(args, "to", "username")
		if err != nil {
			return nil, err
		}
		text, err := argString(args, "text", "message")
		if err != nil {
			return nil, err
		}
		cid, _ := argString(args, "cid")
		return ok, c.SendMessage(to, text, cid)
	case "social_history":
		with, err := argString(args, "with", "username")
		if err != nil {
			return nil, err
		}
		before, _ := argInt64(args, "before")
		return c.History(with, before)
	case "social_unread":
		return c.Unread()
	case "social_read":
		with, err := argString(args, "with", "username")
		if err != nil {
			return nil, err
		}
		return ok, c.MarkRead(with)
	case "social_typing":
		to, err := argString(args, "to", "username")
		if err != nil {
			return nil, err
		}
		return ok, c.Typing(to)
	default:
		return nil, fmt.Errorf("comando social desconocido: %s", cmd)
	}
}

func reconnectSocial() {
	if c := social.Default(); c != nil {
		c.Reconnect()
	}
}
