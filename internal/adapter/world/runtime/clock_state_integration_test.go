package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/domain/world"
)

func TestProvider_GormClockStateStorePersistsPhaseSwitch(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}
	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Exec("DELETE FROM world_clock_state").Error; err != nil {
		t.Fatalf("cleanup world_clock_state: %v", err)
	}

	start := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
	now := start.Add(9 * time.Minute)
	p := NewProvider(Config{
		Clock:           world.NewClock(world.ClockConfig{StartAt: start, DayDuration: 10 * time.Minute, NightDuration: 5 * time.Minute}),
		Now:             func() time.Time { return now },
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})

	ctx := context.Background()
	_, err = p.SnapshotForAgent(ctx, "a1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("snapshot day: %v", err)
	}
	phase, _, ok, err := gormrepo.NewWorldClockStateRepo(db).Get(ctx)
	if err != nil || !ok {
		t.Fatalf("get day phase error=%v ok=%v", err, ok)
	}
	if phase != "day" {
		t.Fatalf("expected day phase, got %s", phase)
	}

	now = start.Add(11 * time.Minute)
	_, err = p.SnapshotForAgent(ctx, "a1", world.Point{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("snapshot night: %v", err)
	}
	phase, _, ok, err = gormrepo.NewWorldClockStateRepo(db).Get(ctx)
	if err != nil || !ok {
		t.Fatalf("get night phase error=%v ok=%v", err, ok)
	}
	if phase != "night" {
		t.Fatalf("expected night phase, got %s", phase)
	}
}
