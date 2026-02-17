package replay

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawverse/internal/adapter/repo/gorm"
	worldmock "clawverse/internal/adapter/world/mock"
	"clawverse/internal/app/action"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_E2E_FiltersByOccurredTimeWindow(t *testing.T) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVERSE_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-replay-window"
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

	actionUC := action.UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	if _, err := actionUC.Execute(ctx, action.Request{
		AgentID:        agentID,
		IdempotencyKey: "replay-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
		StrategyHash:   "sha-old",
	}); err != nil {
		t.Fatalf("first action execute: %v", err)
	}

	actionUC.Now = func() time.Time { return time.Unix(1700003600, 0) }
	if _, err := actionUC.Execute(ctx, action.Request{
		AgentID:        agentID,
		IdempotencyKey: "replay-2",
		Intent:         survival.ActionIntent{Type: survival.ActionRest},
		DeltaMinutes:   30,
		StrategyHash:   "sha-new",
	}); err != nil {
		t.Fatalf("second action execute: %v", err)
	}

	replayUC := UseCase{Events: eventRepo}
	out, err := replayUC.Execute(ctx, Request{
		AgentID:      agentID,
		Limit:        50,
		OccurredFrom: 1700003000,
		OccurredTo:   1700004000,
	})
	if err != nil {
		t.Fatalf("replay execute: %v", err)
	}
	if got, want := len(out.Events), 1; got != want {
		t.Fatalf("filtered event count mismatch: got=%d want=%d", got, want)
	}
	if got, want := out.Events[0].Payload["strategy_hash"], "sha-new"; got != want {
		t.Fatalf("strategy hash mismatch: got=%v want=%v", got, want)
	}
}
