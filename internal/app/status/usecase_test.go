package status

import (
	"context"
	"testing"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_IncludesWorldTimeInfo(t *testing.T) {
	repo := statusStateRepo{state: survival.AgentStateAggregate{
		AgentID:  "agent-1",
		Position: survival.Position{X: 3, Y: 4},
	}}
	worldProvider := statusWorldProvider{snapshot: world.Snapshot{
		TimeOfDay:          "night",
		NextPhaseInSeconds: 123,
	}}

	uc := UseCase{StateRepo: repo, World: worldProvider}
	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.TimeOfDay != "night" {
		t.Fatalf("expected night, got %s", resp.TimeOfDay)
	}
	if resp.NextPhaseInSeconds != 123 {
		t.Fatalf("expected next phase 123, got %d", resp.NextPhaseInSeconds)
	}
}

type statusStateRepo struct {
	state survival.AgentStateAggregate
}

func (r statusStateRepo) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	return r.state, nil
}

func (r statusStateRepo) SaveWithVersion(_ context.Context, _ survival.AgentStateAggregate, _ int64) error {
	return nil
}

type statusWorldProvider struct {
	snapshot world.Snapshot
}

func (p statusWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	return p.snapshot, nil
}

var _ ports.AgentStateRepository = statusStateRepo{}
var _ ports.WorldProvider = statusWorldProvider{}
