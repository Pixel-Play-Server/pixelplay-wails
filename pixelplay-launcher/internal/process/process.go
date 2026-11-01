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
	mu      sync.Mutex
	procs   = map[string]*entry{}
	stopped = map[string]bool{}
)

type entry struct {
	meta Metadata
	proc *os.Process
	stop func() error
}

func Add(p *os.Process, meta Metadata) {
	AddWithStop(p, meta, nil)
}

// AddWithStop lets native launchers terminate their owned child processes too.
func AddWithStop(p *os.Process, meta Metadata, stop func() error) {
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
	procs[meta.ProfileID] = &entry{meta: meta, proc: p, stop: stop}
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
	if ok && e.proc != nil {
		stopped[profileID] = true
	}
	mu.Unlock()
	if !ok || e.proc == nil {
		return nil
	}
	if e.stop != nil {
		return e.stop()
	}
	return e.proc.Kill()
}

// TakeStopped reports whether the launcher asked to close this instance.
func TakeStopped(profileID string) bool {
	mu.Lock()
	defer mu.Unlock()
	ok := stopped[profileID]
	delete(stopped, profileID)
	return ok
}
