//go:build !cgo

package voice

import "errors"

type stubEngine struct{}

func New() Engine { return stubEngine{} }

func (stubEngine) Start(func([]byte)) error {
	return errors.New("esta build no incluye audio de llamadas")
}
func (stubEngine) Play([]byte)      {}
func (stubEngine) SetMuted(bool)    {}
func (stubEngine) SetDeafened(bool) {}
func (stubEngine) Level() float32   { return 0 }
func (stubEngine) Stop()            {}
