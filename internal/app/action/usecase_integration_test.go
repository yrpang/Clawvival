package action

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	"clawverse/internal/adapter/repo/gorm/model"
	worldmock "clawverse/internal/adapter/world/mock"
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
