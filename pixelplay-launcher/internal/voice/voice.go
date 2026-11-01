// Package voice captures and plays call audio as 16 kHz mono PCM16 in 20 ms frames,
// the format relayed by the pixel-social server.
package voice

import "sync"

const (
	SampleRate    = 16000
	FrameSamples  = SampleRate / 50
	FrameBytes    = FrameSamples * 2
	CodecPCM16    = 1
	maxBufferedMs = 300
	startBufferMs = 60
)

// Engine is implemented by the native (cgo) backend or a stub without cgo.
type Engine interface {
	Start(onFrame func([]byte)) error
	Play(frame []byte)
	SetMuted(bool)
	SetDeafened(bool)
	Level() float32
	Stop()
}

// jitter is a byte FIFO that smooths network delivery before playback.
type jitter struct {
	mu      sync.Mutex
	buf     []byte
	primed  bool
	maxSize int
	start   int
}

func newJitter() *jitter {
	return &jitter{
		maxSize: SampleRate * 2 * maxBufferedMs / 1000,
		start:   SampleRate * 2 * startBufferMs / 1000,
	}
}

func (j *jitter) push(pcm []byte) {
	j.mu.Lock()
	j.buf = append(j.buf, pcm...)
	if over := len(j.buf) - j.maxSize; over > 0 {
		j.buf = j.buf[over:]
	}
	j.mu.Unlock()
}

// pull fills out with buffered audio and silence when starved.
func (j *jitter) pull(out []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.primed {
		if len(j.buf) < j.start {
			clear(out)
			return
		}
		j.primed = true
	}
	n := copy(out, j.buf)
	j.buf = j.buf[n:]
	if n < len(out) {
		clear(out[n:])
		j.primed = false
	}
}

func (j *jitter) reset() {
	j.mu.Lock()
	j.buf = j.buf[:0]
	j.primed = false
	j.mu.Unlock()
}
