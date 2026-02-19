package status

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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
		VisibleTiles: []world.Tile{
			{X: 3, Y: 4, Zone: world.ZoneQuarry, Passable: true},
		},
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
	if got, want := resp.State.CurrentZone, string(world.ZoneQuarry); got != want {
		t.Fatalf("expected current_zone=%q, got %q", want, got)
	}
	if got, want := resp.State.SessionID, "session-agent-1"; got != want {
		t.Fatalf("expected session_id=%q, got %q", want, got)
	}
	if resp.World.Rules.StandardTickMinutes != survival.StandardTickMinutes {
		t.Fatalf("expected standard tick %d, got=%d", survival.StandardTickMinutes, resp.World.Rules.StandardTickMinutes)
	}
	if resp.World.Rules.DrainsPer30m.HungerDrain != survival.BaseHungerDrainPer30 || resp.World.Rules.DrainsPer30m.EnergyDrain != 0 {
		t.Fatalf("unexpected drains_per_30m: %+v", resp.World.Rules.DrainsPer30m)
	}
	if got := resp.ActionCosts["gather"]; got.BaseMinutes != survival.StandardTickMinutes || got.DeltaHunger != -7 || got.DeltaEnergy != -18 {
		t.Fatalf("gather action cost mismatch: %+v", got)
	}
	if got := resp.ActionCosts["sleep"]; got.BaseMinutes != survival.StandardTickMinutes || got.DeltaHunger != -4 || got.DeltaEnergy != survival.SleepBaseEnergyRecovery || got.DeltaHP != survival.SleepBaseHPRecovery {
		t.Fatalf("sleep action cost mismatch: %+v", got)
	}
	if got := resp.ActionCosts["sleep"].Variants["bed_quality_good"]; got.DeltaHunger != -4 || got.DeltaEnergy != 36 || got.DeltaHP != 12 {
		t.Fatalf("sleep good-bed variant mismatch: %+v", got)
	}
	if got, ok := resp.ActionCosts["terminate"]; !ok {
		t.Fatalf("expected terminate action cost configured")
	} else if got.BaseMinutes != 1 || got.DeltaHunger != 0 || got.DeltaEnergy != 0 {
		t.Fatalf("terminate action cost mismatch: %+v", got)
	}
	if resp.State.InventoryUsed != 3 {
		t.Fatalf("expected inventory_used=3, got=%d", resp.State.InventoryUsed)
	}
	if got := resp.World.Rules.Farming.WheatYieldRange; len(got) != 2 || got[0] != survival.WheatYieldMin || got[1] != survival.WheatYieldMax {
		t.Fatalf("expected wheat_yield_range [%d,%d], got=%v", survival.WheatYieldMin, survival.WheatYieldMax, got)
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

func TestUseCase_IncludesActionCooldownsInAgentState(t *testing.T) {
	now := time.Unix(1700100000, 0)
	uc := UseCase{
		StateRepo: statusStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 0, Y: 0},
		}},
		EventRepo: statusEventRepo{events: []survival.DomainEvent{
			{
				Type:       "action_settled",
				OccurredAt: now.Add(-20 * time.Second),
				Payload: map[string]any{
					"decision": map[string]any{"intent": "move"},
				},
			},
		}},
		World: statusWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:    "day",
			VisibleTiles: []world.Tile{{X: 0, Y: 0, Zone: world.ZoneSafe, Passable: true}},
		}},
		Now: func() time.Time { return now },
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.State.ActionCooldowns["move"] <= 0 {
		t.Fatalf("expected move cooldown remaining in agent_state, got=%v", resp.State.ActionCooldowns)
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

type statusEventRepo struct {
	events []survival.DomainEvent
	err    error
}

func (p statusWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	if p.err != nil {
		return world.Snapshot{}, p.err
	}
	return p.snapshot, nil
}

func (r statusEventRepo) Append(_ context.Context, _ string, _ []survival.DomainEvent) error {
	return nil
}

func (r statusEventRepo) ListByAgentID(_ context.Context, _ string, _ int) ([]survival.DomainEvent, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.events) == 0 {
		return nil, ports.ErrNotFound
	}
	out := make([]survival.DomainEvent, len(r.events))
	copy(out, r.events)
	return out, nil
}

var _ ports.AgentStateRepository = statusStateRepo{}
var _ ports.WorldProvider = statusWorldProvider{}
