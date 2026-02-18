package status

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_IncludesWorldTimeInfo(t *testing.T) {
	repo := statusStateRepo{state: survival.AgentStateAggregate{
		AgentID:   "agent-1",
		Position:  survival.Position{X: 3, Y: 4},
		Vitals:    survival.Vitals{HP: 12, Hunger: 20, Energy: 5},
		Inventory: map[string]int{"wood": 1, "seed": 2},
	}}
	worldProvider := statusWorldProvider{snapshot: world.Snapshot{
		WorldTimeSeconds:   456,
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
	if resp.WorldTimeSeconds != 456 {
		t.Fatalf("expected world time 456, got %d", resp.WorldTimeSeconds)
	}
	if resp.NextPhaseInSeconds != 123 {
		t.Fatalf("expected next phase 123, got %d", resp.NextPhaseInSeconds)
	}
	if resp.World.Rules.StandardTickMinutes != 30 {
		t.Fatalf("expected standard tick 30, got=%d", resp.World.Rules.StandardTickMinutes)
	}
	if resp.World.Rules.DrainsPer30m.HungerDrain != 4 || resp.World.Rules.DrainsPer30m.EnergyDrain != 0 {
		t.Fatalf("unexpected drains_per_30m: %+v", resp.World.Rules.DrainsPer30m)
	}
	if got := resp.ActionCosts["gather"]; got.DeltaHunger != -3 || got.DeltaEnergy != -18 {
		t.Fatalf("gather action cost mismatch: %+v", got)
	}
	if resp.State.InventoryUsed != 3 {
		t.Fatalf("expected inventory_used=3, got=%d", resp.State.InventoryUsed)
	}
	if got := resp.World.Rules.Farming.WheatYieldRange; len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("expected wheat_yield_range [1,3], got=%v", got)
	}
	if len(resp.State.StatusEffects) == 0 {
		t.Fatalf("expected status effects for low hp/energy")
	}
	if resp.HPDrainFeedback.IsLosingHP {
		t.Fatalf("did not expect hp drain at hunger=20 energy=5, got=%+v", resp.HPDrainFeedback)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal status response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}
	if _, ok := payload["action_costs"]; !ok {
		t.Fatalf("expected action_costs in status response, got=%v", payload)
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
