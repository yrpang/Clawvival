package replay

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawvival/internal/adapter/repo/gorm"
	worldmock "clawvival/internal/adapter/world/mock"
	"clawvival/internal/app/action"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_E2E_FiltersByOccurredTimeWindow(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-old",
	}); err != nil {
		t.Fatalf("first action execute: %v", err)
	}

	actionUC.Now = func() time.Time { return time.Unix(1700003600, 0) }
	if _, err := actionUC.Execute(ctx, action.Request{
		AgentID:        agentID,
		IdempotencyKey: "replay-2",
		Intent:         survival.ActionIntent{Type: survival.ActionRest}, StrategyHash: "sha-new",
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
	if got, want := out.Events[0].Payload["session_id"], "session-"+agentID; got != want {
		t.Fatalf("session id mismatch: got=%v want=%v", got, want)
	}

	none, err := replayUC.Execute(ctx, Request{
		AgentID:   agentID,
		Limit:     50,
		SessionID: "session-other",
	})
	if err != nil {
		t.Fatalf("replay with other session: %v", err)
	}
	if len(none.Events) != 0 {
		t.Fatalf("expected empty events for unmatched session, got=%d", len(none.Events))
	}

	bySession, err := replayUC.Execute(ctx, Request{
		AgentID:   agentID,
		Limit:     50,
		SessionID: "session-" + agentID,
	})
	if err != nil {
		t.Fatalf("replay with target session: %v", err)
	}
	if len(bySession.Events) == 0 {
		t.Fatalf("expected events for matched session")
	}
}

func TestUseCase_E2E_AppliesFiltersBeforeLimit(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-replay-filter-limit"
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
		Now:    func() time.Time { return time.Unix(1700010000, 0) },
	}
	if _, err := actionUC.Execute(ctx, action.Request{
		AgentID:        agentID,
		IdempotencyKey: "limit-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}}); err != nil {
		t.Fatalf("first action execute: %v", err)
	}
	actionUC.Now = func() time.Time { return time.Unix(1700013600, 0) }
	if _, err := actionUC.Execute(ctx, action.Request{
		AgentID:        agentID,
		IdempotencyKey: "limit-2",
		Intent:         survival.ActionIntent{Type: survival.ActionRest}}); err != nil {
		t.Fatalf("second action execute: %v", err)
	}

	replayUC := UseCase{Events: eventRepo}
	out, err := replayUC.Execute(ctx, Request{
		AgentID:      agentID,
		Limit:        1,
		OccurredFrom: 1700009900,
		OccurredTo:   1700010500,
	})
	if err != nil {
		t.Fatalf("replay execute: %v", err)
	}
	if got, want := len(out.Events), 1; got != want {
		t.Fatalf("expected one filtered event, got=%d", got)
	}
	if got, want := out.Events[0].OccurredAt.Unix(), int64(1700010000); got != want {
		t.Fatalf("expected older filtered event timestamp=%d, got=%d", want, got)
	}
}
