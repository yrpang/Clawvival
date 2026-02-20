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

func TestUseCase_EatAllowsJamAndRecoversMoreThanDefault(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 40, Energy: 60}, Inventory: map[string]int{"jam": 1}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 0}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-eat-jam",
		Intent:         survival.ActionIntent{Type: survival.ActionEat, ItemType: "jam", Count: 1},
	})
	if err != nil {
		t.Fatalf("expected jam eat success, got err=%v", err)
	}
	if got := out.UpdatedState.Inventory["jam"]; got != 0 {
		t.Fatalf("expected jam consumed, got=%d", got)
	}
	if got := out.UpdatedState.Vitals.Hunger; got <= 40+survival.ActionEatDeltaHunger {
		t.Fatalf("expected jam hunger recovery > default(%d), got hunger=%d", survival.ActionEatDeltaHunger, got)
	}
	if got, want := out.UpdatedState.Vitals.Hunger, 100; got != want {
		t.Fatalf("expected jam hunger=%d, got=%d", want, got)
	}
}

func TestUseCase_CraftFurnaceRecipeRequiresFurnaceObject(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Inventory: map[string]int{"stone": 2},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 0}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-craft-brick-no-furnace",
		Intent:         survival.ActionIntent{Type: survival.ActionCraft, RecipeID: int(survival.RecipeBrick)},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed without furnace, got=%v", err)
	}
}

func TestUseCase_CraftFurnaceRecipeSucceedsWithFurnaceObject(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Inventory: map[string]int{"stone": 2},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"furnace-1": {ObjectID: "furnace-1", ObjectType: "furnace", Kind: int(survival.BuildFurnace), X: 0, Y: 0},
	}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 0}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-craft-brick-with-furnace",
		Intent:         survival.ActionIntent{Type: survival.ActionCraft, RecipeID: int(survival.RecipeBrick)},
	})
	if err != nil {
		t.Fatalf("expected craft brick success with furnace, got err=%v", err)
	}
	if got := out.UpdatedState.Inventory["brick"]; got != 1 {
		t.Fatalf("expected crafted brick=1, got=%d", got)
	}
	if got := out.UpdatedState.Inventory["stone"]; got != 0 {
		t.Fatalf("expected stone consumed, got=%d", got)
	}
}

func TestUseCase_CraftFurnaceRecipeUsesBuiltFurnaceFromBuildAction(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 80},
			Inventory: map[string]int{"stone": 8},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 0,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
				{X: 1, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	if _, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-build-furnace-for-craft",
		Intent: survival.ActionIntent{
			Type:       survival.ActionBuild,
			ObjectType: "furnace",
			Pos:        &survival.Position{X: 1, Y: 0},
		},
	}); err != nil {
		t.Fatalf("expected build furnace success, got err=%v", err)
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-craft-brick-after-build-furnace",
		Intent:         survival.ActionIntent{Type: survival.ActionCraft, RecipeID: int(survival.RecipeBrick)},
	})
	if err != nil {
		t.Fatalf("expected craft brick success after building furnace, got err=%v", err)
	}
	if got := out.UpdatedState.Inventory["brick"]; got != 1 {
		t.Fatalf("expected crafted brick=1, got=%d", got)
	}
}
