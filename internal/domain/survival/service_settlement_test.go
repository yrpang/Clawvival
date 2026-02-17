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

	out, err := svc.Settle(state, ActionIntent{Type: ActionCombat}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{
		TimeOfDay:   "night",
		ThreatLevel: 3,
	})
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
	if got, ok := settled.Payload["settled_dt_minutes"].(int); !ok || got != 30 {
		t.Fatalf("expected settled_dt_minutes=30, got=%v", settled.Payload["settled_dt_minutes"])
	}
}
