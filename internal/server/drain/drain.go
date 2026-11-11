// Package drain tracks whether this instance is shutting down.
package drain

import "sync/atomic"

// State is set when the instance starts draining. From then on /ready
// and new extraction requests return 503.
type State struct{ draining atomic.Bool }

// New returns a state that is not draining.
func New() *State { return &State{} }

// Start marks the instance as draining.
func (s *State) Start() { s.draining.Store(true) }

// Draining reports whether the instance is shutting down.
func (s *State) Draining() bool { return s.draining.Load() }
