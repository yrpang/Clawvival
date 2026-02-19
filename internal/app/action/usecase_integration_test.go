package action

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	gormrepo "clawvival/internal/adapter/repo/gorm"
	"clawvival/internal/adapter/repo/gorm/model"
	worldmock "clawvival/internal/adapter/world/mock"
	worldruntime "clawvival/internal/adapter/world/runtime"
	"clawvival/internal/app/observe"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_E2E_PersistsWorldObjectAndSessionLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
		Inventory: map[string]int{"wood": 10},
		Version:   1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(1700000000, 0)
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
		Now:    func() time.Time { return now },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "build-1",
		Intent: survival.ActionIntent{
			Type:       survival.ActionBuild,
			ObjectType: "bed_rough",
			Pos:        &survival.Position{X: 0, Y: 0},
		}})
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
	now = now.Add(30 * time.Minute)

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "die-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}})
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
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
				{X: 1, Y: 0, Passable: true, Resource: "stone"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700001000, 0) },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "gather-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}})
	if err != nil {
		t.Fatalf("gather execute: %v", err)
	}
	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "gather-2",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_1_0_stone"}})
	if err != nil {
		t.Fatalf("second gather execute: %v", err)
	}

	st, err := stateRepo.GetByAgentID(ctx, agentID)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if got, want := st.Inventory["wood"], 2; got != want {
		t.Fatalf("wood gather mismatch: got=%d want=%d", got, want)
	}
	if got, want := st.Inventory["stone"], 2; got != want {
		t.Fatalf("stone gather mismatch: got=%d want=%d", got, want)
	}
}

func TestUseCase_E2E_CriticalHPTriggersAutoRetreat(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
		IdempotencyKey: "gather-critical-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_5_5_wood"}})
	if err != nil {
		t.Fatalf("gather execute: %v", err)
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
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
		Intent:         survival.ActionIntent{Type: survival.ActionMove}})
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
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
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
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat}}); err != nil {
		t.Fatalf("first execute: %v", err)
	}

	now = time.Unix(0, 0).Add(11 * time.Minute)
	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "phase-night",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat}}); err != nil {
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
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
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
			Type:       survival.ActionBuild,
			ObjectType: "bed_rough",
			Pos:        &survival.Position{X: 0, Y: 0},
		}})
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

func TestUseCase_E2E_RejectsMoveDuringCooldown(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-cooldown-precheck"
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
	now := time.Unix(1700006000, 0)

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

	if err := eventRepo.Append(ctx, agentID, []survival.DomainEvent{
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-30 * time.Second),
			Payload: map[string]any{
				"decision": map[string]any{"intent": "move"},
			},
		},
	}); err != nil {
		t.Fatalf("seed recent move event: %v", err)
	}

	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "night",
			ThreatLevel: 3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	_, err = uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "move-cooldown-1",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Direction: "E"}})
	if !errors.Is(err, ErrActionCooldownActive) {
		t.Fatalf("expected ErrActionCooldownActive, got %v", err)
	}

	var executionCount int64
	if err := db.Table("action_executions").Where("agent_id = ?", agentID).Count(&executionCount).Error; err != nil {
		t.Fatalf("count action_executions: %v", err)
	}
	if executionCount != 0 {
		t.Fatalf("expected no persisted execution for cooldown failure, got=%d", executionCount)
	}
}

func TestUseCase_E2E_ContainerDepositWithdraw_PersistsBoxState(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-container-box-state"
	containerID := "obj-box-it-1"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	objRepo := gormrepo.NewWorldObjectRepo(db)
	txManager := gormrepo.NewTxManager(db)

	if err := stateRepo.SaveWithVersion(ctx, survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 90, Energy: 90},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{"wood": 6},
		Version:   1,
	}, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if err := db.Create(&model.WorldObject{
		ObjectID:      containerID,
		Kind:          "2",
		X:             0,
		Y:             0,
		Hp:            100,
		OwnerAgentID:  agentID,
		ObjectType:    "box",
		CapacitySlots: 60,
		UsedSlots:     0,
		ObjectState:   `{"inventory":{}}`,
	}).Error; err != nil {
		t.Fatalf("seed box object: %v", err)
	}

	now := time.Unix(1700010000, 0)
	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "box-deposit-1",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerDeposit,
			ContainerID: containerID,
			Items:       []survival.ItemAmount{{ItemType: "wood", Count: 4}},
		},
	}); err != nil {
		t.Fatalf("container deposit: %v", err)
	}

	now = now.Add(5 * time.Minute)
	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "box-withdraw-1",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerWithdraw,
			ContainerID: containerID,
			Items:       []survival.ItemAmount{{ItemType: "wood", Count: 3}},
		},
	}); err != nil {
		t.Fatalf("container withdraw: %v", err)
	}

	var box model.WorldObject
	if err := db.Where("owner_agent_id = ? AND object_id = ?", agentID, containerID).First(&box).Error; err != nil {
		t.Fatalf("query box object: %v", err)
	}
	if got, want := int(box.UsedSlots), 1; got != want {
		t.Fatalf("expected box used_slots=%d, got %d", want, got)
	}
	var state map[string]map[string]int
	if err := json.Unmarshal([]byte(box.ObjectState), &state); err != nil {
		t.Fatalf("unmarshal box object_state: %v", err)
	}
	if got, want := state["inventory"]["wood"], 1; got != want {
		t.Fatalf("expected box wood=%d, got %d", want, got)
	}
}

func TestUseCase_E2E_FarmPlantHarvest_UsesFarmObjectStateMachine(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-farm-object-state"
	farmID := "obj-farm-it-1"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	objRepo := gormrepo.NewWorldObjectRepo(db)
	txManager := gormrepo.NewTxManager(db)

	if err := stateRepo.SaveWithVersion(ctx, survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 90, Energy: 90},
		Position:  survival.Position{X: 1, Y: 1},
		Inventory: map[string]int{"seed": 1},
		Version:   1,
	}, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if err := db.Create(&model.WorldObject{
		ObjectID:     farmID,
		Kind:         "3",
		X:            1,
		Y:            1,
		Hp:           100,
		OwnerAgentID: agentID,
		ObjectType:   "farm_plot",
		ObjectState:  `{"state":"IDLE"}`,
	}).Error; err != nil {
		t.Fatalf("seed farm object: %v", err)
	}

	now := time.Unix(1700020000, 0)
	uc := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "farm-plant-1",
		Intent: survival.ActionIntent{
			Type:   survival.ActionFarmPlant,
			FarmID: farmID,
		},
	}); err != nil {
		t.Fatalf("farm plant: %v", err)
	}

	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "farm-harvest-too-early",
		Intent: survival.ActionIntent{
			Type:   survival.ActionFarmHarvest,
			FarmID: farmID,
		},
	}); !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed for early harvest, got %v", err)
	}

	now = now.Add(61 * time.Minute)
	if _, err := uc.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "farm-harvest-1",
		Intent: survival.ActionIntent{
			Type:   survival.ActionFarmHarvest,
			FarmID: farmID,
		},
	}); err != nil {
		t.Fatalf("farm harvest: %v", err)
	}

	var farm model.WorldObject
	if err := db.Where("owner_agent_id = ? AND object_id = ?", agentID, farmID).First(&farm).Error; err != nil {
		t.Fatalf("query farm object: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(farm.ObjectState), &state); err != nil {
		t.Fatalf("unmarshal farm object_state: %v", err)
	}
	if got, ok := state["state"].(string); !ok || got != "IDLE" {
		t.Fatalf("expected farm state IDLE after harvest, got %v", state["state"])
	}
}

func TestUseCase_E2E_GatherDepletesNodeThenRespawnsAtSamePosition(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-gather-node-respawn"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_resource_nodes WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	resourceRepo := gormrepo.NewAgentResourceNodeRepo(db)
	txManager := gormrepo.NewTxManager(db)

	if err := stateRepo.SaveWithVersion(ctx, survival.AgentStateAggregate{
		AgentID:   agentID,
		Vitals:    survival.Vitals{HP: 100, Hunger: 90, Energy: 90},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   1,
	}, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(1700030000, 0)
	worldProvider := worldmock.Provider{Snapshot: world.Snapshot{
		TimeOfDay:      "day",
		ThreatLevel:    1,
		NearbyResource: map[string]int{"wood": 1},
		VisibleTiles: []world.Tile{
			{X: 0, Y: 0, Passable: true, Resource: "wood"},
		},
	}}
	observeUC := observe.UseCase{
		StateRepo:    stateRepo,
		ResourceRepo: resourceRepo,
		World:        worldProvider,
		Now:          func() time.Time { return now },
	}
	actionUC := UseCase{
		TxManager:    txManager,
		StateRepo:    stateRepo,
		ActionRepo:   actionRepo,
		EventRepo:    eventRepo,
		ResourceRepo: resourceRepo,
		World:        worldProvider,
		Settle:       survival.SettlementService{},
		Now:          func() time.Time { return now },
	}

	before, err := observeUC.Execute(ctx, observe.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe before gather: %v", err)
	}
	if len(before.Resources) != 1 || before.Resources[0].ID != "res_0_0_wood" {
		t.Fatalf("expected wood visible before gather, got %+v", before.Resources)
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "gather-respawn-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	}); err != nil {
		t.Fatalf("gather: %v", err)
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "gather-respawn-2",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	}); !errors.Is(err, ErrResourceDepleted) {
		t.Fatalf("expected ErrResourceDepleted before respawn, got %v", err)
	}

	mid, err := observeUC.Execute(ctx, observe.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe after gather before respawn: %v", err)
	}
	if len(mid.Resources) != 0 {
		t.Fatalf("expected depleted node hidden before respawn, got %+v", mid.Resources)
	}
	for _, tile := range mid.Snapshot.VisibleTiles {
		if tile.X == 0 && tile.Y == 0 && tile.Resource != "" {
			t.Fatalf("expected snapshot tile resource cleared after gather, got=%q", tile.Resource)
		}
	}

	now = now.Add(61 * time.Minute)
	after, err := observeUC.Execute(ctx, observe.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe after respawn: %v", err)
	}
	if len(after.Resources) != 1 || after.Resources[0].ID != "res_0_0_wood" {
		t.Fatalf("expected same-position resource respawn, got %+v", after.Resources)
	}
}

func TestUseCase_E2E_GatherDepletionIsPerAgentMapState(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentA := "it-map-state-agent-a"
	agentB := "it-map-state-agent-b"
	for _, agentID := range []string{agentA, agentB} {
		_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_resource_nodes WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	resourceRepo := gormrepo.NewAgentResourceNodeRepo(db)
	txManager := gormrepo.NewTxManager(db)

	for _, agentID := range []string{agentA, agentB} {
		if err := stateRepo.SaveWithVersion(ctx, survival.AgentStateAggregate{
			AgentID:   agentID,
			Vitals:    survival.Vitals{HP: 100, Hunger: 90, Energy: 90},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		}, 0); err != nil {
			t.Fatalf("seed state %s: %v", agentID, err)
		}
	}

	now := time.Unix(1700040000, 0)
	worldProvider := worldmock.Provider{Snapshot: world.Snapshot{
		TimeOfDay:      "day",
		ThreatLevel:    1,
		NearbyResource: map[string]int{"wood": 1},
		VisibleTiles: []world.Tile{
			{X: 0, Y: 0, Passable: true, Resource: "wood"},
		},
	}}
	observeUC := observe.UseCase{
		StateRepo:    stateRepo,
		ResourceRepo: resourceRepo,
		World:        worldProvider,
		Now:          func() time.Time { return now },
	}
	actionUC := UseCase{
		TxManager:    txManager,
		StateRepo:    stateRepo,
		ActionRepo:   actionRepo,
		EventRepo:    eventRepo,
		ResourceRepo: resourceRepo,
		World:        worldProvider,
		Settle:       survival.SettlementService{},
		Now:          func() time.Time { return now },
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentA,
		IdempotencyKey: "agent-a-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	}); err != nil {
		t.Fatalf("agent A gather: %v", err)
	}

	obsA, err := observeUC.Execute(ctx, observe.Request{AgentID: agentA})
	if err != nil {
		t.Fatalf("observe A: %v", err)
	}
	if len(obsA.Resources) != 0 {
		t.Fatalf("expected depleted resource hidden for agent A, got %+v", obsA.Resources)
	}

	obsB, err := observeUC.Execute(ctx, observe.Request{AgentID: agentB})
	if err != nil {
		t.Fatalf("observe B: %v", err)
	}
	if len(obsB.Resources) != 1 || obsB.Resources[0].ID != "res_0_0_wood" {
		t.Fatalf("expected resource still visible for agent B, got %+v", obsB.Resources)
	}
}
