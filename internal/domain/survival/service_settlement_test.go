package survival

import (
	"testing"
	"time"
)

func TestSettlementService_GatherConsumesStats(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 100, Hunger: 80, Energy: 60},
		Version: 1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}

	if out.UpdatedState.Vitals.Hunger >= state.Vitals.Hunger {
		t.Fatalf("expected hunger to decrease")
	}
	if out.UpdatedState.Vitals.Energy >= state.Vitals.Energy {
		t.Fatalf("expected energy to decrease")
	}
	if out.ResultCode != ResultOK {
		t.Fatalf("expected result ok, got %s", out.ResultCode)
	}
}

func TestSettlementService_GameOver(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 1, Hunger: -100, Energy: -100},
		Version: 1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.ResultCode != ResultGameOver {
		t.Fatalf("expected game over, got %s", out.ResultCode)
	}
}

func TestSettlementService_CriticalHP(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 25, Hunger: -100, Energy: 0},
		Version: 1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.ResultCode != ResultOK {
		t.Fatalf("expected result ok, got %s", out.ResultCode)
	}
	if out.UpdatedState.Vitals.HP > 20 || out.UpdatedState.Vitals.HP <= 0 {
		t.Fatalf("expected hp in critical range (1-20), got %d", out.UpdatedState.Vitals.HP)
	}

	foundCritical := false
	for _, e := range out.Events {
		if e.Type == "critical_hp" {
			foundCritical = true
			break
		}
	}
	if !foundCritical {
		t.Fatalf("expected critical_hp event")
	}
}

func TestSettlementService_CriticalHPAutoRetreatsTowardHome(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 22, Hunger: -120, Energy: 10},
		Position: Position{X: 5, Y: 5},
		Home:     Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.ResultCode != ResultOK {
		t.Fatalf("expected result ok, got %s", out.ResultCode)
	}
	if out.UpdatedState.Vitals.HP <= 0 || out.UpdatedState.Vitals.HP > 20 {
		t.Fatalf("expected critical hp range, got %d", out.UpdatedState.Vitals.HP)
	}
	if out.UpdatedState.Position.X != 4 || out.UpdatedState.Position.Y != 4 {
		t.Fatalf("expected auto retreat toward home to (4,4), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}
}

func TestSettlementService_NoCriticalEventsAboveCriticalThreshold(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 28, Hunger: -100, Energy: 100},
		Position: Position{X: 5, Y: 5},
		Home:     Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.ResultCode != ResultOK {
		t.Fatalf("expected result ok, got %s", out.ResultCode)
	}
	if got, want := out.UpdatedState.Vitals.HP, 19; got != want {
		t.Fatalf("expected hp=%d, got %d", want, got)
	}
	for _, evt := range out.Events {
		if evt.Type == "critical_hp" || evt.Type == "force_retreat" {
			t.Fatalf("did not expect critical events above threshold, got=%s", evt.Type)
		}
	}
}

func TestSettlementService_MoveChangesPositionAndConsumesEnergy(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 2, Y: -1},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type:      ActionMove,
		Direction: "E",
		DX:        1,
		DY:        -1,
	}, HeartbeatDelta{Minutes: 1}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}

	if out.UpdatedState.Position.X != 3 || out.UpdatedState.Position.Y != -2 {
		t.Fatalf("expected moved position (3,-2), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}
	if out.UpdatedState.Vitals.Energy >= state.Vitals.Energy {
		t.Fatalf("expected move to consume energy, before=%d after=%d", state.Vitals.Energy, out.UpdatedState.Vitals.Energy)
	}
}

func TestSettlementService_EatRecoversHungerAndConsumesFood(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 100, Hunger: 40, Energy: 60},
		Inventory: map[string]int{
			"berry": 2,
		},
		Version: 1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type:     ActionEat,
		ItemType: "berry",
		Count:    1,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}

	if out.UpdatedState.Vitals.Hunger <= state.Vitals.Hunger {
		t.Fatalf("expected hunger recover, before=%d after=%d", state.Vitals.Hunger, out.UpdatedState.Vitals.Hunger)
	}
	if got, want := out.UpdatedState.Inventory["berry"], 1; got != want {
		t.Fatalf("expected berry consumed by 1, got=%d want=%d", got, want)
	}
}

func TestSettlementService_SleepRecoversEnergyAndHp(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 60, Hunger: 70, Energy: 20},
		Position: Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type: ActionSleep,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.UpdatedState.Vitals.Energy <= state.Vitals.Energy {
		t.Fatalf("expected sleep to recover energy, before=%d after=%d", state.Vitals.Energy, out.UpdatedState.Vitals.Energy)
	}
	if out.UpdatedState.Vitals.HP <= state.Vitals.HP {
		t.Fatalf("expected sleep to recover hp, before=%d after=%d", state.Vitals.HP, out.UpdatedState.Vitals.HP)
	}
	if out.UpdatedState.Vitals.Hunger <= state.Vitals.Hunger {
		t.Fatalf("expected sleep to recover hunger, before=%d after=%d", state.Vitals.Hunger, out.UpdatedState.Vitals.Hunger)
	}
}

func TestSettlementService_TwoSleepTicksReachTargetRecovery(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 80, Hunger: 20, Energy: 20},
		Position: Position{X: 0, Y: 0},
		Version:  1,
	}

	first, err := svc.Settle(state, ActionIntent{
		Type: ActionSleep,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("first sleep settle error: %v", err)
	}
	second, err := svc.Settle(first.UpdatedState, ActionIntent{
		Type: ActionSleep,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("second sleep settle error: %v", err)
	}
	if got, want := second.UpdatedState.Vitals.Hunger, 60; got != want {
		t.Fatalf("expected hunger=%d after two sleep ticks, got=%d", want, got)
	}
	if got, want := second.UpdatedState.Vitals.Energy, 80; got != want {
		t.Fatalf("expected energy=%d after two sleep ticks, got=%d", want, got)
	}
}

func TestSettlementService_RestRecoversHungerSlightly(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 80, Hunger: 80, Energy: 20},
		Position: Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type: ActionRest,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got, want := out.UpdatedState.Vitals.Hunger, 90; got != want {
		t.Fatalf("expected rest hunger=%d, got=%d", want, got)
	}
}

func TestSettlementService_SleepGoodBedAppliesQualityMultiplier(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 40, Hunger: 70, Energy: 10},
		Position: Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type:       ActionSleep,
		BedID:      "bed-good",
		BedQuality: "GOOD",
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got, want := out.UpdatedState.Vitals.Energy, 55; got != want {
		t.Fatalf("expected good bed energy=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.HP, 52; got != want {
		t.Fatalf("expected good bed hp=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Vitals.Hunger, 90; got != want {
		t.Fatalf("expected good bed hunger=%d, got=%d", want, got)
	}
}

func TestSettlementService_FarmPlantConsumesSeed(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 100, Hunger: 90, Energy: 80},
		Inventory: map[string]int{
			"seed": 2,
		},
		Version: 1,
	}

	out, err := svc.Settle(state, ActionIntent{
		Type: ActionFarmPlant,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got, want := out.UpdatedState.Inventory["seed"], 1; got != want {
		t.Fatalf("expected seed consumed by 1, got=%d want=%d", got, want)
	}
}

func TestSettlementService_ActionSettledIncludesWorldTimeFields(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 100, Hunger: 80, Energy: 60},
		Version: 1,
	}
	out, err := svc.Settle(state, ActionIntent{
		Type: ActionGather,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{
		WorldTimeSeconds: 1200,
	})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	var settled *DomainEvent
	for i := range out.Events {
		if out.Events[i].Type == "action_settled" {
			settled = &out.Events[i]
			break
		}
	}
	if settled == nil {
		t.Fatalf("expected action_settled event")
	}
	if got, ok := settled.Payload["world_time_before_seconds"].(int64); !ok || got != 1200 {
		t.Fatalf("expected world_time_before_seconds=1200, got=%v", settled.Payload["world_time_before_seconds"])
	}
	if got, ok := settled.Payload["world_time_after_seconds"].(int64); !ok || got != 3000 {
		t.Fatalf("expected world_time_after_seconds=3000, got=%v", settled.Payload["world_time_after_seconds"])
	}
	before, ok := settled.Payload["state_before"].(map[string]any)
	if !ok {
		t.Fatalf("expected state_before object, got=%T", settled.Payload["state_before"])
	}
	if _, ok := before["x"]; !ok {
		t.Fatalf("expected state_before.x, got=%v", before)
	}
	if _, ok := before["y"]; !ok {
		t.Fatalf("expected state_before.y, got=%v", before)
	}
	if _, ok := before["pos"]; !ok {
		t.Fatalf("expected state_before.pos, got=%v", before)
	}
	if _, ok := before["inventory_used"]; !ok {
		t.Fatalf("expected state_before.inventory_used, got=%v", before)
	}
	after, ok := settled.Payload["state_after"].(map[string]any)
	if !ok {
		t.Fatalf("expected state_after object, got=%T", settled.Payload["state_after"])
	}
	if _, ok := after["x"]; !ok {
		t.Fatalf("expected state_after.x, got=%v", after)
	}
	if _, ok := after["y"]; !ok {
		t.Fatalf("expected state_after.y, got=%v", after)
	}
	if _, ok := after["pos"]; !ok {
		t.Fatalf("expected state_after.pos, got=%v", after)
	}
	if _, ok := after["inventory_used"]; !ok {
		t.Fatalf("expected state_after.inventory_used, got=%v", after)
	}
}

func TestSettlementService_ActionSettledIncludesVitalsChangeReasons(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID: "a-1",
		Vitals:  Vitals{HP: 50, Hunger: 0, Energy: 0},
		Version: 1,
	}
	out, err := svc.Settle(state, ActionIntent{
		Type: ActionGather,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{WorldTimeSeconds: 100})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	var settled *DomainEvent
	for i := range out.Events {
		if out.Events[i].Type == "action_settled" {
			settled = &out.Events[i]
			break
		}
	}
	if settled == nil {
		t.Fatalf("expected action_settled event")
	}
	result, ok := settled.Payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result payload, got=%T", settled.Payload["result"])
	}
	vitalsDelta, ok := result["vitals_delta"].(map[string]int)
	if !ok {
		t.Fatalf("expected vitals_delta map[string]int, got=%T", result["vitals_delta"])
	}
	if vitalsDelta["hp"] >= 0 || vitalsDelta["hunger"] >= 0 || vitalsDelta["energy"] >= 0 {
		t.Fatalf("expected all vitals to decrease, got=%v", vitalsDelta)
	}
	reasonsByVital, ok := result["vitals_change_reasons"].(map[string]any)
	if !ok {
		t.Fatalf("expected vitals_change_reasons map, got=%T", result["vitals_change_reasons"])
	}
	if len(asReasonList(t, reasonsByVital["hunger"])) == 0 {
		t.Fatalf("expected hunger reasons, got=%v", reasonsByVital["hunger"])
	}
	if len(asReasonList(t, reasonsByVital["energy"])) == 0 {
		t.Fatalf("expected energy reasons, got=%v", reasonsByVital["energy"])
	}
	hpReasons := asReasonList(t, reasonsByVital["hp"])
	if len(hpReasons) == 0 {
		t.Fatalf("expected hp reasons, got=%v", reasonsByVital["hp"])
	}
}

func asReasonList(t *testing.T, v any) []map[string]any {
	t.Helper()
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("reason item should be map, got=%T", item)
			}
			out = append(out, m)
		}
		return out
	default:
		t.Fatalf("reasons should be list, got=%T", v)
		return nil
	}
}

func TestSettlementService_RetreatUsesExplicitDirectionWhenProvided(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 0, Y: 0},
		Home:     Position{X: 5, Y: 5},
		Version:  1,
	}
	out, err := svc.Settle(state, ActionIntent{
		Type: ActionRetreat,
		DX:   -1,
		DY:   0,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.UpdatedState.Position.X != -1 || out.UpdatedState.Position.Y != 0 {
		t.Fatalf("expected retreat move to (-1,0), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}
}

func TestSettlementService_RetreatWithoutDirectionDoesNotFallbackToHome(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 0, Y: 0},
		Home:     Position{X: 5, Y: 5},
		Version:  1,
	}
	out, err := svc.Settle(state, ActionIntent{
		Type: ActionRetreat,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if out.UpdatedState.Position.X != 0 || out.UpdatedState.Position.Y != 0 {
		t.Fatalf("expected no fallback movement, got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}
}

func TestSettlementService_FarmHarvestReturnsSeedOnChance(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:    "a-1",
		Vitals:     Vitals{HP: 100, Hunger: 80, Energy: 60},
		Inventory:  map[string]int{},
		DeathCause: DeathCauseUnknown,
		Version:    1,
	}
	out, err := svc.Settle(state, ActionIntent{Type: ActionFarmHarvest}, HeartbeatDelta{Minutes: 30}, time.Unix(10, 0), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got := out.UpdatedState.Inventory["wheat"]; got != 2 {
		t.Fatalf("expected wheat yield=2, got=%d", got)
	}
	if got := out.UpdatedState.Inventory["seed"]; got != 1 {
		t.Fatalf("expected seed return on 20%% chance tick, got=%d", got)
	}
}

func TestSettlementService_FarmHarvestNoSeedWhenChanceMisses(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:    "a-1",
		Vitals:     Vitals{HP: 100, Hunger: 80, Energy: 60},
		Inventory:  map[string]int{},
		DeathCause: DeathCauseUnknown,
		Version:    1,
	}
	out, err := svc.Settle(state, ActionIntent{Type: ActionFarmHarvest}, HeartbeatDelta{Minutes: 30}, time.Unix(11, 0), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got := out.UpdatedState.Inventory["seed"]; got != 0 {
		t.Fatalf("expected no seed return when chance misses, got=%d", got)
	}
}

func TestSettlementService_ActionSettledIncludesInventoryDelta(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:    "a-1",
		Vitals:     Vitals{HP: 100, Hunger: 80, Energy: 60},
		Inventory:  map[string]int{},
		DeathCause: DeathCauseUnknown,
		Version:    1,
	}
	out, err := svc.Settle(state, ActionIntent{Type: ActionFarmHarvest}, HeartbeatDelta{Minutes: 30}, time.Unix(11, 0), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	var settled *DomainEvent
	for i := range out.Events {
		if out.Events[i].Type == "action_settled" {
			settled = &out.Events[i]
			break
		}
	}
	if settled == nil {
		t.Fatalf("expected action_settled event")
	}
	result, ok := settled.Payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result payload, got=%T", settled.Payload["result"])
	}
	delta, ok := result["inventory_delta"].(map[string]int)
	if !ok {
		t.Fatalf("expected inventory_delta, got=%T value=%v", result["inventory_delta"], result["inventory_delta"])
	}
	if got, want := delta["wheat"], 2; got != want {
		t.Fatalf("expected inventory_delta.wheat=%d, got=%d", want, got)
	}
}

func TestSettlementService_DoesNotMutateInputState(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:   "a-1",
		Vitals:    Vitals{HP: 100, Hunger: 80, Energy: 60},
		Inventory: map[string]int{"wheat": 0},
		Version:   1,
	}
	_, err := svc.Settle(state, ActionIntent{Type: ActionFarmHarvest}, HeartbeatDelta{Minutes: 30}, time.Unix(11, 0), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got := state.Inventory["wheat"]; got != 0 {
		t.Fatalf("expected input state unchanged, wheat=%d", got)
	}
}

func TestSettlementService_EatRespectsCount(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:   "a-1",
		Vitals:    Vitals{HP: 100, Hunger: 10, Energy: 60},
		Inventory: map[string]int{"berry": 2},
		Version:   1,
	}
	out, err := svc.Settle(state, ActionIntent{
		Type:     ActionEat,
		ItemType: "berry",
		Count:    2,
	}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if got := out.UpdatedState.Inventory["berry"]; got != 0 {
		t.Fatalf("expected berry inventory=0 after eating 2, got=%d", got)
	}
	if got := out.UpdatedState.Vitals.Hunger; got <= 20 {
		t.Fatalf("expected hunger to increase after eat count=2, got=%d", got)
	}
}
