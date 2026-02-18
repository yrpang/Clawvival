package replay

import (
	"context"
	"testing"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

func TestUseCase_ReconstructsLatestStateFromEvents(t *testing.T) {
	repo := fakeRepo{events: []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: time.Unix(2, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 60.0, "hunger": 65.0, "energy": 40.0, "x": 3.0, "y": 4.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(1, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 80.0, "hunger": 70.0, "energy": 50.0, "x": 2.0, "y": 3.0}}},
	}}

	uc := UseCase{Events: repo}
	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", Limit: 10})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if out.LatestState.Vitals.HP != 60 {
		t.Fatalf("expected latest hp=60, got %d", out.LatestState.Vitals.HP)
	}
	if len(out.Events) != 2 {
		t.Fatalf("expected 2 events")
	}
}

func TestUseCase_FiltersEventsByTimeWindow(t *testing.T) {
	repo := fakeRepo{events: []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: time.Unix(100, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 80.0, "hunger": 70.0, "energy": 50.0, "x": 2.0, "y": 3.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(200, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 60.0, "hunger": 65.0, "energy": 40.0, "x": 3.0, "y": 4.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(300, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 55.0, "hunger": 60.0, "energy": 35.0, "x": 4.0, "y": 5.0}}},
	}}

	uc := UseCase{Events: repo}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:      "agent-1",
		Limit:        10,
		OccurredFrom: 150,
		OccurredTo:   250,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got, want := len(out.Events), 1; got != want {
		t.Fatalf("filtered events mismatch: got=%d want=%d", got, want)
	}
	if got, want := out.LatestState.Vitals.HP, 60; got != want {
		t.Fatalf("latest hp mismatch: got=%d want=%d", got, want)
	}
}

func TestUseCase_AppliesFiltersBeforeLimit(t *testing.T) {
	repo := fakeRepoWithLimit{events: []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: time.Unix(300, 0), Payload: map[string]any{"session_id": "session-a", "state_after": map[string]any{"hp": 90.0, "hunger": 70.0, "energy": 50.0, "x": 1.0, "y": 1.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(200, 0), Payload: map[string]any{"session_id": "session-b", "state_after": map[string]any{"hp": 60.0, "hunger": 65.0, "energy": 40.0, "x": 3.0, "y": 4.0}}},
	}}

	uc := UseCase{Events: repo}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:   "agent-1",
		Limit:     1,
		SessionID: "session-b",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got, want := len(out.Events), 1; got != want {
		t.Fatalf("filtered events mismatch: got=%d want=%d", got, want)
	}
	if got, want := out.Events[0].Payload["session_id"], "session-b"; got != want {
		t.Fatalf("expected filtered session-b event, got=%v", got)
	}
}

func TestUseCase_FiltersEventsBySessionID(t *testing.T) {
	repo := fakeRepo{events: []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: time.Unix(100, 0), Payload: map[string]any{"session_id": "session-a", "state_after": map[string]any{"hp": 80.0, "hunger": 70.0, "energy": 50.0, "x": 2.0, "y": 3.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(200, 0), Payload: map[string]any{"session_id": "session-b", "state_after": map[string]any{"hp": 60.0, "hunger": 65.0, "energy": 40.0, "x": 3.0, "y": 4.0}}},
	}}

	uc := UseCase{Events: repo}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:   "agent-1",
		Limit:     10,
		SessionID: "session-b",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got, want := len(out.Events), 1; got != want {
		t.Fatalf("filtered events mismatch: got=%d want=%d", got, want)
	}
	if got, want := out.LatestState.Vitals.HP, 60; got != want {
		t.Fatalf("latest hp mismatch: got=%d want=%d", got, want)
	}
}

func TestUseCase_EmptyEventsReturnsEmptyResponse(t *testing.T) {
	repo := fakeRepoErr{err: ports.ErrNotFound}
	uc := UseCase{Events: repo}

	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", Limit: 10})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := len(out.Events); got != 0 {
		t.Fatalf("expected empty events, got=%d", got)
	}
	if got, want := out.LatestState.AgentID, "agent-1"; got != want {
		t.Fatalf("expected latest state agent id=%s, got=%s", want, got)
	}
}

type fakeRepo struct {
	events []survival.DomainEvent
}

func (r fakeRepo) Append(_ context.Context, _ string, _ []survival.DomainEvent) error {
	return nil
}

func (r fakeRepo) ListByAgentID(_ context.Context, _ string, _ int) ([]survival.DomainEvent, error) {
	return r.events, nil
}

type fakeRepoWithLimit struct {
	events []survival.DomainEvent
}

func (r fakeRepoWithLimit) Append(_ context.Context, _ string, _ []survival.DomainEvent) error {
	return nil
}

func (r fakeRepoWithLimit) ListByAgentID(_ context.Context, _ string, limit int) ([]survival.DomainEvent, error) {
	if limit <= 0 || limit > len(r.events) {
		limit = len(r.events)
	}
	out := make([]survival.DomainEvent, limit)
	copy(out, r.events[:limit])
	return out, nil
}

type fakeRepoErr struct {
	err error
}

func (r fakeRepoErr) Append(_ context.Context, _ string, _ []survival.DomainEvent) error {
	return nil
}

func (r fakeRepoErr) ListByAgentID(_ context.Context, _ string, _ int) ([]survival.DomainEvent, error) {
	return nil, r.err
}
