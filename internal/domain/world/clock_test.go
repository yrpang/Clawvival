package world

import (
	"testing"
	"time"
)

func TestClockPhaseCycle(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewClock(ClockConfig{
		StartAt:       start,
		DayDuration:   10 * time.Minute,
		NightDuration: 5 * time.Minute,
	})

	phase, remain := clock.PhaseAt(start)
	if phase != PhaseDay {
		t.Fatalf("expected day at start, got %s", phase)
	}
	if remain != 10*time.Minute {
		t.Fatalf("expected 10m remain, got %s", remain)
	}

	phase, remain = clock.PhaseAt(start.Add(11 * time.Minute))
	if phase != PhaseNight {
		t.Fatalf("expected night at +11m, got %s", phase)
	}
	if remain != 4*time.Minute {
		t.Fatalf("expected 4m remain, got %s", remain)
	}

	phase, _ = clock.PhaseAt(start.Add(16 * time.Minute))
	if phase != PhaseDay {
		t.Fatalf("expected cycle back to day, got %s", phase)
	}
}
