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

func TestUseCase_SleepUsesInstantFixedRecovery(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 50, Hunger: 70, Energy: 10},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{events: []survival.DomainEvent{
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-100 * time.Minute),
			Payload: map[string]any{
				"decision": map[string]any{"intent": "gather"},
			},
		},
	}}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"bed-1": {ObjectID: "bed-1", ObjectType: "bed", Quality: "ROUGH", X: 0, Y: 0},
	}}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 2000, TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-sleep-instant",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep, BedID: "bed-1"},
	})
	if err != nil {
		t.Fatalf("sleep execute error: %v", err)
	}
	if got, want := out.UpdatedState.Vitals.Energy, 34; got != want {
		t.Fatalf("expected fixed sleep energy=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.HP, 58; got != want {
		t.Fatalf("expected fixed sleep hp=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.Hunger, 70; got != want {
		t.Fatalf("expected instant sleep no hunger change=%d, got=%d", want, got)
	}
}

func TestUseCase_SleepRecoveryScalesByBedQuality(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 40, Hunger: 70, Energy: 10},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"bed-good": {ObjectID: "bed-good", ObjectType: "bed", Quality: "GOOD", X: 0, Y: 0},
	}}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 2000, TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-sleep-good-bed",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep, BedID: "bed-good"},
	})
	if err != nil {
		t.Fatalf("sleep execute error: %v", err)
	}
	if got, want := out.UpdatedState.Vitals.Energy, 46; got != want {
		t.Fatalf("expected good bed energy=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.HP, 52; got != want {
		t.Fatalf("expected good bed hp=%d, got=%d", want, got)
	}
}

func TestUseCase_RejectsSleepDuringCooldown(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 90, Hunger: 70, Energy: 40},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{events: []survival.DomainEvent{
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-30 * time.Second),
			Payload: map[string]any{
				"decision": map[string]any{"intent": "sleep"},
			},
		},
	}}
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"bed-1": {ObjectID: "bed-1", ObjectType: "bed", Quality: "ROUGH", X: 0, Y: 0},
	}}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		ObjectRepo: objectRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 2000, TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-sleep-cooldown",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep, BedID: "bed-1"},
	})
	if !errors.Is(err, ErrActionCooldownActive) {
		t.Fatalf("expected ErrActionCooldownActive, got %v", err)
	}
}

func TestUseCase_RejectsSleepWhenBedMissing(t *testing.T) {
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
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{}}

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
		IdempotencyKey: "k-sleep-missing-bed",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep, BedID: "missing-bed"}})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}
