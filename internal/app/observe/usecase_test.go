package observe

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

func TestUseCase_RejectsEmptyAgentID(t *testing.T) {
	uc := UseCase{}
	if _, err := uc.Execute(context.Background(), Request{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUseCase_PropagatesWorldError(t *testing.T) {
	wantErr := errors.New("world down")
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
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
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
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
	if got, want := resp.State.SessionID, "session-agent-1"; got != want {
		t.Fatalf("expected session_id=%q, got %q", want, got)
	}
	if resp.WorldTimeSeconds != 0 || resp.TimeOfDay != "" || resp.NextPhaseInSeconds != 0 {
		t.Fatalf("unexpected default time projection: world_time=%d time_of_day=%q next_phase=%d", resp.WorldTimeSeconds, resp.TimeOfDay, resp.NextPhaseInSeconds)
	}
	if resp.World.Rules.StandardTickMinutes != survival.StandardTickMinutes {
		t.Fatalf("expected standard tick %d, got=%d", survival.StandardTickMinutes, resp.World.Rules.StandardTickMinutes)
	}
	if resp.World.Rules.DrainsPer30m.HungerDrain != survival.BaseHungerDrainPer30 || resp.World.Rules.DrainsPer30m.EnergyDrain != 0 {
		t.Fatalf("unexpected drains_per_30m: %+v", resp.World.Rules.DrainsPer30m)
	}
	if got := resp.ActionCosts["gather"]; got.DeltaHunger != -7 || got.DeltaEnergy != -18 {
		t.Fatalf("gather action cost mismatch: %+v", got)
	}
	if got := resp.ActionCosts["sleep"]; got.DeltaHunger != 20 || got.DeltaEnergy != survival.SleepBaseEnergyRecovery || got.DeltaHP != survival.SleepBaseHPRecovery {
		t.Fatalf("sleep action cost mismatch: %+v", got)
	}
	if got := resp.ActionCosts["sleep"].Variants["bed_quality_good"]; got.DeltaHunger != 20 || got.DeltaEnergy != 45 || got.DeltaHP != 12 {
		t.Fatalf("sleep good-bed variant mismatch: %+v", got)
	}
	if got, ok := resp.ActionCosts["terminate"]; !ok {
		t.Fatalf("expected terminate action cost configured")
	} else if got.DeltaHunger != 0 || got.DeltaEnergy != 0 {
		t.Fatalf("terminate action cost mismatch: %+v", got)
	}
	if got := resp.World.Rules.Farming.WheatYieldRange; len(got) != 2 || got[0] != survival.WheatYieldMin || got[1] != survival.WheatYieldMax {
		t.Fatalf("expected wheat_yield_range [%d,%d], got=%v", survival.WheatYieldMin, survival.WheatYieldMax, got)
	}
	if got := resp.World.Rules.ProductionRecipes; len(got) < 2 {
		t.Fatalf("expected production_recipes, got=%v", got)
	} else {
		if got[0].RecipeID != int(survival.RecipePlank) || got[0].In["wood"] != 2 || got[0].Out["plank"] != 1 {
			t.Fatalf("unexpected first production recipe: %+v", got[0])
		}
		if got[1].RecipeID != int(survival.RecipeBread) || got[1].In["wheat"] != 2 || got[1].Out["bread"] != 1 {
			t.Fatalf("unexpected second production recipe: %+v", got[1])
		}
	}
	b, err := json.Marshal(resp.ActionCosts["gather"])
	if err != nil {
		t.Fatalf("marshal gather action cost: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal gather action cost: %v", err)
	}
	if _, ok := payload["delta_hunger"]; !ok {
		t.Fatalf("expected delta_hunger in gather action cost, got=%v", payload)
	}
	if _, ok := payload["delta_energy"]; !ok {
		t.Fatalf("expected delta_energy in gather action cost, got=%v", payload)
	}
	if _, ok := payload["requirements"]; !ok {
		t.Fatalf("expected requirements in gather action cost, got=%v", payload)
	}
}

func TestUseCase_ProjectsTilesResourcesAndThreats(t *testing.T) {
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:   "agent-1",
			Position:  survival.Position{X: 0, Y: 0},
			Vitals:    survival.Vitals{HP: 10, Hunger: -20, Energy: 10},
			Inventory: map[string]int{"wood": 2, "stone": 1},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 2,
			VisibleTiles: []world.Tile{{
				X: 0, Y: 0, Kind: world.TileTree, Zone: world.ZoneForest, Passable: false, Resource: "wood", BaseThreat: 2,
			}},
		}},
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got, want := resp.State.CurrentZone, string(world.ZoneForest); got != want {
		t.Fatalf("expected current_zone=%q, got %q", want, got)
	}
	if resp.LocalThreatLevel != 2 {
		t.Fatalf("expected local threat level 2, got %d", resp.LocalThreatLevel)
	}
	if resp.TimeOfDay != "day" {
		t.Fatalf("expected top-level time_of_day=day, got %q", resp.TimeOfDay)
	}
	if len(resp.Tiles) != 121 {
		t.Fatalf("expected 121 window tiles, got %d", len(resp.Tiles))
	}
	visibleCount := 0
	for _, tile := range resp.Tiles {
		if tile.IsVisible {
			visibleCount++
		}
	}
	if visibleCount != 1 {
		t.Fatalf("expected one visible tile in sparse window, got %d", visibleCount)
	}
	if len(resp.Resources) != 1 || resp.Resources[0].ID == "" {
		t.Fatalf("expected one resource with stable id, got %+v", resp.Resources)
	}
	if len(resp.Threats) != 1 || resp.Threats[0].DangerScore <= 0 {
		t.Fatalf("expected one threat with positive danger score, got %+v", resp.Threats)
	}
	if resp.State.InventoryUsed != 3 {
		t.Fatalf("expected inventory_used=3, got %d", resp.State.InventoryUsed)
	}
	if len(resp.State.StatusEffects) == 0 {
		t.Fatalf("expected derived status effects, got empty")
	}
	if !resp.HPDrainFeedback.IsLosingHP || resp.HPDrainFeedback.EstimatedLossPer30 <= 0 {
		t.Fatalf("expected hp_drain_feedback to indicate active hp loss, got=%+v", resp.HPDrainFeedback)
	}
}

func TestUseCase_ProjectsVisibleObjectsOnly(t *testing.T) {
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 0, Y: 0},
		}},
		ObjectRepo: observeObjectRepo{objects: []ports.WorldObjectRecord{
			{ObjectID: "obj-visible-box", ObjectType: "box", X: 0, Y: 0, CapacitySlots: 60, UsedSlots: 5},
			{ObjectID: "obj-hidden-farm", ObjectType: "farm_plot", X: 2, Y: 2, ObjectState: `{"state":"growing"}`},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay: "day",
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Kind: world.TileGrass, Passable: true},
			},
		}},
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(resp.Objects) != 1 {
		t.Fatalf("expected one visible object, got %+v", resp.Objects)
	}
	if resp.Objects[0].ID != "obj-visible-box" || resp.Objects[0].CapacitySlots != 60 || resp.Objects[0].UsedSlots != 5 {
		t.Fatalf("unexpected projected object: %+v", resp.Objects[0])
	}
}

func TestUseCase_NightVisibilityRadiusMasksOuterTiles(t *testing.T) {
	tiles := make([]world.Tile, 0, 121)
	for y := -5; y <= 5; y++ {
		for x := -5; x <= 5; x++ {
			tiles = append(tiles, world.Tile{
				X:        x,
				Y:        y,
				Kind:     world.TileGrass,
				Passable: true,
				Resource: "wood",
			})
		}
	}
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 0, Y: 0},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:    "night",
			VisibleTiles: tiles,
		}},
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	visible := 0
	for _, t := range resp.Tiles {
		if t.IsVisible {
			visible++
		}
	}
	if visible != 25 {
		t.Fatalf("expected night visible tiles=25 (radius 3 Manhattan), got %d", visible)
	}
	for _, res := range resp.Resources {
		if abs(res.Pos.X)+abs(res.Pos.Y) > 3 {
			t.Fatalf("resource outside night visibility leaked: %+v", res)
		}
	}
}

func TestUseCase_HidesDepletedGatherTargetAndUpdatesNearbySummary(t *testing.T) {
	now := time.Unix(1700000000, 0)
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 0, Y: 0},
		}},
		ResourceRepo: observeResourceRepo{recordsByAgent: map[string][]ports.AgentResourceNodeRecord{
			"agent-1": {
				{
					AgentID:       "agent-1",
					TargetID:      "res_0_0_wood",
					ResourceType:  "wood",
					X:             0,
					Y:             0,
					DepletedUntil: now.Add(50 * time.Minute),
				},
			},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay: "day",
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Kind: world.TileTree, Passable: false, Resource: "wood"},
				{X: 1, Y: 0, Kind: world.TileRock, Passable: false, Resource: "stone"},
			},
		}},
		Now: func() time.Time { return now },
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	for _, res := range resp.Resources {
		if res.ID == "res_0_0_wood" {
			t.Fatalf("expected depleted wood to be hidden from resources, got %+v", resp.Resources)
		}
	}
	for _, tile := range resp.Snapshot.VisibleTiles {
		if tile.X == 0 && tile.Y == 0 && tile.Resource != "" {
			t.Fatalf("expected depleted wood removed from snapshot tiles, got resource=%q", tile.Resource)
		}
	}
	if got := resp.Snapshot.NearbyResource["stone"]; got != 1 {
		t.Fatalf("expected stone nearby count=1, got=%d", got)
	}
	if got := resp.Snapshot.NearbyResource["wood"]; got != 0 {
		t.Fatalf("expected depleted wood nearby count=0, got=%d", got)
	}
}

func TestUseCase_IncludesActionCooldownsInAgentState(t *testing.T) {
	now := time.Unix(1700100000, 0)
	uc := UseCase{
		StateRepo: &observeStateRepo{state: survival.AgentStateAggregate{
			AgentID:  "agent-1",
			Position: survival.Position{X: 0, Y: 0},
		}},
		EventRepo: &observeEventRepo{eventsByAgent: map[string][]survival.DomainEvent{
			"agent-1": {
				{
					Type:       "action_settled",
					OccurredAt: now.Add(-20 * time.Second),
					Payload: map[string]any{
						"decision": map[string]any{"intent": "move"},
					},
				},
			},
		}},
		World: observeWorldProvider{snapshot: world.Snapshot{
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

func TestUseCase_SettlesDueOngoingActionBeforeObserveProjection(t *testing.T) {
	now := time.Unix(1700200000, 0)
	stateRepo := &observeStateRepo{state: survival.AgentStateAggregate{
		AgentID:   "agent-1",
		Vitals:    survival.Vitals{HP: 80, Hunger: 100, Energy: 20},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   3,
		OngoingAction: &survival.OngoingActionInfo{
			Type:    survival.ActionRest,
			Minutes: 60,
			EndAt:   now,
		},
	}}
	eventRepo := &observeEventRepo{eventsByAgent: map[string][]survival.DomainEvent{}}
	uc := UseCase{
		StateRepo: stateRepo,
		EventRepo: eventRepo,
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:         "day",
			WorldTimeSeconds:  3600,
			ThreatLevel:       0,
			VisibilityPenalty: 0,
			VisibleTiles:      []world.Tile{{X: 0, Y: 0, Zone: world.ZoneSafe, Passable: true}},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.State.OngoingAction != nil {
		t.Fatalf("expected ongoing action cleared after observe settle, got=%+v", resp.State.OngoingAction)
	}
	if resp.State.Vitals.Hunger != 120 {
		t.Fatalf("expected hunger=120 after 60m rest settle, got=%d", resp.State.Vitals.Hunger)
	}
	if resp.State.Vitals.Energy != 56 {
		t.Fatalf("expected energy=56 after 60m rest settle, got=%d", resp.State.Vitals.Energy)
	}
	if stateRepo.saveCalls != 1 {
		t.Fatalf("expected one state save for ongoing settle, got=%d", stateRepo.saveCalls)
	}
	if stateRepo.lastExpectedVersion != 3 {
		t.Fatalf("expected save with prior version=3, got=%d", stateRepo.lastExpectedVersion)
	}
	if stateRepo.lastSaved.Version != 4 {
		t.Fatalf("expected saved state version incremented to 4, got=%d", stateRepo.lastSaved.Version)
	}
	if len(eventRepo.appended["agent-1"]) == 0 {
		t.Fatalf("expected settle events appended for ongoing settle")
	}
	foundEnded := false
	for _, evt := range eventRepo.appended["agent-1"] {
		if evt.Type == "ongoing_action_ended" {
			foundEnded = true
			break
		}
	}
	if !foundEnded {
		t.Fatalf("expected ongoing_action_ended event, got=%+v", eventRepo.appended["agent-1"])
	}
}

func TestUseCase_OngoingSettleEventWorldTimeMatchesElapsedWindow(t *testing.T) {
	now := time.Unix(1700200000, 0)
	baseNow := now
	stateRepo := &observeStateRepo{state: survival.AgentStateAggregate{
		AgentID:   "agent-1",
		Vitals:    survival.Vitals{HP: 80, Hunger: 100, Energy: 20},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   3,
		OngoingAction: &survival.OngoingActionInfo{
			Type:    survival.ActionRest,
			Minutes: 60,
			EndAt:   now,
		},
	}}
	eventRepo := &observeEventRepo{eventsByAgent: map[string][]survival.DomainEvent{}}
	uc := UseCase{
		StateRepo: stateRepo,
		EventRepo: eventRepo,
		World: observeDynamicWorldProvider{
			startAt:       baseNow,
			startWorldSec: 3600,
			nowFn:         func() time.Time { return now },
			baseSnapshot: world.Snapshot{
				TimeOfDay:         "day",
				ThreatLevel:       0,
				VisibilityPenalty: 0,
				VisibleTiles:      []world.Tile{{X: 0, Y: 0, Zone: world.ZoneSafe, Passable: true}},
			},
		},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	now = now.Add(60 * time.Minute)
	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got, want := resp.WorldTimeSeconds, int64(7200); got != want {
		t.Fatalf("expected projected world_time_seconds=%d, got=%d", want, got)
	}
	appended := eventRepo.appended["agent-1"]
	foundSettled := false
	for _, evt := range appended {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		foundSettled = true
		if got, ok := evt.Payload["world_time_before_seconds"].(int64); !ok || got != 3600 {
			t.Fatalf("expected action_settled world_time_before_seconds=3600, got=%v", evt.Payload["world_time_before_seconds"])
		}
		if got, ok := evt.Payload["world_time_after_seconds"].(int64); !ok || got != 7200 {
			t.Fatalf("expected action_settled world_time_after_seconds=7200, got=%v", evt.Payload["world_time_after_seconds"])
		}
	}
	if !foundSettled {
		t.Fatalf("expected action_settled event appended for ongoing settle")
	}
}

func TestUseCase_DoesNotSettleIdleEnvironmentDuringObserve(t *testing.T) {
	now := time.Unix(1700300000, 0)
	stateRepo := &observeStateRepo{state: survival.AgentStateAggregate{
		AgentID:   "agent-1",
		Vitals:    survival.Vitals{HP: 100, Hunger: -40, Energy: -20},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   7,
		UpdatedAt: now.Add(-30 * time.Minute),
	}}
	eventRepo := &observeEventRepo{eventsByAgent: map[string][]survival.DomainEvent{}}
	uc := UseCase{
		StateRepo: stateRepo,
		EventRepo: eventRepo,
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:         "night",
			WorldTimeSeconds:  7200,
			ThreatLevel:       1,
			VisibilityPenalty: 1,
			VisibleTiles:      []world.Tile{{X: 0, Y: 0, Zone: world.ZoneSafe, Passable: true}},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.State.Vitals.Hunger != -40 {
		t.Fatalf("expected hunger unchanged during observe idle, got=%d", resp.State.Vitals.Hunger)
	}
	if resp.State.Vitals.HP != 100 {
		t.Fatalf("expected hp unchanged during observe idle, got=%d", resp.State.Vitals.HP)
	}
	if resp.State.Vitals.Energy != -20 {
		t.Fatalf("expected energy unchanged during observe idle, got=%d", resp.State.Vitals.Energy)
	}
	if stateRepo.saveCalls != 0 {
		t.Fatalf("expected no state save for idle observe, got=%d", stateRepo.saveCalls)
	}
	if len(eventRepo.appended["agent-1"]) != 0 {
		t.Fatalf("expected no idle settlement events appended, got=%+v", eventRepo.appended["agent-1"])
	}
}

func TestUseCase_DoesNotSettleIdleEvenAfterFullTick(t *testing.T) {
	now := time.Unix(1700400000, 0)
	stateRepo := &observeStateRepo{state: survival.AgentStateAggregate{
		AgentID:   "agent-1",
		Vitals:    survival.Vitals{HP: 100, Hunger: -40, Energy: -20},
		Position:  survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{},
		Version:   2,
		UpdatedAt: now.Add(-29 * time.Minute),
	}}
	eventRepo := &observeEventRepo{eventsByAgent: map[string][]survival.DomainEvent{}}
	uc := UseCase{
		StateRepo: stateRepo,
		EventRepo: eventRepo,
		World: observeWorldProvider{snapshot: world.Snapshot{
			TimeOfDay:         "day",
			WorldTimeSeconds:  100,
			ThreatLevel:       0,
			VisibilityPenalty: 0,
			VisibleTiles:      []world.Tile{{X: 0, Y: 0, Zone: world.ZoneSafe, Passable: true}},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return now },
	}

	resp, err := uc.Execute(context.Background(), Request{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if resp.State.Vitals.Hunger != -40 || resp.State.Vitals.HP != 100 || resp.State.Vitals.Energy != -20 {
		t.Fatalf("expected no idle settle on observe, got vitals=%+v", resp.State.Vitals)
	}
	if stateRepo.saveCalls != 0 {
		t.Fatalf("expected no save for idle observe, got=%d", stateRepo.saveCalls)
	}
	if len(eventRepo.appended["agent-1"]) != 0 {
		t.Fatalf("expected no events appended for idle observe, got=%+v", eventRepo.appended["agent-1"])
	}
}

type observeStateRepo struct {
	state survival.AgentStateAggregate
	err   error
	saveCalls           int
	lastSaved           survival.AgentStateAggregate
	lastExpectedVersion int64
}

func (r observeStateRepo) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	if r.err != nil {
		return survival.AgentStateAggregate{}, r.err
	}
	return r.state, nil
}

func (r *observeStateRepo) SaveWithVersion(_ context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	r.saveCalls++
	r.lastSaved = state
	r.lastExpectedVersion = expectedVersion
	r.state = state
	return nil
}

type observeWorldProvider struct {
	snapshot world.Snapshot
	err      error
}

type observeDynamicWorldProvider struct {
	startAt       time.Time
	startWorldSec int64
	nowFn         func() time.Time
	baseSnapshot  world.Snapshot
}

type observeEventRepo struct {
	eventsByAgent map[string][]survival.DomainEvent
	err           error
	appended      map[string][]survival.DomainEvent
}

type observeResourceRepo struct {
	recordsByAgent map[string][]ports.AgentResourceNodeRecord
	err            error
}

type observeObjectRepo struct {
	objects []ports.WorldObjectRecord
	err     error
}

func (r observeObjectRepo) Save(_ context.Context, _ string, _ ports.WorldObjectRecord) error {
	return nil
}

func (r observeObjectRepo) GetByObjectID(_ context.Context, _ string, _ string) (ports.WorldObjectRecord, error) {
	return ports.WorldObjectRecord{}, ports.ErrNotFound
}

func (r observeObjectRepo) ListByAgentID(_ context.Context, _ string) ([]ports.WorldObjectRecord, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.objects, nil
}

func (r observeObjectRepo) Update(_ context.Context, _ string, _ ports.WorldObjectRecord) error {
	return nil
}

func (r *observeEventRepo) Append(_ context.Context, agentID string, events []survival.DomainEvent) error {
	if r.eventsByAgent == nil {
		r.eventsByAgent = map[string][]survival.DomainEvent{}
	}
	if r.appended == nil {
		r.appended = map[string][]survival.DomainEvent{}
	}
	r.eventsByAgent[agentID] = append(r.eventsByAgent[agentID], events...)
	r.appended[agentID] = append(r.appended[agentID], events...)
	return nil
}

func (r observeEventRepo) ListByAgentID(_ context.Context, agentID string, _ int) ([]survival.DomainEvent, error) {
	if r.err != nil {
		return nil, r.err
	}
	events := r.eventsByAgent[agentID]
	if len(events) == 0 {
		return nil, ports.ErrNotFound
	}
	out := make([]survival.DomainEvent, len(events))
	copy(out, events)
	return out, nil
}

func (r observeResourceRepo) Upsert(_ context.Context, _ ports.AgentResourceNodeRecord) error {
	return nil
}

func (r observeResourceRepo) GetByTargetID(_ context.Context, _ string, _ string) (ports.AgentResourceNodeRecord, error) {
	return ports.AgentResourceNodeRecord{}, ports.ErrNotFound
}

func (r observeResourceRepo) ListByAgentID(_ context.Context, agentID string) ([]ports.AgentResourceNodeRecord, error) {
	if r.err != nil {
		return nil, r.err
	}
	records := r.recordsByAgent[agentID]
	if len(records) == 0 {
		return nil, ports.ErrNotFound
	}
	out := make([]ports.AgentResourceNodeRecord, len(records))
	copy(out, records)
	return out, nil
}

func (p observeWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	if p.err != nil {
		return world.Snapshot{}, p.err
	}
	return p.snapshot, nil
}

func (p observeDynamicWorldProvider) SnapshotForAgent(_ context.Context, _ string, _ world.Point) (world.Snapshot, error) {
	s := p.baseSnapshot
	nowAt := p.nowFn()
	elapsed := int64(nowAt.Sub(p.startAt).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	s.WorldTimeSeconds = p.startWorldSec + elapsed
	return s, nil
}

var _ ports.AgentStateRepository = &observeStateRepo{}
var _ ports.WorldProvider = observeWorldProvider{}
var _ ports.WorldProvider = observeDynamicWorldProvider{}
var _ ports.WorldObjectRepository = observeObjectRepo{}
var _ ports.AgentResourceNodeRepository = observeResourceRepo{}
var _ ports.EventRepository = &observeEventRepo{}
