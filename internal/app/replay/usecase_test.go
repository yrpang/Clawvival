package replay

import (
	"context"
	"testing"
	"time"

	"clawverse/internal/domain/survival"
)

func TestUseCase_ReconstructsLatestStateFromEvents(t *testing.T) {
	repo := fakeRepo{events: []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: time.Unix(1, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 80.0, "hunger": 70.0, "energy": 50.0, "x": 2.0, "y": 3.0}}},
		{Type: "action_settled", OccurredAt: time.Unix(2, 0), Payload: map[string]any{"state_after": map[string]any{"hp": 60.0, "hunger": 65.0, "energy": 40.0, "x": 3.0, "y": 4.0}}},
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

type fakeRepo struct {
	events []survival.DomainEvent
}

func (r fakeRepo) Append(_ context.Context, _ string, _ []survival.DomainEvent) error {
	return nil
}

func (r fakeRepo) ListByAgentID(_ context.Context, _ string, _ int) ([]survival.DomainEvent, error) {
	return r.events, nil
}
