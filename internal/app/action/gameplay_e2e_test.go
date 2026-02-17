package action

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/adapter/repo/gorm/model"
	worldruntime "clawverse/internal/adapter/world/runtime"
	"clawverse/internal/app/observe"
	"clawverse/internal/app/replay"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestGameplayLoop_E2E_ObserveActionStatusReplay(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-gameplay-loop"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	objectRepo := gormrepo.NewWorldObjectRepo(db)
	sessionRepo := gormrepo.NewAgentSessionRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:  agentID,
		Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: survival.Position{X: 0, Y: 0},
		Home:     survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{
			"plank": 4,
			"seed":  1,
			"wood":  2,
		},
		Version: 1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ChunkStore:      gormrepo.NewWorldChunkRepo(db),
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})

	observeUC := observe.UseCase{StateRepo: stateRepo, World: worldProvider}
	actionUC := UseCase{
		TxManager:   txManager,
		StateRepo:   stateRepo,
		ActionRepo:  actionRepo,
		EventRepo:   eventRepo,
		ObjectRepo:  objectRepo,
		SessionRepo: sessionRepo,
		World:       worldProvider,
		Settle:      survival.SettlementService{},
		Now:         func() time.Time { return now },
	}
	statusUC := status.UseCase{StateRepo: stateRepo, World: worldProvider}
	replayUC := replay.UseCase{Events: eventRepo}

	obs, err := observeUC.Execute(ctx, observe.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if len(obs.Snapshot.VisibleTiles) == 0 {
		t.Fatalf("expected visible tiles from observe")
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
		StrategyHash:   "sha-loop",
	}); err != nil {
		t.Fatalf("gather: %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-move",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Params: map[string]int{"dx": 1, "dy": 0}},
		DeltaMinutes:   30,
		StrategyHash:   "sha-loop",
	}); err != nil {
		t.Fatalf("move: %v", err)
	}

	now = now.Add(6 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-build",
		Intent:         survival.ActionIntent{Type: survival.ActionBuild, Params: map[string]int{"kind": int(survival.BuildBed)}},
		DeltaMinutes:   30,
		StrategyHash:   "sha-loop",
	}); err != nil {
		t.Fatalf("build: %v", err)
	}

	now = now.Add(4 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-farm",
		Intent:         survival.ActionIntent{Type: survival.ActionFarm, Params: map[string]int{"seed": 1}},
		DeltaMinutes:   30,
		StrategyHash:   "sha-loop",
	}); err != nil {
		t.Fatalf("farm: %v", err)
	}

	now = now.Add(11 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-combat",
		Intent:         survival.ActionIntent{Type: survival.ActionCombat, Params: map[string]int{"target_level": 1}},
		DeltaMinutes:   30,
		StrategyHash:   "sha-loop",
	}); err != nil {
		t.Fatalf("combat: %v", err)
	}

	st, err := statusUC.Execute(ctx, status.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.State.Version <= 1 {
		t.Fatalf("expected state version to advance, got=%d", st.State.Version)
	}
	if st.TimeOfDay == "" {
		t.Fatalf("expected status time of day")
	}

	rep, err := replayUC.Execute(ctx, replay.Request{AgentID: agentID, Limit: 100})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(rep.Events) == 0 {
		t.Fatalf("expected replay events")
	}
	if rep.LatestState.AgentID != agentID {
		t.Fatalf("latest state agent mismatch: got=%s", rep.LatestState.AgentID)
	}

	var objs []model.WorldObject
	if err := db.Where("owner_agent_id = ?", agentID).Find(&objs).Error; err != nil {
		t.Fatalf("query world_objects: %v", err)
	}
	if len(objs) == 0 {
		t.Fatalf("expected built world objects from gameplay loop")
	}
}

func TestGameplayLoop_E2E_AgentIsolationAndSharedWorldClock(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentA := "it-isolation-a"
	agentB := "it-isolation-b"
	for _, agentID := range []string{agentA, agentB} {
		_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	}
	_ = db.Exec("DELETE FROM world_clock_state").Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seedA := survival.AgentStateAggregate{
		AgentID:    agentA,
		Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:   survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{"wood": 1},
		Dead:       false,
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	seedB := survival.AgentStateAggregate{
		AgentID:    agentB,
		Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:   survival.Position{X: 5, Y: 5},
		Inventory:  map[string]int{"wood": 1},
		Dead:       false,
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seedA, 0); err != nil {
		t.Fatalf("seed state A: %v", err)
	}
	if err := stateRepo.SaveWithVersion(ctx, seedB, 0); err != nil {
		t.Fatalf("seed state B: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ChunkStore:      gormrepo.NewWorldChunkRepo(db),
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})
	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}
	statusUC := status.UseCase{StateRepo: stateRepo, World: worldProvider}

	// world clock is global: first call persists day phase into shared row.
	stA1, err := statusUC.Execute(ctx, status.Request{AgentID: agentA})
	if err != nil {
		t.Fatalf("status A first: %v", err)
	}
	stB1, err := statusUC.Execute(ctx, status.Request{AgentID: agentB})
	if err != nil {
		t.Fatalf("status B first: %v", err)
	}
	if stA1.TimeOfDay != stB1.TimeOfDay {
		t.Fatalf("expected shared world phase, got A=%s B=%s", stA1.TimeOfDay, stB1.TimeOfDay)
	}

	// state mutation is isolated by agent_id.
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentA,
		IdempotencyKey: "isolation-a-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
		StrategyHash:   "sha-isolation",
	}); err != nil {
		t.Fatalf("action A gather: %v", err)
	}

	afterA, err := stateRepo.GetByAgentID(ctx, agentA)
	if err != nil {
		t.Fatalf("load state A: %v", err)
	}
	afterB, err := stateRepo.GetByAgentID(ctx, agentB)
	if err != nil {
		t.Fatalf("load state B: %v", err)
	}
	if afterA.Version <= seedA.Version {
		t.Fatalf("expected A version to advance, before=%d after=%d", seedA.Version, afterA.Version)
	}
	if afterB.Version != seedB.Version {
		t.Fatalf("expected B version unchanged, before=%d after=%d", seedB.Version, afterB.Version)
	}

	now = now.Add(11 * time.Minute)
	stA2, err := statusUC.Execute(ctx, status.Request{AgentID: agentA})
	if err != nil {
		t.Fatalf("status A second: %v", err)
	}
	stB2, err := statusUC.Execute(ctx, status.Request{AgentID: agentB})
	if err != nil {
		t.Fatalf("status B second: %v", err)
	}
	if stA2.TimeOfDay != "night" || stB2.TimeOfDay != "night" {
		t.Fatalf("expected both agents in night phase, got A=%s B=%s", stA2.TimeOfDay, stB2.TimeOfDay)
	}

	var clockRows []model.WorldClockState
	if err := db.Find(&clockRows).Error; err != nil {
		t.Fatalf("query world_clock_state: %v", err)
	}
	if len(clockRows) != 1 {
		t.Fatalf("expected single global clock row, got=%d", len(clockRows))
	}
	if clockRows[0].StateKey != "global" {
		t.Fatalf("expected global clock state key, got=%s", clockRows[0].StateKey)
	}
}
