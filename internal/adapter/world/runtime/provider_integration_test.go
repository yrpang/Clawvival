package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/domain/world"
)

func TestProvider_GormChunkStoreCachesChunks(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}
	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Exec("DELETE FROM world_chunks").Error; err != nil {
		t.Fatalf("cleanup world_chunks: %v", err)
	}

	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	p := NewProvider(Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       now,
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		ViewRadius: 2,
		Now:        func() time.Time { return now },
		ChunkStore: gormrepo.NewWorldChunkRepo(db),
	})

	ctx := context.Background()
	_, err = p.SnapshotForAgent(ctx, "a1", world.Point{X: 3, Y: 3})
	if err != nil {
		t.Fatalf("snapshot1 error: %v", err)
	}
	var count1 int64
	if err := db.Table("world_chunks").Count(&count1).Error; err != nil {
		t.Fatalf("count1: %v", err)
	}
	if count1 == 0 {
		t.Fatalf("expected cached chunks after first snapshot")
	}

	_, err = p.SnapshotForAgent(ctx, "a1", world.Point{X: 3, Y: 3})
	if err != nil {
		t.Fatalf("snapshot2 error: %v", err)
	}
	var count2 int64
	if err := db.Table("world_chunks").Count(&count2).Error; err != nil {
		t.Fatalf("count2: %v", err)
	}
	if count2 != count1 {
		t.Fatalf("expected cache hit without new rows, count1=%d count2=%d", count1, count2)
	}
}
