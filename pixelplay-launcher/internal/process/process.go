package process

import (
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Metadata struct {
	ID               string  `json:"id"`
	ProfileID        string  `json:"profile_id"`
	StartTime        string  `json:"start_time"`
	State            string  `json:"state"`
	PID              int     `json:"pid"`
	AccountUUID      *string `json:"account_uuid"`
	AccountName      *string `json:"account_name"`
	MinecraftVersion *string `json:"minecraft_version"`
	Modloader        *string `json:"modloader"`
	ModloaderVersion *string `json:"modloader_version"`
	PixelplayPack    *string `json:"pixelplay_pack"`
	ProfileName      *string `json:"profile_name"`
	ProfileImageURL  *string `json:"profile_image_url"`
}

var (
	mu    sync.Mutex
	procs = map[string]*entry{}
)

type entry struct {
	meta Metadata
	proc *os.Process
}

func Add(p *os.Process, meta Metadata) {
	if meta.ID == "" {
		meta.ID = uuid.NewString()
	}
	if meta.StartTime == "" {
		meta.StartTime = time.Now().UTC().Format(time.RFC3339Nano)
	}
	meta.State = "Running"
	if p != nil {
		meta.PID = p.Pid
	}
	mu.Lock()
	defer mu.Unlock()
	procs[meta.ProfileID] = &entry{meta: meta, proc: p}
}

func Any() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(procs) > 0
}

func List() []Metadata {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Metadata, 0, len(procs))
	for _, e := range procs {
		out = append(out, e.meta)
	}
	return out
}

func Remove(profileID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(procs, profileID)
}

func Kill(profileID string) error {
	mu.Lock()
	e, ok := procs[profileID]
	mu.Unlock()
	if !ok || e.proc == nil {
		return nil
	}
	return e.proc.Kill()
}
