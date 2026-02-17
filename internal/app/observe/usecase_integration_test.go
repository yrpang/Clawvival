package observe

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/adapter/repo/gorm/model"
	worldruntime "clawverse/internal/adapter/world/runtime"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_E2E_ReturnsWindowedMapAndPersistsChunks(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-observe-map"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM world_chunks WHERE phase = 'day'").Error; err != nil {
		t.Fatalf("cleanup world_chunks: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 100, Energy: 100},
		Position:  survival.Position{X: 6, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		ViewRadius: 2,
		Now:        func() time.Time { return time.Unix(0, 0) },
		ChunkStore: worldruntime.NewGormChunkStore(db),
	})
	uc := UseCase{StateRepo: stateRepo, World: worldProvider}

	resp, err := uc.Execute(ctx, Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe execute: %v", err)
	}
	if got, want := len(resp.Snapshot.VisibleTiles), 25; got != want {
		t.Fatalf("visible tiles mismatch: got=%d want=%d", got, want)
	}
	if resp.Snapshot.Center.X != 6 || resp.Snapshot.Center.Y != 0 {
		t.Fatalf("unexpected center: %+v", resp.Snapshot.Center)
	}

	zoneSet := map[world.Zone]bool{}
	for _, tile := range resp.Snapshot.VisibleTiles {
		zoneSet[tile.Zone] = true
	}
	if !zoneSet[world.ZoneSafe] || !zoneSet[world.ZoneForest] {
		t.Fatalf("expected safe+forest zones in window, got=%v", zoneSet)
	}

	var rows []model.WorldChunk
	if err := db.Where("phase = ?", "day").Find(&rows).Error; err != nil {
		t.Fatalf("query world_chunks: %v", err)
	}
	if got, want := len(rows), 4; got != want {
		t.Fatalf("chunk rows mismatch: got=%d want=%d", got, want)
	}

	if _, err := uc.Execute(ctx, Request{AgentID: agentID}); err != nil {
		t.Fatalf("second observe execute: %v", err)
	}
	var rowsAfter []model.WorldChunk
	if err := db.Where("phase = ?", "day").Find(&rowsAfter).Error; err != nil {
		t.Fatalf("query world_chunks after second observe: %v", err)
	}
	if got, want := len(rowsAfter), 4; got != want {
		t.Fatalf("chunk rows should be reused: got=%d want=%d", got, want)
	}
}

