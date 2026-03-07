package observe

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawvival/internal/adapter/repo/gorm"
	"clawvival/internal/adapter/repo/gorm/model"
	worldruntime "clawvival/internal/adapter/world/runtime"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func requireObserveDSN(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}
	return dsn
}

func TestUseCase_E2E_ReturnsWindowedMapAndPersistsChunks(t *testing.T) {
	dsn := requireObserveDSN(t)
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
		ChunkStore: gormrepo.NewWorldChunkRepo(db),
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

func TestUseCase_E2E_ObserveSummarizesVisibleResources(t *testing.T) {
	dsn := requireObserveDSN(t)
	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-observe-refresh"
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
		Position:  survival.Position{X: 12, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		ViewRadius:      2,
		Now:             func() time.Time { return now },
		RefreshInterval: 5 * time.Minute,
		ChunkStore:      gormrepo.NewWorldChunkRepo(db),
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})
	uc := UseCase{StateRepo: stateRepo, World: worldProvider}

	first, err := uc.Execute(ctx, Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("first observe execute: %v", err)
	}
	now = now.Add(6 * time.Minute)
	second, err := uc.Execute(ctx, Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("second observe execute: %v", err)
	}

	if len(first.Resources) == 0 || len(second.Resources) == 0 {
		t.Fatalf("expected visible resources in both observes, first=%v second=%v", first.Resources, second.Resources)
	}
	if first.Snapshot.NearbyResource["wood"] != second.Snapshot.NearbyResource["wood"] ||
		first.Snapshot.NearbyResource["stone"] != second.Snapshot.NearbyResource["stone"] {
		t.Fatalf("expected observe nearby summary to reflect visible resources consistently, first=%v second=%v", first.Snapshot.NearbyResource, second.Snapshot.NearbyResource)
	}
}
