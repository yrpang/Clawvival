package gormrepo

import (
	"context"
	"os"
	"testing"
	"time"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}
	return dsn
}

func TestAgentStateRepo_RoundTripInventoryAndDeath(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	agentID := "it-state-roundtrip"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	repo := NewAgentStateRepo(db)
	seed := survival.AgentStateAggregate{
		AgentID: agentID,
		Vitals:  survival.Vitals{HP: 88, Hunger: 55, Energy: 44},
		Position: survival.Position{X: 2, Y: 3},
		Inventory: map[string]int{"wood": 3, "stone": 1},
		Dead: true,
		DeathCause: survival.DeathCauseCombat,
		Version: 1,
	}
	if err := repo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := repo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Inventory["wood"] != 3 {
		t.Fatalf("expected wood=3, got %d", got.Inventory["wood"])
	}
	if !got.Dead || got.DeathCause != survival.DeathCauseCombat {
		t.Fatalf("expected dead combat, got dead=%v cause=%s", got.Dead, got.DeathCause)
	}
}

func TestWorldObjectAndSessionRepos_PersistLifecycle(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	agentID := "it-repo-lifecycle"
	sessionID := "session-" + agentID
	_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE session_id = ?", sessionID).Error

	objRepo := NewWorldObjectRepo(db)
	sessionRepo := NewAgentSessionRepo(db)

	if err := objRepo.Save(ctx, agentID, ports.WorldObjectRecord{
		ObjectID: "obj-2",
		Kind:     1,
		X:        7,
		Y:        9,
		HP:       100,
	}); err != nil {
		t.Fatalf("save object: %v", err)
	}

	if err := sessionRepo.EnsureActive(ctx, sessionID, agentID, 1); err != nil {
		t.Fatalf("ensure active: %v", err)
	}
	if err := sessionRepo.Close(ctx, sessionID, survival.DeathCauseStarvation, time.Now()); err != nil {
		t.Fatalf("close session: %v", err)
	}

	var obj model.WorldObject
	if err := db.Where("object_id = ?", "obj-2").First(&obj).Error; err != nil {
		t.Fatalf("query object: %v", err)
	}
	if obj.OwnerAgentID != agentID {
		t.Fatalf("expected owner %s, got %s", agentID, obj.OwnerAgentID)
	}

	var s model.AgentSession
	if err := db.Where("session_id = ?", sessionID).First(&s).Error; err != nil {
		t.Fatalf("query session: %v", err)
	}
	if s.Status != "dead" || s.DeathCause == "" {
		t.Fatalf("expected dead session with cause, got status=%s cause=%s", s.Status, s.DeathCause)
	}
}

func TestWorldChunkRepo_ZeroCoordinateRoundTrip(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_chunks").Error

	repo := NewWorldChunkRepo(db)
	otherCoord := world.ChunkCoord{X: 1, Y: -1}
	otherChunk := world.Chunk{
		Coord: otherCoord,
		Tiles: []world.Tile{
			{X: 8, Y: -8, Passable: true, Kind: world.TileGrass},
		},
	}
	if err := repo.SaveChunk(ctx, otherCoord, "day", otherChunk); err != nil {
		t.Fatalf("save other chunk: %v", err)
	}

	coord := world.ChunkCoord{X: 0, Y: -1}
	chunk := world.Chunk{
		Coord: coord,
		Tiles: []world.Tile{
			{X: 0, Y: -8, Passable: true, Kind: world.TileGrass},
			{X: 1, Y: -8, Passable: false, Kind: world.TileTree, Resource: "wood"},
		},
	}
	if err := repo.SaveChunk(ctx, coord, "day", chunk); err != nil {
		t.Fatalf("save chunk: %v", err)
	}

	got, ok, err := repo.GetChunk(ctx, coord, "day")
	if err != nil {
		t.Fatalf("get chunk: %v", err)
	}
	if !ok {
		t.Fatalf("expected chunk found")
	}
	if got.Coord.X != 0 || got.Coord.Y != -1 {
		t.Fatalf("unexpected coord: %+v", got.Coord)
	}
	if len(got.Tiles) != 2 {
		t.Fatalf("expected 2 tiles, got %d", len(got.Tiles))
	}
}
