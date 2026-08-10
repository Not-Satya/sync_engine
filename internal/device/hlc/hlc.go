// Package hlc implements a Hybrid Logical Clock (ADR 16) used to stamp local
// metadata mutations so they can be ordered against events from other devices
// via Last-Write-Wins.
package hlc

import (
	"sync"
	"time"
)

// Clock is a thread-safe Hybrid Logical Clock. Wall is unix nanoseconds; when
// physical time does not advance between calls, Counter increments to preserve
// a strict local ordering.
type Clock struct {
	mu       sync.Mutex
	lastWall int64
	counter  int64
	now      func() time.Time
}

// New returns a clock backed by the system wall clock.
func New() *Clock {
	return &Clock{now: time.Now}
}

// newWithNow is for tests that need a controllable time source.
func newWithNow(now func() time.Time) *Clock {
	return &Clock{now: now}
}

// Now returns the next monotonic HLC stamp for a local event.
func (c *Clock) Now() (wall int64, counter int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	physical := c.now().UnixNano()
	if physical > c.lastWall {
		c.lastWall = physical
		c.counter = 0
	} else {
		c.counter++
	}
	return c.lastWall, c.counter
}

// Observe advances the clock past a stamp seen from another device, keeping the
// local clock causally ahead of anything it has applied. Call this when pulling
// remote events (P4.5) before generating further local stamps.
func (c *Clock) Observe(wall, counter int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	physical := c.now().UnixNano()
	maxWall := c.lastWall
	if wall > maxWall {
		maxWall = wall
	}
	if physical > maxWall {
		c.lastWall = physical
		c.counter = 0
		return
	}
	c.lastWall = maxWall
	if wall == maxWall && counter > c.counter {
		c.counter = counter
	}
}
