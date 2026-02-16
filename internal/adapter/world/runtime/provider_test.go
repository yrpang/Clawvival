package runtime

import (
	"context"
	"testing"
	"time"

	"clawverse/internal/domain/world"
)

func TestProvider_UsesWorldClockAndReportsNextPhase(t *testing.T) {
	start := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	p := NewProvider(Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       start,
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		ThreatDay:      2,
		ThreatNight:    5,
		ResourcesDay:   map[string]int{"wood": 12},
		ResourcesNight: map[string]int{"wood": 3},
		ViewRadius:     2,
		Now: func() time.Time {
			return start.Add(11 * time.Minute)
		},
	})

	s, err := p.SnapshotForAgent(context.Background(), "agent-1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.TimeOfDay != "night" {
		t.Fatalf("expected night, got %q", s.TimeOfDay)
	}
	if s.NextPhaseInSeconds != 240 {
		t.Fatalf("expected 240 seconds until next phase, got %d", s.NextPhaseInSeconds)
	}
}

func TestProvider_WindowCenterAndTiles(t *testing.T) {
	p := NewProvider(DefaultConfig())
	s, err := p.SnapshotForAgent(context.Background(), "agent-1", world.Point{X: 40, Y: 0})
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.Center.X != 40 || s.Center.Y != 0 {
		t.Fatalf("unexpected center: %+v", s.Center)
	}
	if len(s.VisibleTiles) == 0 {
		t.Fatalf("expected visible tiles")
	}
}
