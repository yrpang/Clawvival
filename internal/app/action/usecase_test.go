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
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 10},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	req := Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}}

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
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, // external value should be ignored
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, // external value should be ignored
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

type stubTxManager struct{}

func (stubTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type stubStateRepo struct {
	byAgent map[string]survival.AgentStateAggregate
}

func (r *stubStateRepo) GetByAgentID(_ context.Context, agentID string) (survival.AgentStateAggregate, error) {
	state, ok := r.byAgent[agentID]
	if !ok {
		return survival.AgentStateAggregate{}, ports.ErrNotFound
	}
	return state, nil
}

func (r *stubStateRepo) SaveWithVersion(_ context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	current, ok := r.byAgent[state.AgentID]
	if !ok {
		if expectedVersion != 0 {
			return ports.ErrConflict
		}
		r.byAgent[state.AgentID] = state
		return nil
	}
	if current.Version != expectedVersion {
		return ports.ErrConflict
	}
	r.byAgent[state.AgentID] = state
	return nil
}

type stubActionRepo struct {
	byKey map[string]ports.ActionExecutionRecord
}

func (r *stubActionRepo) GetByIdempotencyKey(_ context.Context, agentID, key string) (*ports.ActionExecutionRecord, error) {
	record, ok := r.byKey[agentID+"|"+key]
	if !ok {
		return nil, ports.ErrNotFound
	}
	copy := record
	return &copy, nil
}

func (r *stubActionRepo) SaveExecution(_ context.Context, execution ports.ActionExecutionRecord) error {
	r.byKey[execution.AgentID+"|"+execution.IdempotencyKey] = execution
	return nil
}

type stubEventRepo struct {
	events []survival.DomainEvent
}

func (r *stubEventRepo) Append(_ context.Context, _ string, events []survival.DomainEvent) error {
	r.events = append(r.events, events...)
	return nil
}

func (r *stubEventRepo) ListByAgentID(_ context.Context, _ string, limit int) ([]survival.DomainEvent, error) {
	if limit <= 0 || limit > len(r.events) {
		limit = len(r.events)
	}
	out := make([]survival.DomainEvent, limit)
	copy(out, r.events[:limit])
	return out, nil
}

func TestUseCase_RejectsInvalidActionParams(t *testing.T) {
	cases := []Request{
		{AgentID: "agent-1", IdempotencyKey: "k0", Intent: survival.ActionIntent{Type: survival.ActionRest}},
		{AgentID: "agent-1", IdempotencyKey: "k1", Intent: survival.ActionIntent{Type: survival.ActionMove}},
		{AgentID: "agent-1", IdempotencyKey: "k3", Intent: survival.ActionIntent{Type: survival.ActionBuild}},
		{AgentID: "agent-1", IdempotencyKey: "k4", Intent: survival.ActionIntent{Type: survival.ActionFarm}},
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
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
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

	now = now.Add(10 * time.Minute)
	_, err = uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "gather-during-rest",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
	})
	if !errors.Is(err, ErrActionInProgress) {
		t.Fatalf("expected ErrActionInProgress, got %v", err)
	}

	now = now.Add(21 * time.Minute)
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "gather-after-rest",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
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
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 1},
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
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
	if rec := actionRepo.byKey["agent-1|rest-terminate"]; rec.DT != 10 {
		t.Fatalf("expected terminate execution dt=10, got=%d", rec.DT)
	}
	foundEnded := false
	for _, evt := range out.Events {
		if evt.Type == "ongoing_action_ended" {
			if got, ok := evt.Payload["actual_minutes"].(int); !ok || got != 10 {
				t.Fatalf("expected ongoing_action_ended actual_minutes=10, got=%v", evt.Payload["actual_minutes"])
			}
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
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

func TestUseCase_AcceptsValidExpandedAction(t *testing.T) {
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
		World:      worldmock.Provider{Snapshot: world.Snapshot{TimeOfDay: "day", ThreatLevel: 1}},
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-expanded",
		Intent:         survival.ActionIntent{Type: survival.ActionSleep}})
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-123",
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather}})
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

func TestUseCase_RejectsEatWhenInventoryInsufficient(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
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
			TimeOfDay:   "day",
			ThreatLevel: 1,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-eat-precheck",
		Intent: survival.ActionIntent{
			Type:     survival.ActionEat,
			ItemType: "berry",
			Count:    1,
		},
	})
	if err == nil {
		t.Fatalf("expected precondition error for eat without food")
	}
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}

func TestUseCase_RejectsMoveWhenTargetTileBlocked(t *testing.T) {
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
				{X: 1, Y: 0, Passable: false},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-blocked",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Direction: "E"}})
	if !errors.Is(err, ErrActionInvalidPosition) {
		t.Fatalf("expected ErrActionInvalidPosition, got %v", err)
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
}
