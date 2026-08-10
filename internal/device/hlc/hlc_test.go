package hlc

import (
	"testing"
	"time"
)

func TestNowMonotonicWithinSameWall(t *testing.T) {
	fixed := time.Unix(1000, 0)
	c := newWithNow(func() time.Time { return fixed })

	w1, ctr1 := c.Now()
	w2, ctr2 := c.Now()
	w3, ctr3 := c.Now()

	if w1 != w2 || w2 != w3 {
		t.Fatalf("wall should be stable: %d %d %d", w1, w2, w3)
	}
	if ctr1 != 0 || ctr2 != 1 || ctr3 != 2 {
		t.Fatalf("counter should increment: %d %d %d", ctr1, ctr2, ctr3)
	}
}

func TestNowResetsCounterOnWallAdvance(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newWithNow(func() time.Time { return now })

	_, ctr0 := c.Now()
	if ctr0 != 0 {
		t.Fatalf("first counter = %d", ctr0)
	}
	now = time.Unix(1001, 0)
	w, ctr := c.Now()
	if ctr != 0 {
		t.Fatalf("counter should reset on wall advance, got %d", ctr)
	}
	if w != now.UnixNano() {
		t.Fatalf("wall = %d want %d", w, now.UnixNano())
	}
}

func TestObserveKeepsClockAhead(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newWithNow(func() time.Time { return now })

	future := time.Unix(5000, 0).UnixNano()
	c.Observe(future, 7)

	w, ctr := c.Now()
	if w != future {
		t.Fatalf("wall should track observed future %d, got %d", future, w)
	}
	if ctr <= 7 {
		t.Fatalf("counter should advance past observed 7, got %d", ctr)
	}
}
