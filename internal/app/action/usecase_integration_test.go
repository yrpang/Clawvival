package action

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/adapter/repo/gorm/model"
	worldmock "clawverse/internal/adapter/world/mock"
	worldruntime "clawverse/internal/adapter/world/runtime"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_E2E_PersistsWorldObjectAndSessionLifecycle(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-agent-e2e"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup world_objects: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_sessions: %v", err)
	}
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	objRepo := gormrepo.NewWorldObjectRepo(db)
	sessionRepo := gormrepo.NewAgentSessionRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{"plank": 10},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	uc := UseCase{
		TxManager:   txManager,
		StateRepo:   stateRepo,
		ActionRepo:  actionRepo,
		EventRepo:   eventRepo,
		ObjectRepo:  objRepo,
		SessionRepo: sessionRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 1},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "build-1",
		Intent: survival.ActionIntent{
			Type:   survival.ActionBuild,
			Params: map[string]int{"kind": int(survival.BuildBed)},
		},
		DeltaMinutes: 30,
	})
	if err != nil {
		t.Fatalf("build execute: %v", err)
	}

	var objs []model.WorldObject
	if err := db.Where("owner_agent_id = ?", agentID).Find(&objs).Error; err != nil {
		t.Fatalf("query world_objects: %v", err)
	}
	if len(objs) == 0 {
		t.Fatalf("expected world_objects persisted")
	}

	var session model.AgentSession
	if err := db.Where("agent_id = ?", agentID).First(&session).Error; err != nil {
		t.Fatalf("query session: %v", err)
	}
	if session.Status != "alive" {
		t.Fatalf("expected session alive, got %s", session.Status)
	}

	st, err := stateRepo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	st.Vitals.HP = 1
	st.Vitals.Hunger = -100
	st.Vitals.Energy = -100
	if err := stateRepo.SaveWithVersion(ctx, st, st.Version); err != nil {
		t.Fatalf("prepare gameover state: %v", err)
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "die-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	})
	if err != nil {
		t.Fatalf("gameover execute: %v", err)
	}

	if err := db.Where("agent_id = ?", agentID).First(&session).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.Status != "dead" {
		t.Fatalf("expected session dead, got %s", session.Status)
	}
	if session.DeathCause == "" {
		t.Fatalf("expected death cause recorded")
	}
}

func TestUseCase_E2E_GatherAppliesToolEfficiency(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-gather-tools"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:  agentID,
		Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{
			"tool_axe":     1,
			"tool_pickaxe": 1,
		},
		Version: 1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 2, "stone": 3},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700001000, 0) },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "gather-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	})
	if err != nil {
		t.Fatalf("gather execute: %v", err)
	}

	st, err := stateRepo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if got, want := st.Inventory["wood"], 4; got != want {
		t.Fatalf("wood gather mismatch: got=%d want=%d", got, want)
	}
	if got, want := st.Inventory["stone"], 6; got != want {
		t.Fatalf("stone gather mismatch: got=%d want=%d", got, want)
	}
}

func TestUseCase_E2E_CriticalHPTriggersAutoRetreat(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-critical-retreat"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 22, Hunger: -120, Energy: 10},
		Position:  survival.Position{X: 5, Y: 5},
		Home:      survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "night",
			ThreatLevel:    3,
			NearbyResource: map[string]int{},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700002000, 0) },
	}

	resp, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "combat-critical-1",
		Intent:         survival.ActionIntent{Type: survival.ActionCombat, Params: map[string]int{"target_level": 1}},
		DeltaMinutes:   30,
	})
	if err != nil {
		t.Fatalf("combat execute: %v", err)
	}
	if resp.UpdatedState.Position.X != 4 || resp.UpdatedState.Position.Y != 4 {
		t.Fatalf("expected auto retreat position (4,4), got (%d,%d)", resp.UpdatedState.Position.X, resp.UpdatedState.Position.Y)
	}

	foundRetreat := false
	for _, evt := range resp.Events {
		if evt.Type == "force_retreat" {
			foundRetreat = true
			break
		}
	}
	if !foundRetreat {
		t.Fatalf("expected force_retreat event in response")
	}

	st, err := stateRepo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if st.Position.X != 4 || st.Position.Y != 4 {
		t.Fatalf("expected persisted retreat position (4,4), got (%d,%d)", st.Position.X, st.Position.Y)
	}
}

func TestUseCase_E2E_InvalidActionParamsRejectedWithoutPersistence(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-invalid-action"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700003000, 0) },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "invalid-move-1",
		Intent:         survival.ActionIntent{Type: survival.ActionMove},
		DeltaMinutes:   30,
	})
	if !errors.Is(err, ErrInvalidActionParams) {
		t.Fatalf("expected ErrInvalidActionParams, got %v", err)
	}

	var executionCount int64
	if err := db.Table("action_executions").Where("agent_id = ?", agentID).Count(&executionCount).Error; err != nil {
		t.Fatalf("count action_executions: %v", err)
	}
	if executionCount != 0 {
		t.Fatalf("expected no persisted execution for invalid params, got=%d", executionCount)
	}
	var eventCount int64
	if err := db.Table("domain_events").Where("agent_id = ?", agentID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count domain_events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected no persisted events for invalid params, got=%d", eventCount)
	}
}

func TestUseCase_E2E_EmitsWorldPhaseChangedEventOnClockSwitch(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-world-phase-switch"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}
	if err := db.Exec("DELETE FROM world_clock_state").Error; err != nil {
		t.Fatalf("cleanup world_clock_state: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0).Add(9 * time.Minute)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ClockStateStore: worldruntime.NewGormClockStateStore(db),
	})

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "phase-day",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	}); err != nil {
		t.Fatalf("first execute: %v", err)
	}

	now = time.Unix(0, 0).Add(11 * time.Minute)
	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "phase-night",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	}); err != nil {
		t.Fatalf("second execute: %v", err)
	}

	events, err := eventRepo.ListByAgentID(ctx, agentID, 20)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	found := false
	for _, evt := range events {
		if evt.Type != "world_phase_changed" {
			continue
		}
		found = true
		if evt.Payload["from"] != "day" || evt.Payload["to"] != "night" {
			t.Fatalf("unexpected phase payload: %+v", evt.Payload)
		}
		break
	}
	if !found {
		t.Fatalf("expected world_phase_changed event")
	}
}

func TestUseCase_E2E_RejectsBuildWhenResourcesInsufficient(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-build-precheck"
	ctx := context.Background()
	if err := db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup action_executions: %v", err)
	}
	if err := db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup domain_events: %v", err)
	}
	if err := db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error; err != nil {
		t.Fatalf("cleanup agent_states: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700005000, 0) },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "build-precheck-1",
		Intent: survival.ActionIntent{
			Type:   survival.ActionBuild,
			Params: map[string]int{"kind": int(survival.BuildBed)},
		},
		DeltaMinutes: 30,
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}

	var executionCount int64
	if err := db.Table("action_executions").Where("agent_id = ?", agentID).Count(&executionCount).Error; err != nil {
		t.Fatalf("count action_executions: %v", err)
	}
	if executionCount != 0 {
		t.Fatalf("expected no persisted execution for precheck failure, got=%d", executionCount)
	}
}
