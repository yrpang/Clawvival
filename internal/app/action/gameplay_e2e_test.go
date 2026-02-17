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
