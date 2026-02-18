package action

import (
	"context"
	"errors"
	"strings"
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}, // external value should be ignored
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_0_0_wood"}, // external value should be ignored
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

type stubObjectRepo struct {
	byID map[string]ports.WorldObjectRecord
}

type stubResourceNodeRepo struct {
	byTarget map[string]ports.AgentResourceNodeRecord
}

func (r *stubResourceNodeRepo) Upsert(_ context.Context, record ports.AgentResourceNodeRecord) error {
	if r.byTarget == nil {
		r.byTarget = map[string]ports.AgentResourceNodeRecord{}
	}
	r.byTarget[record.AgentID+"|"+record.TargetID] = record
	return nil
}

func (r *stubResourceNodeRepo) GetByTargetID(_ context.Context, agentID, targetID string) (ports.AgentResourceNodeRecord, error) {
	if r.byTarget == nil {
		return ports.AgentResourceNodeRecord{}, ports.ErrNotFound
	}
	record, ok := r.byTarget[agentID+"|"+targetID]
	if !ok {
		return ports.AgentResourceNodeRecord{}, ports.ErrNotFound
	}
	return record, nil
}

func (r *stubResourceNodeRepo) ListByAgentID(_ context.Context, agentID string) ([]ports.AgentResourceNodeRecord, error) {
	if r.byTarget == nil {
		return nil, ports.ErrNotFound
	}
	out := make([]ports.AgentResourceNodeRecord, 0, len(r.byTarget))
	for key, record := range r.byTarget {
		if strings.HasPrefix(key, agentID+"|") {
			out = append(out, record)
		}
	}
	if len(out) == 0 {
		return nil, ports.ErrNotFound
	}
	return out, nil
}

func (r *stubObjectRepo) Save(_ context.Context, _ string, obj ports.WorldObjectRecord) error {
	if r.byID == nil {
		r.byID = map[string]ports.WorldObjectRecord{}
	}
	r.byID[obj.ObjectID] = obj
	return nil
}

func (r *stubObjectRepo) GetByObjectID(_ context.Context, _ string, objectID string) (ports.WorldObjectRecord, error) {
	obj, ok := r.byID[objectID]
	if !ok {
		return ports.WorldObjectRecord{}, ports.ErrNotFound
	}
	return obj, nil
}

func (r *stubObjectRepo) ListByAgentID(_ context.Context, _ string) ([]ports.WorldObjectRecord, error) {
	out := make([]ports.WorldObjectRecord, 0, len(r.byID))
	for _, obj := range r.byID {
		out = append(out, obj)
	}
	return out, nil
}

func (r *stubObjectRepo) Update(_ context.Context, _ string, obj ports.WorldObjectRecord) error {
	if r.byID == nil {
		r.byID = map[string]ports.WorldObjectRecord{}
	}
	r.byID[obj.ObjectID] = obj
	return nil
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
	if rec := actionRepo.byKey["agent-1|rest-terminate"]; rec.DT != 10 {
		t.Fatalf("expected terminate execution dt=10, got=%d", rec.DT)
	}
	if got, want := out.WorldTimeBeforeSeconds, int64(3600); got != want {
		t.Fatalf("expected world_time_before_seconds=%d, got=%d", want, got)
	}
	if got, want := out.WorldTimeAfterSeconds, int64(4200); got != want {
		t.Fatalf("expected world_time_after_seconds=%d, got=%d", want, got)
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

func TestUseCase_EatAllowsWheat(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID: "agent-1",
			Vitals: survival.Vitals{
				HP:     100,
				Hunger: 40,
				Energy: 60,
			},
			Inventory: map[string]int{"wheat": 1},
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
			ThreatLevel: 0,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-eat-wheat",
		Intent: survival.ActionIntent{
			Type:     survival.ActionEat,
			ItemType: "wheat",
			Count:    1,
		},
	})
	if err != nil {
		t.Fatalf("expected wheat eat success, got err=%v", err)
	}
	if out.UpdatedState.Inventory["wheat"] != 0 {
		t.Fatalf("expected wheat consumed, got=%d", out.UpdatedState.Inventory["wheat"])
	}
}

func TestUseCase_EatRespectsCount(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID: "agent-1",
			Vitals: survival.Vitals{
				HP:     100,
				Hunger: 10,
				Energy: 60,
			},
			Inventory: map[string]int{"berry": 2},
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
			ThreatLevel: 0,
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-eat-count-2",
		Intent: survival.ActionIntent{
			Type:     survival.ActionEat,
			ItemType: "berry",
			Count:    2,
		},
	})
	if err != nil {
		t.Fatalf("expected eat success, got err=%v", err)
	}
	if got := out.UpdatedState.Inventory["berry"]; got != 0 {
		t.Fatalf("expected 2 berries consumed, got=%d", got)
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
	var posErr *ActionInvalidPositionError
	if !errors.As(err, &posErr) || posErr == nil {
		t.Fatalf("expected ActionInvalidPositionError details, got %T", err)
	}
	if posErr.TargetPos == nil || posErr.TargetPos.X != 1 || posErr.TargetPos.Y != 0 {
		t.Fatalf("unexpected target_pos details: %+v", posErr.TargetPos)
	}
	if posErr.BlockingTilePos == nil || posErr.BlockingTilePos.X != 1 || posErr.BlockingTilePos.Y != 0 {
		t.Fatalf("unexpected blocking_tile_pos details: %+v", posErr.BlockingTilePos)
	}
}

func TestUseCase_MoveToPosition_SucceedsWithMultiStepPath(t *testing.T) {
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
				{X: 1, Y: 0, Passable: true},
				{X: 2, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-pos-ok",
		Intent: survival.ActionIntent{
			Type: survival.ActionMove,
			Pos:  &survival.Position{X: 2, Y: 0},
		},
	})
	if err != nil {
		t.Fatalf("expected move-to-position success, got %v", err)
	}
	if got, want := out.UpdatedState.Position.X, 2; got != want {
		t.Fatalf("expected moved to x=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Position.Y, 0; got != want {
		t.Fatalf("expected moved to y=%d, got=%d", want, got)
	}
}

func TestUseCase_MoveToPosition_RejectsWhenTargetNotPassable(t *testing.T) {
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
				{X: 1, Y: 0, Passable: false},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-pos-blocked",
		Intent: survival.ActionIntent{
			Type: survival.ActionMove,
			Pos:  &survival.Position{X: 1, Y: 0},
		},
	})
	if !errors.Is(err, ErrActionInvalidPosition) {
		t.Fatalf("expected ErrActionInvalidPosition, got %v", err)
	}
	var posErr *ActionInvalidPositionError
	if !errors.As(err, &posErr) || posErr == nil {
		t.Fatalf("expected ActionInvalidPositionError details, got %T", err)
	}
	if posErr.TargetPos == nil || posErr.TargetPos.X != 1 || posErr.TargetPos.Y != 0 {
		t.Fatalf("unexpected target_pos details: %+v", posErr.TargetPos)
	}
	if posErr.BlockingTilePos == nil || posErr.BlockingTilePos.X != 1 || posErr.BlockingTilePos.Y != 0 {
		t.Fatalf("unexpected blocking_tile_pos details: %+v", posErr.BlockingTilePos)
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

func TestUseCase_RetreatMovesAwayFromHighestThreatTile(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 0, Y: 0},
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
			WorldTimeSeconds: 10,
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: true, BaseThreat: 4},  // east threat
				{X: -1, Y: 0, Passable: true, BaseThreat: 1}, // west safer
				{X: 0, Y: 1, Passable: true, BaseThreat: 2},
				{X: 0, Y: -1, Passable: true, BaseThreat: 2},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700004000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-retreat-away-threat",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X != -1 || out.UpdatedState.Position.Y != 0 {
		t.Fatalf("expected retreat move west to (-1,0), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
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

func TestUseCase_RetreatIgnoresCenterThreatAndStillMovesAway(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 5, Y: 5},
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
			WorldTimeSeconds: 10,
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true, BaseThreat: 4},  // center threat
				{X: 1, Y: 0, Passable: true, BaseThreat: 4},  // east threat
				{X: -1, Y: 0, Passable: true, BaseThreat: 1}, // west safer
				{X: 0, Y: 1, Passable: true, BaseThreat: 2},
				{X: 0, Y: -1, Passable: true, BaseThreat: 2},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700005000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-retreat-center-threat",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X == 0 && out.UpdatedState.Position.Y == 0 {
		t.Fatalf("expected retreat to move away from threat, got no movement")
	}
}

func TestUseCase_RetreatAvoidsBlockedTile(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 0, Y: 0},
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
			WorldTimeSeconds: 10,
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: true, BaseThreat: 4},   // threat east
				{X: -1, Y: 0, Passable: false, BaseThreat: 1}, // west is blocked
				{X: 0, Y: 1, Passable: true, BaseThreat: 1},   // safe passable
				{X: 0, Y: -1, Passable: true, BaseThreat: 1},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700006000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-retreat-blocked",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X == -1 && out.UpdatedState.Position.Y == 0 {
		t.Fatalf("retreat selected blocked tile")
	}
}

func TestUseCase_GameOverEventIncludesLastKnownThreatWhenVisible(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 1, Hunger: -200, Energy: -50},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 0, Y: 0},
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
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 1, BaseThreat: 4, Resource: "wood"},
				{X: -1, Y: 0, BaseThreat: 2},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700007000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gameover-threat",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_1_1_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	found := false
	for _, evt := range out.Events {
		if evt.Type != "game_over" {
			continue
		}
		found = true
		last, _ := evt.Payload["last_known_threat"].(map[string]any)
		if last == nil || last["id"] == nil {
			t.Fatalf("expected last_known_threat in game_over payload, got=%v", evt.Payload["last_known_threat"])
		}
	}
	if !found {
		t.Fatalf("expected game_over event")
	}
}
