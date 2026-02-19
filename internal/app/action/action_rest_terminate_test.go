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

func TestUseCase_RestStartPersistsExecutionStateAndEvent(t *testing.T) {
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

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 3600,
			TimeOfDay:        "day",
			ThreatLevel:      1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "rest-persist-check",
		Intent: survival.ActionIntent{
			Type:        survival.ActionRest,
			RestMinutes: 30,
		},
	})
	if err != nil {
		t.Fatalf("start rest: %v", err)
	}
	if out.UpdatedState.OngoingAction == nil {
		t.Fatalf("expected ongoing action in response")
	}
	if _, ok := actionRepo.byKey["agent-1|rest-persist-check"]; !ok {
		t.Fatalf("expected action execution persisted for rest start")
	}
	savedState, ok := stateRepo.byAgent["agent-1"]
	if !ok || savedState.OngoingAction == nil {
		t.Fatalf("expected state persisted with ongoing rest")
	}
	if len(eventRepo.events) == 0 || eventRepo.events[0].Type != "rest_started" {
		t.Fatalf("expected rest_started event persisted")
	}
}

func TestUseCase_RestBlocksOtherActionsUntilDue(t *testing.T) {
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

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 7200,
			TimeOfDay:        "day",
			ThreatLevel:      1,
			NearbyResource:   map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
				{X: 1, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	restOut, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "rest-1",
		Intent: survival.ActionIntent{
			Type:        survival.ActionRest,
			RestMinutes: 30,
		},
	})
	if err != nil {
		t.Fatalf("start rest: %v", err)
	}
	if restOut.UpdatedState.OngoingAction == nil {
		t.Fatalf("expected ongoing rest after start")
	}
	if got, want := restOut.WorldTimeBeforeSeconds, int64(7200); got != want {
		t.Fatalf("expected world_time_before_seconds=%d, got=%d", want, got)
	}
	if got, want := restOut.WorldTimeAfterSeconds, int64(7200); got != want {
		t.Fatalf("expected world_time_after_seconds=%d, got=%d", want, got)
	}

	restReplay, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "rest-1",
		Intent: survival.ActionIntent{
			Type:        survival.ActionRest,
			RestMinutes: 30,
		},
	})
	if err != nil {
		t.Fatalf("replay rest: %v", err)
	}
	if got, want := restReplay.WorldTimeBeforeSeconds, int64(7200); got != want {
		t.Fatalf("expected replay world_time_before_seconds=%d, got=%d", want, got)
	}
	if got, want := restReplay.WorldTimeAfterSeconds, int64(7200); got != want {
		t.Fatalf("expected replay world_time_after_seconds=%d, got=%d", want, got)
	}

	now = now.Add(10 * time.Minute)
	_, err = uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "gather-during-rest",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if !errors.Is(err, ErrActionInProgress) {
		t.Fatalf("expected ErrActionInProgress, got %v", err)
	}

	now = now.Add(21 * time.Minute)
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "gather-after-rest",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("gather after rest: %v", err)
	}
	if out.UpdatedState.OngoingAction != nil {
		t.Fatalf("expected ongoing action cleared after due")
	}
}

func TestUseCase_TerminateCanStopRestEarly(t *testing.T) {
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

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 3600,
			TimeOfDay:        "day",
			ThreatLevel:      1,
			NearbyResource:   map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, Resource: "wood"},
				{X: 1, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "rest-start",
		Intent: survival.ActionIntent{
			Type:        survival.ActionRest,
			RestMinutes: 30,
		},
	})
	if err != nil {
		t.Fatalf("start rest: %v", err)
	}

	now = now.Add(10 * time.Minute)
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "rest-terminate",
		Intent:         survival.ActionIntent{Type: survival.ActionTerminate},
	})
	if err != nil {
		t.Fatalf("terminate rest: %v", err)
	}
	if out.UpdatedState.OngoingAction != nil {
		t.Fatalf("expected ongoing action cleared by terminate")
	}
	if got, want := out.UpdatedState.Vitals.Energy, 63; got != want {
		t.Fatalf("expected proportional rest settlement energy=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.Hunger, 79; got != want {
		t.Fatalf("expected proportional rest settlement hunger=%d, got=%d", want, got)
	}
	_ = actionRepo.byKey["agent-1|rest-terminate"]
	if got, want := out.WorldTimeBeforeSeconds, int64(3600); got != want {
		t.Fatalf("expected world_time_before_seconds=%d, got=%d", want, got)
	}
	if got, want := out.WorldTimeAfterSeconds, int64(4200); got != want {
		t.Fatalf("expected world_time_after_seconds=%d, got=%d", want, got)
	}
	foundEnded := false
	for _, evt := range out.Events {
		if evt.Type == "ongoing_action_ended" {
			foundEnded = true
			break
		}
	}
	if !foundEnded {
		t.Fatalf("expected ongoing_action_ended event")
	}

	now = now.Add(1 * time.Minute)
	_, err = uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "gather-after-terminate",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("gather after terminate should succeed, got: %v", err)
	}
}

func TestUseCase_TerminateWithoutOngoingReturnsPreconditionFailed(t *testing.T) {
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
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "terminate-no-ongoing",
		Intent:         survival.ActionIntent{Type: survival.ActionTerminate},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_TerminateRejectsNonInterruptibleOngoingAction(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			OngoingAction: &survival.OngoingActionInfo{
				Type:    survival.ActionGather,
				Minutes: 30,
				EndAt:   now.Add(20 * time.Minute),
			},
			Version: 1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 100, TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "terminate-non-interruptible",
		Intent:         survival.ActionIntent{Type: survival.ActionTerminate},
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}
