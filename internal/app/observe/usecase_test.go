package observe

import (
	"context"
	"errors"
	"testing"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_RejectsEmptyAgentID(t *testing.T) {
	uc := UseCase{}
	if _, err := uc.Execute(context.Background(), Request{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUseCase_PropagatesWorldError(t *testing.T) {
	wantErr := errors.New("world down")
	uc := UseCase{
		StateRepo: observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 1, Y: 2},
		}},
		World: observeWorldProvider{err: wantErr},
	}

	if _, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected world error %v, got %v", wantErr, err)
	}
}

type observeStateRepo struct {
	state survival.AgentStateAggregate
	err   error
}

func (r observeStateRepo) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	if r.err != nil {
		return survival.AgentStateAggregate{}, r.err
	}
	return r.state, nil
}

func (r observeStateRepo) SaveWithVersion(_ context.Context, _ survival.AgentStateAggregate, _ int64) error {
	return nil
}

type observeWorldProvider struct {
	snapshot world.Snapshot
	err      error
}

func (p observeWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	if p.err != nil {
		return world.Snapshot{}, p.err
	}
	return p.snapshot, nil
}

var _ ports.AgentStateRepository = observeStateRepo{}
var _ ports.WorldProvider = observeWorldProvider{}
