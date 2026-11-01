//go:build cgo

package voice

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"
)

type malgoEngine struct {
	mu       sync.Mutex
	ctx      *malgo.AllocatedContext
	dev      *malgo.Device
	jit      *jitter
	pending  []byte
	onFrame  func([]byte)
	muted    atomic.Bool
	deafened atomic.Bool
	level    atomic.Uint32
}

func New() Engine { return &malgoEngine{jit: newJitter()} }

func (e *malgoEngine) Start(onFrame func([]byte)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dev != nil {
		return nil
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Duplex)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = 1
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInMilliseconds = 20
	cfg.Alsa.NoMMap = 1

	e.onFrame = onFrame
	e.jit.reset()
	e.pending = e.pending[:0]

	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.data})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("micrófono/parlantes: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("audio start: %w", err)
	}
	e.ctx, e.dev = ctx, dev
	return nil
}

func (e *malgoEngine) data(out, in []byte, _ uint32) {
	if len(in) > 0 {
		e.level.Store(math.Float32bits(rms(in)))
		if !e.muted.Load() && e.onFrame != nil {
			e.pending = append(e.pending, in...)
			for len(e.pending) >= FrameBytes {
				frame := make([]byte, 1+FrameBytes)
				frame[0] = CodecPCM16
				copy(frame[1:], e.pending[:FrameBytes])
				e.pending = e.pending[FrameBytes:]
				e.onFrame(frame)
			}
		} else {
			e.pending = e.pending[:0]
		}
	}
	if len(out) > 0 {
		e.jit.pull(out)
		if e.deafened.Load() {
			clear(out)
		}
	}
}

func rms(pcm []byte) float32 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
		sum += s * s
	}
	return float32(math.Sqrt(sum / float64(n)))
}

func (e *malgoEngine) Play(frame []byte) {
	if len(frame) < 2 || frame[0] != CodecPCM16 {
		return
	}
	e.jit.push(frame[1:])
}

func (e *malgoEngine) SetMuted(v bool)    { e.muted.Store(v) }
func (e *malgoEngine) SetDeafened(v bool) { e.deafened.Store(v) }
func (e *malgoEngine) Level() float32     { return math.Float32frombits(e.level.Load()) }

func (e *malgoEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dev != nil {
		_ = e.dev.Stop()
		e.dev.Uninit()
		e.dev = nil
	}
	if e.ctx != nil {
		_ = e.ctx.Uninit()
		e.ctx.Free()
		e.ctx = nil
	}
	e.onFrame = nil
	e.jit.reset()
	e.muted.Store(false)
	e.deafened.Store(false)
	e.level.Store(0)
}
