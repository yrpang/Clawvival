package action

import (
	"context"
	"testing"
	"time"

	worldmock "clawvival/internal/adapter/world/mock"
	"clawvival/internal/app/observe"
	"clawvival/internal/app/ports"
	"clawvival/internal/app/status"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestP0_MainLoop_ObserveActionStatus(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID: "agent-1",
			Vitals:  survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Version: 1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	worldProvider := worldmock.Provider{Snapshot: world.Snapshot{
		TimeOfDay:      "day",
		ThreatLevel:    1,
		NearbyResource: map[string]int{"wood": 10},
	}}

	observeUC := observe.UseCase{StateRepo: stateRepo, World: worldProvider}
	actionUC := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}
	statusUC := status.UseCase{StateRepo: stateRepo, World: worldProvider}

	obs, err := observeUC.Execute(context.Background(), observe.Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("observe error: %v", err)
	}
	if obs.Snapshot.TimeOfDay != "day" {
		t.Fatalf("expected day snapshot, got %q", obs.Snapshot.TimeOfDay)
	}

	act, err := actionUC.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "loop-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}})
	if err != nil {
		t.Fatalf("action error: %v", err)
	}
	if act.UpdatedState.Version != 2 {
		t.Fatalf("expected version 2 after action, got %d", act.UpdatedState.Version)
	}

	st, err := statusUC.Execute(context.Background(), status.Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if st.State.Version != act.UpdatedState.Version {
		t.Fatalf("status state mismatch: status=%d action=%d", st.State.Version, act.UpdatedState.Version)
	}
}
