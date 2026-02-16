package runtime

import (
	"context"
	"testing"
	"time"

	"clawverse/internal/domain/world"
)

func TestProvider_DaySnapshotHasWindow(t *testing.T) {
	p := NewProvider(Config{
		DayStartHour:   6,
		NightStart:     18,
		ThreatDay:      2,
		ThreatNight:    5,
		ResourcesDay:   map[string]int{"wood": 12},
		ResourcesNight: map[string]int{"wood": 3},
		ViewRadius:     2,
		Now: func() time.Time {
			return time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
		},
	})

	s, err := p.SnapshotForAgent(context.Background(), "agent-1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.TimeOfDay != "day" {
		t.Fatalf("expected day, got %q", s.TimeOfDay)
	}
	if s.ThreatLevel != 2 {
		t.Fatalf("expected threat 2, got %d", s.ThreatLevel)
	}
	if s.Center.X != 0 || s.Center.Y != 0 {
		t.Fatalf("unexpected center: %+v", s.Center)
	}
	if s.ViewRadius != 2 {
		t.Fatalf("expected radius 2, got %d", s.ViewRadius)
	}
	if len(s.VisibleTiles) != 25 {
		t.Fatalf("expected 25 tiles, got %d", len(s.VisibleTiles))
	}
}

func TestProvider_NightSnapshotThreatAndZones(t *testing.T) {
	p := NewProvider(Config{
		DayStartHour:   6,
		NightStart:     18,
		ThreatDay:      1,
		ThreatNight:    4,
		ResourcesDay:   map[string]int{"stone": 8},
		ResourcesNight: map[string]int{"stone": 2},
		ViewRadius:     1,
		Now: func() time.Time {
			return time.Date(2026, 2, 16, 23, 0, 0, 0, time.UTC)
		},
	})

	s, err := p.SnapshotForAgent(context.Background(), "agent-1", world.Point{X: 40, Y: 0})
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.TimeOfDay != "night" {
		t.Fatalf("expected night, got %q", s.TimeOfDay)
	}
	if s.ThreatLevel != 4 {
		t.Fatalf("expected threat 4, got %d", s.ThreatLevel)
	}
	if len(s.VisibleTiles) != 9 {
		t.Fatalf("expected 9 tiles, got %d", len(s.VisibleTiles))
	}
	for _, tile := range s.VisibleTiles {
		if tile.Zone != world.ZoneWild {
			t.Fatalf("expected wild zone, got %s", tile.Zone)
		}
	}
}
