package action

import (
	"context"
	"errors"
	"testing"
	"time"

	worldmock "clawvival/internal/adapter/world/mock"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_RejectsEatWhenInventoryInsufficient(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Inventory: map[string]int{}, Version: 1}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{TxManager: stubTxManager{}, StateRepo: stateRepo, ActionRepo: actionRepo, EventRepo: eventRepo, World: worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}}, Settle: survival.SettlementService{}, Now: func() time.Time {
		return time.Unix(1700000000, 0)
	}}
	_, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", IdempotencyKey: "k-eat-precheck", Intent: survival.ActionIntent{Type: survival.ActionEat, ItemType: "berry", Count: 1}})
	if err == nil {
		t.Fatalf("expected precondition error for eat without food")
	}
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}
func TestUseCase_EatAllowsWheat(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 40, Energy: 60}, Inventory: map[string]int{"wheat": 1}, Version: 1}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{TxManager: stubTxManager{}, StateRepo: stateRepo, ActionRepo: actionRepo, EventRepo: eventRepo, World: worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 0}}, Settle: survival.SettlementService{}, Now: func() time.Time {
		return time.Unix(1700000000, 0)
	}}
	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", IdempotencyKey: "k-eat-wheat", Intent: survival.ActionIntent{Type: survival.ActionEat, ItemType: "wheat", Count: 1}})
	if err != nil {
		t.Fatalf("expected wheat eat success, got err=%v", err)
	}
	if out.UpdatedState.Inventory["wheat"] != 0 {
		t.Fatalf("expected wheat consumed, got=%d", out.UpdatedState.Inventory["wheat"])
	}
}
func TestUseCase_EatRespectsCount(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 10, Energy: 60}, Inventory: map[string]int{"berry": 2}, Version: 1}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{TxManager: stubTxManager{}, StateRepo: stateRepo, ActionRepo: actionRepo, EventRepo: eventRepo, World: worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 0}}, Settle: survival.SettlementService{}, Now: func() time.Time {
		return time.Unix(1700000000, 0)
	}}
	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", IdempotencyKey: "k-eat-count-2", Intent: survival.ActionIntent{Type: survival.ActionEat, ItemType: "berry", Count: 2}})
	if err != nil {
		t.Fatalf("expected eat success, got err=%v", err)
	}
	if got := out.UpdatedState.Inventory["berry"]; got != 0 {
		t.Fatalf("expected 2 berries consumed, got=%d", got)
	}
}
