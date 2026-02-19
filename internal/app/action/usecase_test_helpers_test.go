package action

import (
	"context"
	"strings"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

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

type errorActionRepo struct {
	err error
}

func (r *errorActionRepo) GetByIdempotencyKey(_ context.Context, _, _ string) (*ports.ActionExecutionRecord, error) {
	return nil, r.err
}

func (r *errorActionRepo) SaveExecution(_ context.Context, _ ports.ActionExecutionRecord) error {
	return r.err
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

type stubActionMetrics struct {
	successCalls  int
	conflictCalls int
	failureCalls  int
	lastResult    survival.ResultCode
}

func (m *stubActionMetrics) RecordSuccess(resultCode survival.ResultCode) {
	m.successCalls++
	m.lastResult = resultCode
}

func (m *stubActionMetrics) RecordConflict() {
	m.conflictCalls++
}

func (m *stubActionMetrics) RecordFailure() {
	m.failureCalls++
}

type conflictOnSaveStateRepo struct {
	stubStateRepo
}

func (r *conflictOnSaveStateRepo) SaveWithVersion(_ context.Context, _ survival.AgentStateAggregate, _ int64) error {
	return ports.ErrConflict
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
