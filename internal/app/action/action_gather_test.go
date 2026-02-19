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

func TestUseCase_GatherRejectsTargetOutOfView(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-oov",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_20_20_wood"},
	})
	if !errors.Is(err, ErrTargetOutOfView) {
		t.Fatalf("expected ErrTargetOutOfView, got %v", err)
	}
}

func TestUseCase_GatherRejectsTargetOutsideSnapshotViewRadius(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
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
			ViewRadius:  2,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
				{X: 1, Y: 0, Passable: true, Resource: "wood"},
				{X: 2, Y: 0, Passable: true, Resource: "wood"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-oov-radius",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_3_0_wood"},
	})
	if !errors.Is(err, ErrTargetOutOfView) {
		t.Fatalf("expected ErrTargetOutOfView, got %v", err)
	}
}

func TestUseCase_GatherRejectsTargetNotVisible(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
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
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-hidden",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_1_0_wood"},
	})
	if !errors.Is(err, ErrTargetNotVisible) {
		t.Fatalf("expected ErrTargetNotVisible, got %v", err)
	}
}

func TestUseCase_GatherRejectsNightTargetOutsideVisionRadius(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "night",
			ThreatLevel: 2,
			VisibleTiles: []world.Tile{
				{X: 4, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-night-hidden",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_4_0_wood"},
	})
	if !errors.Is(err, ErrTargetNotVisible) {
		t.Fatalf("expected ErrTargetNotVisible, got %v", err)
	}
}

func TestUseCase_GatherRejectsWhenTargetResourceTypeMismatchesTile(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
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
			NearbyResource: map[string]int{
				"stone": 1,
			},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-mismatch-type",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_stone"},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_GatherRejectsWhenTargetTileHasNoResource(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: ""},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-no-resource-tile",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_GatherRejectsWhenTargetAlreadyDepletedForAgent(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	resourceRepo := &stubResourceNodeRepo{byTarget: map[string]ports.AgentResourceNodeRecord{
		"agent-1|res_0_0_wood": {
			AgentID:       "agent-1",
			TargetID:      "res_0_0_wood",
			ResourceType:  "wood",
			X:             0,
			Y:             0,
			DepletedUntil: now.Add(30 * time.Minute),
		},
	}}
	uc := UseCase{
		TxManager:    stubTxManager{},
		StateRepo:    stateRepo,
		ActionRepo:   actionRepo,
		EventRepo:    eventRepo,
		ResourceRepo: resourceRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-depleted",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if !errors.Is(err, ErrResourceDepleted) {
		t.Fatalf("expected ErrResourceDepleted, got %v", err)
	}
	var depletedErr *ResourceDepletedError
	if !errors.As(err, &depletedErr) || depletedErr == nil || depletedErr.RemainingSeconds <= 0 {
		t.Fatalf("expected remaining_seconds in ResourceDepletedError, got=%v", err)
	}
}

func TestUseCase_GatherOnlyCollectsTargetResourceType(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:        "day",
			WorldTimeSeconds: 100,
			ThreatLevel:      0,
			NearbyResource:   map[string]int{"wood": 7, "stone": 9},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Zone: world.ZoneForest, Passable: true, Resource: "wood"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-target-only",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if got := out.UpdatedState.Inventory["wood"]; got != 1 {
		t.Fatalf("expected exactly one target resource per gather, got=%d", got)
	}
	if got := out.UpdatedState.Inventory["stone"]; got != 0 {
		t.Fatalf("expected non-target stone not gathered, got=%d", got)
	}
	if got, want := out.UpdatedState.CurrentZone, string(world.ZoneForest); got != want {
		t.Fatalf("expected current_zone=%q, got %q", want, got)
	}
}

func TestUseCase_GatherPersistsAgentResourceDepletion(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	resourceRepo := &stubResourceNodeRepo{}
	uc := UseCase{
		TxManager:    stubTxManager{},
		StateRepo:    stateRepo,
		ActionRepo:   actionRepo,
		EventRepo:    eventRepo,
		ResourceRepo: resourceRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:        "day",
			WorldTimeSeconds: 100,
			ThreatLevel:      0,
			NearbyResource:   map[string]int{"wood": 7},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}
	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-persists-depleted",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	record, err := resourceRepo.GetByTargetID(context.Background(), "agent-1", "res_0_0_wood")
	if err != nil {
		t.Fatalf("expected depletion record, got err=%v", err)
	}
	if !record.DepletedUntil.After(now) {
		t.Fatalf("expected depletion future timestamp, got=%v", record.DepletedUntil)
	}
}

func TestUseCase_GatherTriggersSeedPityAfterConsecutiveFails(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	for i := 0; i < 7; i++ {
		eventRepo.events = append(eventRepo.events, survival.DomainEvent{
			Type:       "action_settled",
			OccurredAt: time.Unix(1700000000+int64(i*60), 0),
			Payload: map[string]any{
				"decision": map[string]any{"intent": "gather"},
				"result":   map[string]any{"seed_gained": false},
			},
		})
	}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 100,
			TimeOfDay:        "day",
			ThreatLevel:      1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700003600, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gather-seed-pity",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if got := out.UpdatedState.Inventory["seed"]; got != 1 {
		t.Fatalf("expected pity seed +1, got=%d", got)
	}
	foundSettled := false
	for _, evt := range out.Events {
		if evt.Type != "action_settled" {
			continue
		}
		foundSettled = true
		result, ok := evt.Payload["result"].(map[string]any)
		if !ok {
			t.Fatalf("expected result payload map")
		}
		if got, ok := result["seed_pity_triggered"].(bool); !ok || !got {
			t.Fatalf("expected seed_pity_triggered=true, got=%v", result["seed_pity_triggered"])
		}
	}
	if !foundSettled {
		t.Fatalf("expected action_settled event")
	}
}
