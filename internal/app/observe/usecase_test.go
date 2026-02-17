package observe

import (
	"context"
	"errors"
	"testing"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
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

func TestUseCase_BuildsFixedViewMetadata(t *testing.T) {
	uc := UseCase{
		StateRepo: observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 7, Y: -2},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{}},
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.View.Width != 11 || resp.View.Height != 11 || resp.View.Radius != 5 {
		t.Fatalf("unexpected view shape: %+v", resp.View)
	}
	if resp.View.Center.X != 7 || resp.View.Center.Y != -2 {
		t.Fatalf("unexpected view center: %+v", resp.View.Center)
	}
	if resp.World.Rules.StandardTickMinutes != 30 {
		t.Fatalf("expected standard tick 30, got=%d", resp.World.Rules.StandardTickMinutes)
	}
	if resp.ActionCosts["move"].BaseMinutes <= 0 {
		t.Fatalf("expected move action cost configured, got=%+v", resp.ActionCosts["move"])
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
