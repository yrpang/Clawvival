package status

import (
	"context"
	"errors"
	"testing"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
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

func TestUseCase_RejectsEmptyAgentID(t *testing.T) {
	uc := UseCase{}
	if _, err := uc.Execute(context.Background(), Request{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUseCase_PropagatesStateRepoError(t *testing.T) {
	wantErr := errors.New("state repo down")
	uc := UseCase{
		StateRepo: statusStateRepo{err: wantErr},
		World:     statusWorldProvider{},
	}
	if _, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected state repo error %v, got %v", wantErr, err)
	}
}

func TestUseCase_PropagatesWorldError(t *testing.T) {
	wantErr := errors.New("world down")
	uc := UseCase{
		StateRepo: statusStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 1, Y: 2},
		}},
		World: statusWorldProvider{err: wantErr},
	}
	if _, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected world error %v, got %v", wantErr, err)
	}
}

type statusStateRepo struct {
	state survival.AgentStateAggregate
	err   error
}

func (r statusStateRepo) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	if r.err != nil {
		return survival.AgentStateAggregate{}, r.err
	}
	return r.state, nil
}

func (r statusStateRepo) SaveWithVersion(_ context.Context, _ survival.AgentStateAggregate, _ int64) error {
	return nil
}

type statusWorldProvider struct {
	snapshot world.Snapshot
	err      error
}

func (p statusWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	if p.err != nil {
		return world.Snapshot{}, p.err
	}
	return p.snapshot, nil
}

var _ ports.AgentStateRepository = statusStateRepo{}
var _ ports.WorldProvider = statusWorldProvider{}
