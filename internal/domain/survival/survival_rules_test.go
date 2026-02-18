package survival

import (
	"testing"
	"time"
)

func TestSettlementService_NonCombatHpLossConsistentAcrossDayNight(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{AgentID: "a-1", Vitals: Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1}
	now := time.Now()

	dayOut, err := svc.Settle(state, ActionIntent{Type: ActionRetreat}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{TimeOfDay: "day", ThreatLevel: 1})
	if err != nil {
		t.Fatalf("day settle error: %v", err)
	}
	nightOut, err := svc.Settle(state, ActionIntent{Type: ActionRetreat}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{TimeOfDay: "night", ThreatLevel: 4})
	if err != nil {
		t.Fatalf("night settle error: %v", err)
	}

	if nightOut.UpdatedState.Vitals.HP != dayOut.UpdatedState.Vitals.HP {
		t.Fatalf("expected day/night non-combat hp loss equal, day=%d night=%d", dayOut.UpdatedState.Vitals.HP, nightOut.UpdatedState.Vitals.HP)
	}
}

func TestSettlementService_RetreatMovesByIntentDirection(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 5, Y: -3},
		Home:     Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionRetreat, DX: -1, DY: 1}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{TimeOfDay: "night", ThreatLevel: 4})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if abs(out.UpdatedState.Position.X) >= abs(state.Position.X) {
		t.Fatalf("expected retreat to reduce X distance from risk direction")
	}
	if abs(out.UpdatedState.Position.Y) >= abs(state.Position.Y) {
		t.Fatalf("expected retreat to reduce Y distance from risk direction")
	}
}

func TestSettlementService_GameOverMarksDeathCause(t *testing.T) {
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
		t.Fatalf("expected game over")
	}
	if !out.UpdatedState.Dead {
		t.Fatalf("expected dead state")
	}
	if out.UpdatedState.DeathCause == "" {
		t.Fatalf("expected death cause")
	}
}

func TestSettlementService_NonCombatNotAffectedByVisibilityPenalty(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{AgentID: "a-1", Vitals: Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1}
	now := time.Now()

	lowPenalty, err := svc.Settle(state, ActionIntent{Type: ActionRetreat}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{
		TimeOfDay:         "night",
		ThreatLevel:       3,
		VisibilityPenalty: 0,
	})
	if err != nil {
		t.Fatalf("low penalty settle error: %v", err)
	}
	highPenalty, err := svc.Settle(state, ActionIntent{Type: ActionRetreat}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{
		TimeOfDay:         "night",
		ThreatLevel:       3,
		VisibilityPenalty: 2,
	})
	if err != nil {
		t.Fatalf("high penalty settle error: %v", err)
	}

	if highPenalty.UpdatedState.Vitals.HP != lowPenalty.UpdatedState.Vitals.HP {
		t.Fatalf("expected visibility penalty to not affect non-combat hp, low=%d high=%d", lowPenalty.UpdatedState.Vitals.HP, highPenalty.UpdatedState.Vitals.HP)
	}
}

func TestSettlementService_GameOverEventContainsLastObservableSnapshot(t *testing.T) {
	svc := SettlementService{}
	now := time.Unix(1700000000, 0)
	state := AgentStateAggregate{
		AgentID:   "a-1",
		Vitals:    Vitals{HP: 1, Hunger: -200, Energy: -20},
		Position:  Position{X: 2, Y: -1},
		Home:      Position{X: 0, Y: 0},
		Inventory: map[string]int{"wood": 4, "berry": 2},
		Version:   1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{WorldTimeSeconds: 1234})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	var gameOver *DomainEvent
	for i := range out.Events {
		if out.Events[i].Type == "game_over" {
			gameOver = &out.Events[i]
			break
		}
	}
	if gameOver == nil {
		t.Fatalf("expected game_over event")
	}
	if gameOver.Payload == nil {
		t.Fatalf("expected game_over payload")
	}
	if got, ok := gameOver.Payload["death_cause"].(string); !ok || got == "" {
		t.Fatalf("expected death_cause in payload, got=%v", gameOver.Payload["death_cause"])
	}
	before, ok := gameOver.Payload["state_before_last_action"].(map[string]any)
	if !ok {
		t.Fatalf("expected state_before_last_action object")
	}
	if got, ok := before["world_time_seconds"].(int64); !ok || got != 1234 {
		t.Fatalf("expected world_time_seconds=1234, got=%v", before["world_time_seconds"])
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
