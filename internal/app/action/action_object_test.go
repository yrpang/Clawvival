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

func TestUseCase_RejectsBuildWhenInventoryInsufficient(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Inventory: map[string]int{}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
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

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-build-precheck",
		Intent: survival.ActionIntent{
			Type:       survival.ActionBuild,
			ObjectType: "bed_rough",
			Pos:        &survival.Position{X: 0, Y: 0},
		}})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_ContainerWithdrawRejectsWhenInventoryCapacityExceeded(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:           "agent-1",
			Vitals:            survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:          survival.Position{X: 0, Y: 0},
			Inventory:         map[string]int{"wood": 1},
			InventoryCapacity: 1,
			Version:           1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"box-1": {
			ObjectID:      "box-1",
			ObjectType:    "box",
			X:             0,
			Y:             0,
			CapacitySlots: 60,
			UsedSlots:     1,
			ObjectState:   `{"inventory":{"berry":1}}`,
		},
	}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-withdraw-over-cap",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerWithdraw,
			ContainerID: "box-1",
			Items:       []survival.ItemAmount{{ItemType: "berry", Count: 1}},
		},
	})
	if !errors.Is(err, ErrInventoryFull) {
		t.Fatalf("expected ErrInventoryFull, got %v", err)
	}
}

func TestUseCase_ContainerDepositRejectsDuplicateItemsOverInventory(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:           "agent-1",
			Vitals:            survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:          survival.Position{X: 0, Y: 0},
			Inventory:         map[string]int{"wood": 1},
			InventoryCapacity: 30,
			Version:           1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"box-1": {
			ObjectID:      "box-1",
			ObjectType:    "box",
			X:             0,
			Y:             0,
			CapacitySlots: 60,
			UsedSlots:     0,
			ObjectState:   `{"inventory":{}}`,
		},
	}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-deposit-dup-over",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerDeposit,
			ContainerID: "box-1",
			Items: []survival.ItemAmount{
				{ItemType: "wood", Count: 1},
				{ItemType: "wood", Count: 1},
			},
		},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_ContainerDepositRejectsWhenContainerFull(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:           "agent-1",
			Vitals:            survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:          survival.Position{X: 0, Y: 0},
			Inventory:         map[string]int{"wood": 1},
			InventoryCapacity: 30,
			Version:           1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"box-1": {
			ObjectID:      "box-1",
			ObjectType:    "box",
			X:             0,
			Y:             0,
			CapacitySlots: 1,
			UsedSlots:     1,
			ObjectState:   `{"inventory":{"berry":1}}`,
		},
	}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}
	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-deposit-full",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerDeposit,
			ContainerID: "box-1",
			Items:       []survival.ItemAmount{{ItemType: "wood", Count: 1}},
		},
	})
	if !errors.Is(err, ErrContainerFull) {
		t.Fatalf("expected ErrContainerFull, got %v", err)
	}
}

func TestUseCase_ContainerWithdrawRejectsDuplicateItemsOverBoxInventory(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:           "agent-1",
			Vitals:            survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:          survival.Position{X: 0, Y: 0},
			Inventory:         map[string]int{},
			InventoryCapacity: 30,
			Version:           1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"box-1": {
			ObjectID:      "box-1",
			ObjectType:    "box",
			X:             0,
			Y:             0,
			CapacitySlots: 60,
			UsedSlots:     1,
			ObjectState:   `{"inventory":{"berry":1}}`,
		},
	}}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-withdraw-dup-over",
		Intent: survival.ActionIntent{
			Type:        survival.ActionContainerWithdraw,
			ContainerID: "box-1",
			Items: []survival.ItemAmount{
				{ItemType: "berry", Count: 1},
				{ItemType: "berry", Count: 1},
			},
		},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_BuildActionSettledIncludesBuiltObjectID(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{"wood": 8},
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
			TimeOfDay:        "day",
			ThreatLevel:      0,
			WorldTimeSeconds: 100,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-build-result-id",
		Intent: survival.ActionIntent{
			Type:       survival.ActionBuild,
			ObjectType: "bed_rough",
			Pos:        &survival.Position{X: 0, Y: 0},
		},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	found := false
	for _, evt := range out.Events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		result, _ := evt.Payload["result"].(map[string]any)
		if result == nil {
			continue
		}
		ids, _ := result["built_object_ids"].([]string)
		if len(ids) == 0 {
			continue
		}
		found = true
		if ids[0] != "obj-agent-1-k-build-result-id" {
			t.Fatalf("unexpected built object id: %v", ids)
		}
	}
	if !found {
		t.Fatalf("expected built_object_ids in action_settled.result")
	}
}
