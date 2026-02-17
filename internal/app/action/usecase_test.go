package action

import (
	"context"
	"errors"
	"testing"
	"time"

	worldmock "clawverse/internal/adapter/world/mock"
	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	}

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

func TestUseCase_RejectsMissingIntent(t *testing.T) {
	uc := UseCase{}
	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1",
		DeltaMinutes:   30,
	})
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
		Intent:         survival.ActionIntent{Type: survival.ActionType("unknown")},
		DeltaMinutes:   30,
	})
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
		{AgentID: "agent-1", IdempotencyKey: "k1", Intent: survival.ActionIntent{Type: survival.ActionMove}, DeltaMinutes: 30},
		{AgentID: "agent-1", IdempotencyKey: "k2", Intent: survival.ActionIntent{Type: survival.ActionCombat}, DeltaMinutes: 30},
		{AgentID: "agent-1", IdempotencyKey: "k3", Intent: survival.ActionIntent{Type: survival.ActionBuild}, DeltaMinutes: 30},
		{AgentID: "agent-1", IdempotencyKey: "k4", Intent: survival.ActionIntent{Type: survival.ActionFarm}, DeltaMinutes: 30},
		{AgentID: "agent-1", IdempotencyKey: "k5", Intent: survival.ActionIntent{Type: survival.ActionCraft}, DeltaMinutes: 30},
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
		Intent:         survival.ActionIntent{Type: survival.ActionCombat, Params: map[string]int{"target_level": 1}},
		DeltaMinutes:   30,
	})
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
		StrategyHash:   "sha-123",
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
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	})
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
			Type:   survival.ActionBuild,
			Params: map[string]int{"kind": int(survival.BuildBed)},
		},
		DeltaMinutes: 30,
	})
	if !errors.Is(err, ErrActionPreconditionFailed) {
		t.Fatalf("expected ErrActionPreconditionFailed, got %v", err)
	}
}
