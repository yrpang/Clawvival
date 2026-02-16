package runtime

import (
	"context"
	"testing"
	"time"
)

func TestProvider_DaySnapshot(t *testing.T) {
	p := NewProvider(Config{
		DayStartHour:   6,
		NightStart:     18,
		ThreatDay:      2,
		ThreatNight:    5,
		ResourcesDay:   map[string]int{"wood": 12},
		ResourcesNight: map[string]int{"wood": 3},
		Now: func() time.Time {
			return time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
		},
	})

	s, err := p.SnapshotForAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.TimeOfDay != "day" {
		t.Fatalf("expected day, got %q", s.TimeOfDay)
	}
	if s.ThreatLevel != 2 {
		t.Fatalf("expected threat 2, got %d", s.ThreatLevel)
	}
	if s.NearbyResource["wood"] != 12 {
		t.Fatalf("expected wood 12, got %d", s.NearbyResource["wood"])
	}
}

func TestProvider_NightSnapshot(t *testing.T) {
	p := NewProvider(Config{
		DayStartHour:   6,
		NightStart:     18,
		ThreatDay:      1,
		ThreatNight:    4,
		ResourcesDay:   map[string]int{"stone": 8},
		ResourcesNight: map[string]int{"stone": 2},
		Now: func() time.Time {
			return time.Date(2026, 2, 16, 23, 0, 0, 0, time.UTC)
		},
	})

	s, err := p.SnapshotForAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("SnapshotForAgent error: %v", err)
	}
	if s.TimeOfDay != "night" {
		t.Fatalf("expected night, got %q", s.TimeOfDay)
	}
	if s.ThreatLevel != 4 {
		t.Fatalf("expected threat 4, got %d", s.ThreatLevel)
	}
	if s.NearbyResource["stone"] != 2 {
		t.Fatalf("expected stone 2, got %d", s.NearbyResource["stone"])
	}
}
