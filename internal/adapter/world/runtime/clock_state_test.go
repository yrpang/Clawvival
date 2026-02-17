package runtime

import (
	"context"
	"testing"
	"time"

	"clawverse/internal/domain/world"
)

type fakeClockStateStore struct {
	phase      string
	switchedAt time.Time
	saves      int
}

func (s *fakeClockStateStore) Get(_ context.Context) (string, time.Time, bool, error) {
	if s.phase == "" {
		return "", time.Time{}, false, nil
	}
	return s.phase, s.switchedAt, true, nil
}

func (s *fakeClockStateStore) Save(_ context.Context, phase string, switchedAt time.Time) error {
	s.phase = phase
	s.switchedAt = switchedAt
	s.saves++
	return nil
}

func TestProvider_PersistsClockPhaseOnSwitch(t *testing.T) {
	start := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeClockStateStore{}
	now := start.Add(9 * time.Minute)
	p := NewProvider(Config{
		Clock:           world.NewClock(world.ClockConfig{StartAt: start, DayDuration: 10 * time.Minute, NightDuration: 5 * time.Minute}),
		Now:             func() time.Time { return now },
		ClockStateStore: store,
	})

	_, err := p.SnapshotForAgent(context.Background(), "a1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("snapshot1 error: %v", err)
	}
	if store.phase != "day" {
		t.Fatalf("expected stored phase day, got %s", store.phase)
	}
	firstSaves := store.saves

	now = start.Add(11 * time.Minute)
	_, err = p.SnapshotForAgent(context.Background(), "a1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("snapshot2 error: %v", err)
	}
	if store.phase != "night" {
		t.Fatalf("expected stored phase night, got %s", store.phase)
	}
	if store.saves <= firstSaves {
		t.Fatalf("expected new save on phase switch")
	}
}
