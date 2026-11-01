package discordrpc

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hugolgst/rich-go/ipc"
)

// Discord activity type 0 = Playing.
const activityPlaying = 0

type activityButton struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type activityAssets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
}

type activityTimestamps struct {
	Start int64 `json:"start"`
}

type activityBody struct {
	Type       int                 `json:"type"`
	Details    string              `json:"details,omitempty"`
	State      string              `json:"state,omitempty"`
	Assets     *activityAssets     `json:"assets,omitempty"`
	Timestamps *activityTimestamps `json:"timestamps,omitempty"`
	Buttons    []activityButton    `json:"buttons,omitempty"`
}

type setActivityFrame struct {
	Cmd   string `json:"cmd"`
	Nonce string `json:"nonce"`
	Args  struct {
		PID      int           `json:"pid"`
		Activity activityBody  `json:"activity"`
	} `json:"args"`
}

func presenceButtons() []activityButton {
	return []activityButton{
		{Label: "Descargar", URL: "https://launcher.pixelplay.gg"},
		{Label: "Discord", URL: discordInviteURL},
	}
}

func setDiscordActivity(details, state, largeImage, largeText string, start time.Time) error {
	frame := setActivityFrame{
		Cmd:   "SET_ACTIVITY",
		Nonce: activityNonce(),
	}
	frame.Args.PID = os.Getpid()
	frame.Args.Activity = activityBody{
		Type:    activityPlaying,
		Details: details,
		State:   state,
		Assets: &activityAssets{
			LargeImage: largeImage,
			LargeText:  largeText,
		},
		Timestamps: &activityTimestamps{Start: start.UnixMilli()},
		Buttons:    presenceButtons(),
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	resp := ipc.Send(1, string(payload))
	if resp != "" {
		log.Printf("[discord] set_activity ack: %s", truncate(resp, 180))
	}
	return nil
}

func activityNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}
