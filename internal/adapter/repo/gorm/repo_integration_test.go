package gormrepo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
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
		AgentID:           agentID,
		Vitals:            survival.Vitals{HP: 88, Hunger: 55, Energy: 44},
		Position:          survival.Position{X: 2, Y: 3},
		Inventory:         map[string]int{"wood": 3, "stone": 1},
		InventoryCapacity: 40,
		InventoryUsed:     4,
		Dead:              true,
		DeathCause:        survival.DeathCauseThreat,
		Version:           1,
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
	if got.InventoryCapacity != 40 || got.InventoryUsed != 4 {
		t.Fatalf("expected inventory cap/used 40/4, got %d/%d", got.InventoryCapacity, got.InventoryUsed)
	}
	if !got.Dead || got.DeathCause != survival.DeathCauseThreat {
		t.Fatalf("expected dead threat, got dead=%v cause=%s", got.Dead, got.DeathCause)
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
		ObjectID:      "obj-2",
		Kind:          1,
		X:             7,
		Y:             9,
		HP:            100,
		ObjectType:    "box",
		CapacitySlots: 60,
		ObjectState:   `{"inventory":{}}`,
	}); err != nil {
		t.Fatalf("save object: %v", err)
	}
	saved, err := objRepo.GetByObjectID(ctx, agentID, "obj-2")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if saved.ObjectType != "box" || saved.CapacitySlots != 60 {
		t.Fatalf("unexpected object defaults: %+v", saved)
	}
	saved.UsedSlots = 5
	saved.ObjectState = `{"inventory":{"wood":5}}`
	if err := objRepo.Update(ctx, agentID, saved); err != nil {
		t.Fatalf("update object: %v", err)
	}
	updated, err := objRepo.GetByObjectID(ctx, agentID, "obj-2")
	if err != nil {
		t.Fatalf("get updated object: %v", err)
	}
	if updated.UsedSlots != 5 || updated.ObjectState == "" {
		t.Fatalf("unexpected updated object: %+v", updated)
	}
	list, err := objRepo.ListByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("list objects: %v", err)
	}
	if len(list) != 1 || list[0].ObjectID != "obj-2" {
		t.Fatalf("unexpected object list: %+v", list)
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

func TestActionExecutionRepo_SaveAndGetRoundTrip(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	agentID := "it-action-exec"
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error

	repo := NewActionExecutionRepo(db)
	rec := ports.ActionExecutionRecord{
		AgentID:        agentID,
		IdempotencyKey: "key-1",
		IntentType:     "gather",
		DT:             30,
		Result: ports.ActionResult{
			UpdatedState: survival.AgentStateAggregate{
				AgentID:  agentID,
				Vitals:   survival.Vitals{HP: 90, Hunger: 70, Energy: 50},
				Position: survival.Position{X: 1, Y: 2},
				Version:  2,
			},
			Events: []survival.DomainEvent{
				{Type: "action_settled", OccurredAt: time.Unix(10, 0), Payload: map[string]any{"decision": map[string]any{"intent": "gather"}}},
			},
			ResultCode: survival.ResultOK,
		},
		AppliedAt: time.Unix(20, 0),
	}
	if err := repo.SaveExecution(ctx, rec); err != nil {
		t.Fatalf("save execution: %v", err)
	}
	got, err := repo.GetByIdempotencyKey(ctx, agentID, "key-1")
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if got.Result.UpdatedState.Version != 2 {
		t.Fatalf("expected version 2, got %d", got.Result.UpdatedState.Version)
	}
	if len(got.Result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Result.Events))
	}
	if _, err := repo.GetByIdempotencyKey(ctx, agentID, "missing"); err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing key, got %v", err)
	}
}

func TestEventRepo_AppendAndListByAgentID(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	agentID := "it-event-repo"
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error

	repo := NewEventRepo(db)
	if err := repo.Append(ctx, agentID, []survival.DomainEvent{
		{Type: "e-old", OccurredAt: time.Unix(100, 0), Payload: map[string]any{"k": "v1"}},
		{Type: "e-new", OccurredAt: time.Unix(200, 0), Payload: map[string]any{"k": "v2"}},
	}); err != nil {
		t.Fatalf("append events: %v", err)
	}

	list, err := repo.ListByAgentID(ctx, agentID, 1)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(list) != 1 || list[0].Type != "e-new" {
		t.Fatalf("expected only latest event, got=%+v", list)
	}
	all, err := repo.ListByAgentID(ctx, agentID, 0)
	if err != nil {
		t.Fatalf("list all events: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}
}

func TestWorldClockStateRepo_SaveAndGet(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_clock_state").Error

	repo := NewWorldClockStateRepo(db)
	if _, _, ok, err := repo.Get(ctx); err != nil || ok {
		t.Fatalf("expected no state initially, ok=%v err=%v", ok, err)
	}
	t1 := time.Unix(300, 0)
	if err := repo.Save(ctx, "day", t1); err != nil {
		t.Fatalf("save day: %v", err)
	}
	phase, switchedAt, ok, err := repo.Get(ctx)
	if err != nil || !ok {
		t.Fatalf("get day state failed: ok=%v err=%v", ok, err)
	}
	if phase != "day" || !switchedAt.Equal(t1) {
		t.Fatalf("unexpected day state: phase=%s switched=%v", phase, switchedAt)
	}
	t2 := time.Unix(600, 0)
	if err := repo.Save(ctx, "night", t2); err != nil {
		t.Fatalf("save night: %v", err)
	}
	phase, switchedAt, ok, err = repo.Get(ctx)
	if err != nil || !ok {
		t.Fatalf("get night state failed: ok=%v err=%v", ok, err)
	}
	if phase != "night" || !switchedAt.Equal(t2) {
		t.Fatalf("unexpected night state: phase=%s switched=%v", phase, switchedAt)
	}
}

func TestTxManager_RunInTxCommitAndRollback(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	agentID := "it-tx-manager"
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	txManager := NewTxManager(db)
	stateRepo := NewAgentStateRepo(db)

	commitErr := txManager.RunInTx(ctx, func(txCtx context.Context) error {
		return stateRepo.SaveWithVersion(txCtx, survival.AgentStateAggregate{
			AgentID:  agentID,
			Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position: survival.Position{X: 0, Y: 0},
			Version:  1,
		}, 0)
	})
	if commitErr != nil {
		t.Fatalf("commit tx failed: %v", commitErr)
	}
	if _, err := stateRepo.GetByAgentID(ctx, agentID); err != nil {
		t.Fatalf("expected committed state exists, got err=%v", err)
	}

	rollbackErr := txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := stateRepo.SaveWithVersion(txCtx, survival.AgentStateAggregate{
			AgentID:  agentID + "-rb",
			Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position: survival.Position{X: 0, Y: 0},
			Version:  1,
		}, 0); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if rollbackErr == nil {
		t.Fatalf("expected rollback error")
	}
	if _, err := stateRepo.GetByAgentID(ctx, agentID+"-rb"); err != ports.ErrNotFound {
		t.Fatalf("expected rollback to remove state, got err=%v", err)
	}
}

func TestAgentCredentialRepo_CreateGetAndConflict(t *testing.T) {
	dsn := requireDSN(t)
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx := context.Background()
	agentID := "it-agent-credential"
	_ = db.Exec("DELETE FROM agent_credentials WHERE agent_id = ?", agentID).Error

	repo := NewAgentCredentialRepo(db)
	rec := ports.AgentCredentialRecord{
		AgentID:   agentID,
		KeySalt:   []byte("salt"),
		KeyHash:   []byte("hash"),
		Status:    "active",
		CreatedAt: time.Unix(1000, 0).UTC(),
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create credential: %v", err)
	}
	got, err := repo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if got.AgentID != agentID || got.Status != "active" {
		t.Fatalf("unexpected credential: %+v", got)
	}
	if err := repo.Create(ctx, rec); err != ports.ErrConflict {
		t.Fatalf("expected conflict on duplicate create, got %v", err)
	}
	if _, err := repo.GetByAgentID(ctx, agentID+"-missing"); err != ports.ErrNotFound {
		t.Fatalf("expected not found on missing credential, got %v", err)
	}
}
