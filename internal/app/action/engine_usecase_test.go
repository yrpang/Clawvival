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

func TestUseCase_Idempotency(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID: "agent-1",
			Vitals:  survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
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
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 1234,
			TimeOfDay:        "day",
			ThreatLevel:      1,
			NearbyResource:   map[string]int{"wood": 10},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	req := Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}}

	first, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	second, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}

	if first.UpdatedState.Version != second.UpdatedState.Version {
		t.Fatalf("idempotency broken: version mismatch first=%d second=%d", first.UpdatedState.Version, second.UpdatedState.Version)
	}
	if got, want := first.UpdatedState.SessionID, "session-agent-1"; got != want {
		t.Fatalf("expected session_id=%q, got %q", want, got)
	}
	if got, want := second.UpdatedState.SessionID, "session-agent-1"; got != want {
		t.Fatalf("expected idempotent session_id=%q, got %q", want, got)
	}
	if first.WorldTimeBeforeSeconds != second.WorldTimeBeforeSeconds || first.WorldTimeAfterSeconds != second.WorldTimeAfterSeconds {
		t.Fatalf(
			"idempotency should preserve world time window: first=(%d,%d) second=(%d,%d)",
			first.WorldTimeBeforeSeconds,
			first.WorldTimeAfterSeconds,
			second.WorldTimeBeforeSeconds,
			second.WorldTimeAfterSeconds,
		)
	}
}

func TestUseCase_DeltaUsesSystemTimeDefaultOnFirstAction(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 1000,
			TimeOfDay:        "day",
			ThreatLevel:      1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-system-dt-default",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.SettledDTMinutes != 30 {
		t.Fatalf("expected settled dt=30, got %d", out.SettledDTMinutes)
	}
	if out.WorldTimeBeforeSeconds != 1000 {
		t.Fatalf("expected world_time_before=1000, got %d", out.WorldTimeBeforeSeconds)
	}
	if out.WorldTimeAfterSeconds != 2800 {
		t.Fatalf("expected world_time_after=2800, got %d", out.WorldTimeAfterSeconds)
	}
	got := actionRepo.byKey["agent-1|k-system-dt-default"]
	if got.DT != 30 {
		t.Fatalf("expected default system dt=30, got %d", got.DT)
	}
}

func TestUseCase_DeltaUsesElapsedSinceLastSettle(t *testing.T) {
	nowAt := time.Unix(1700000900, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{
		events: []survival.DomainEvent{
			{Type: "action_settled", OccurredAt: nowAt.Add(-45 * time.Minute)},
		},
	}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 2000,
			TimeOfDay:        "day",
			ThreatLevel:      1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return nowAt },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-system-dt-elapsed",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.SettledDTMinutes != 45 {
		t.Fatalf("expected settled dt=45, got %d", out.SettledDTMinutes)
	}
	if out.WorldTimeBeforeSeconds != 2000 {
		t.Fatalf("expected world_time_before=2000, got %d", out.WorldTimeBeforeSeconds)
	}
	if out.WorldTimeAfterSeconds != 4700 {
		t.Fatalf("expected world_time_after=4700, got %d", out.WorldTimeAfterSeconds)
	}
	got := actionRepo.byKey["agent-1|k-system-dt-elapsed"]
	if got.DT != 45 {
		t.Fatalf("expected elapsed system dt=45, got %d", got.DT)
	}
}

func TestResolveHeartbeatDeltaMinutes_ClampsBounds(t *testing.T) {
	nowAt := time.Unix(1700000000, 0)
	cases := []struct {
		name string
		last time.Time
		want int
	}{
		{name: "min clamp", last: nowAt, want: 1},
		{name: "max clamp", last: nowAt.Add(-500 * time.Minute), want: 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubEventRepo{events: []survival.DomainEvent{
				{Type: "action_settled", OccurredAt: tc.last},
			}}
			got, err := resolveHeartbeatDeltaMinutes(context.Background(), repo, "agent-1", nowAt)
			if err != nil {
				t.Fatalf("resolve delta error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected dt=%d, got %d", tc.want, got)
			}
		})
	}
}

func TestUseCase_RejectsMissingIntent(t *testing.T) {
	uc := UseCase{}
	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1"})
	if err == nil {
		t.Fatalf("expected error for missing intent")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUseCase_RejectsUnknownIntentType(t *testing.T) {
	uc := UseCase{}
	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1",
		Intent:         survival.ActionIntent{Type: survival.ActionType("unknown")}})
	if err == nil {
		t.Fatalf("expected error for unknown intent type")
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUseCase_MetricsRecordsSuccessOnExecuteSuccess(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	metrics := &stubActionMetrics{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		Metrics:    metrics,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 1000,
			TimeOfDay:        "day",
			ThreatLevel:      1,
			NearbyResource:   map[string]int{"wood": 1},
			VisibleTiles:     []world.Tile{{X: 0, Y: 0, Passable: true, Resource: "wood"}},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "metrics-success",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if metrics.successCalls != 1 || metrics.failureCalls != 0 || metrics.conflictCalls != 0 {
		t.Fatalf("unexpected metrics calls: success=%d failure=%d conflict=%d", metrics.successCalls, metrics.failureCalls, metrics.conflictCalls)
	}
}

func TestUseCase_MetricsRecordsConflictOnVersionConflict(t *testing.T) {
	stateRepo := &conflictOnSaveStateRepo{stubStateRepo: stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	metrics := &stubActionMetrics{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		Metrics:    metrics,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 1000,
			TimeOfDay:        "day",
			ThreatLevel:      1,
			NearbyResource:   map[string]int{"wood": 1},
			VisibleTiles:     []world.Tile{{X: 0, Y: 0, Passable: true, Resource: "wood"}},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "metrics-conflict",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expected ports.ErrConflict, got %v", err)
	}
	if metrics.successCalls != 0 || metrics.failureCalls != 0 || metrics.conflictCalls != 1 {
		t.Fatalf("unexpected metrics calls: success=%d failure=%d conflict=%d", metrics.successCalls, metrics.failureCalls, metrics.conflictCalls)
	}
}

func TestUseCase_MetricsRecordsFailureOnUnexpectedError(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &errorActionRepo{err: errors.New("db down")}
	eventRepo := &stubEventRepo{}
	metrics := &stubActionMetrics{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		Metrics:    metrics,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "metrics-failure",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if metrics.successCalls != 0 || metrics.failureCalls != 1 || metrics.conflictCalls != 0 {
		t.Fatalf("unexpected metrics calls: success=%d failure=%d conflict=%d", metrics.successCalls, metrics.failureCalls, metrics.conflictCalls)
	}
}

func TestUseCase_RejectsInvalidActionParams(t *testing.T) {
	cases := []Request{
		{AgentID: "agent-1", IdempotencyKey: "k0", Intent: survival.ActionIntent{Type: survival.ActionRest}},
		{AgentID: "agent-1", IdempotencyKey: "k1", Intent: survival.ActionIntent{Type: survival.ActionMove}},
		{AgentID: "agent-1", IdempotencyKey: "k3", Intent: survival.ActionIntent{Type: survival.ActionBuild}},
		{AgentID: "agent-1", IdempotencyKey: "k5", Intent: survival.ActionIntent{Type: survival.ActionCraft}},
		{AgentID: "agent-1", IdempotencyKey: "k6", Intent: survival.ActionIntent{Type: survival.ActionEat}},
	}

	uc := UseCase{}
	for _, req := range cases {
		_, err := uc.Execute(context.Background(), req)
		if err == nil {
			t.Fatalf("expected invalid request for intent=%s", req.Intent.Type)
		}
		if !errors.Is(err, ErrInvalidActionParams) {
			t.Fatalf("expected ErrInvalidActionParams for intent=%s, got %v", req.Intent.Type, err)
		}
	}
}

func TestUseCase_AcceptsValidExpandedAction(t *testing.T) {
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
	objectRepo := &stubObjectRepo{byID: map[string]ports.WorldObjectRecord{
		"bed-1": {ObjectID: "bed-1", ObjectType: "bed", X: 0, Y: 0},
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
		IdempotencyKey: "k-expanded",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep, BedID: "bed-1"}})
	if err != nil {
		t.Fatalf("expected valid expanded action, got %v", err)
	}
}

func TestUseCase_AppendsStrategyMetadataToEvents(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
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
		IdempotencyKey: "k-strategy",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}, StrategyHash: "sha-123",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if len(eventRepo.events) == 0 {
		t.Fatalf("expected events")
	}
	if eventRepo.events[0].Payload["strategy_hash"] != "sha-123" {
		t.Fatalf("expected strategy hash in payload")
	}
	if eventRepo.events[0].Payload["session_id"] != "session-agent-1" {
		t.Fatalf("expected session id in payload")
	}
}

func TestUseCase_AppendsPhaseChangedEvent(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:    "night",
			ThreatLevel:  3,
			PhaseChanged: true,
			PhaseFrom:    "day",
			PhaseTo:      "night",
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-phase-switch",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	found := false
	for _, evt := range eventRepo.events {
		if evt.Type != "world_phase_changed" {
			continue
		}
		found = true
		if evt.Payload["from"] != "day" || evt.Payload["to"] != "night" {
			t.Fatalf("unexpected phase payload: %+v", evt.Payload)
		}
	}
	if !found {
		t.Fatalf("expected world_phase_changed event")
	}
}

func TestUseCase_RejectsActionDuringCooldown(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{events: []survival.DomainEvent{
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-30 * time.Second),
			Payload: map[string]any{
				"decision": map[string]any{"intent": "move"},
			},
		},
	}}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "night",
			ThreatLevel: 3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-cooldown",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Direction: "E"}})
	if !errors.Is(err, ErrActionCooldownActive) {
		t.Fatalf("expected ErrActionCooldownActive, got %v", err)
	}
	var cooldownErr *ActionCooldownActiveError
	if !errors.As(err, &cooldownErr) || cooldownErr == nil || cooldownErr.RemainingSeconds <= 0 {
		t.Fatalf("expected ActionCooldownActiveError with remaining seconds, got %v", err)
	}
}

func TestUseCase_72hGate_MinimumSettlingPath(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID: "agent-1",
			Vitals:  survival.Vitals{HP: 100, Hunger: 90, Energy: 90},
			Position: survival.Position{
				X: 0,
				Y: 0,
			},
			Home:      survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{"wood": 20, "stone": 3, "seed": 2},
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
			WorldTimeSeconds: 100,
			TimeOfDay:        "day",
			ThreatLevel:      1,
		}},
		Settle: survival.SettlementService{},
		Now: func() time.Time {
			now = now.Add(6 * time.Minute)
			return now
		},
	}

	actions := []Request{
		{
			AgentID:        "agent-1",
			IdempotencyKey: "gate-build-bed",
			Intent:         survival.ActionIntent{Type: survival.ActionBuild, ObjectType: "bed_rough", Pos: &survival.Position{X: 0, Y: 1}},
		},
		{
			AgentID:        "agent-1",
			IdempotencyKey: "gate-build-box",
			Intent:         survival.ActionIntent{Type: survival.ActionBuild, ObjectType: "box", Pos: &survival.Position{X: 1, Y: 0}},
		},
		{
			AgentID:        "agent-1",
			IdempotencyKey: "gate-build-farm",
			Intent:         survival.ActionIntent{Type: survival.ActionBuild, ObjectType: "farm_plot", Pos: &survival.Position{X: 1, Y: 1}},
		},
		{
			AgentID:        "agent-1",
			IdempotencyKey: "gate-farm-plant",
			Intent:         survival.ActionIntent{Type: survival.ActionFarmPlant, FarmID: "obj-agent-1-gate-build-farm"},
		},
	}

	var last Response
	var err error
	for _, req := range actions {
		last, err = uc.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("action %s failed: %v", req.IdempotencyKey, err)
		}
		if last.ResultCode != survival.ResultOK {
			t.Fatalf("expected OK for %s, got %s", req.IdempotencyKey, last.ResultCode)
		}
		if got, want := last.UpdatedState.SessionID, "session-agent-1"; got != want {
			t.Fatalf("expected same session_id=%q during gate actions, got %q", want, got)
		}
	}

	if got := last.UpdatedState.Inventory["seed"]; got >= 2 {
		t.Fatalf("expected seed consumed by farm_plant, got=%d", got)
	}
}

func TestUseCase_ActionResponseIncludesDerivedStatusEffects(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:    "agent-1",
			Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 10},
			Position:   survival.Position{X: 0, Y: 0},
			Inventory:  map[string]int{},
			Version:    1,
			DeathCause: survival.DeathCauseUnknown,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "night", ThreatLevel: 0}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-action-stateview",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if len(out.UpdatedState.StatusEffects) == 0 {
		t.Fatalf("expected derived status_effects in action response")
	}
}

func TestUseCase_ResponseIncludesSettlementReasonSummary(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: -20, Energy: -10},
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
			WorldTimeSeconds: 100,
			TimeOfDay:        "day",
			ThreatLevel:      0,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700005000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-settlement-summary",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.Settlement == nil {
		t.Fatalf("expected settlement summary in action response")
	}
	reasons, ok := out.Settlement["vitals_change_reasons"].(map[string]any)
	if !ok {
		t.Fatalf("expected vitals_change_reasons map in settlement summary, got=%T", out.Settlement["vitals_change_reasons"])
	}
	hpReasons, ok := reasons["hp"].([]map[string]any)
	if ok && len(hpReasons) == 0 {
		t.Fatalf("expected hp reasons to explain hp changes")
	}
}
