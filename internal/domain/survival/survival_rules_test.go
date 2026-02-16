package survival

import (
	"testing"
	"time"
)

func TestSettlementService_NightCombatHigherRisk(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{AgentID: "a-1", Vitals: Vitals{HP: 100, Hunger: 80, Energy: 60}, Version: 1}
	now := time.Now()

	dayOut, err := svc.Settle(state, ActionIntent{Type: ActionCombat, Params: map[string]int{"target_level": 1}}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{TimeOfDay: "day", ThreatLevel: 1})
	if err != nil {
		t.Fatalf("day settle error: %v", err)
	}
	nightOut, err := svc.Settle(state, ActionIntent{Type: ActionCombat, Params: map[string]int{"target_level": 1}}, HeartbeatDelta{Minutes: 30}, now, WorldSnapshot{TimeOfDay: "night", ThreatLevel: 4})
	if err != nil {
		t.Fatalf("night settle error: %v", err)
	}

	if nightOut.UpdatedState.Vitals.HP >= dayOut.UpdatedState.Vitals.HP {
		t.Fatalf("expected night combat to lose more HP, day=%d night=%d", dayOut.UpdatedState.Vitals.HP, nightOut.UpdatedState.Vitals.HP)
	}
}

func TestSettlementService_RetreatMovesTowardHome(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 5, Y: -3},
		Home:     Position{X: 0, Y: 0},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionRetreat}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{TimeOfDay: "night", ThreatLevel: 4})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if abs(out.UpdatedState.Position.X) >= abs(state.Position.X) {
		t.Fatalf("expected retreat to reduce X distance")
	}
	if abs(out.UpdatedState.Position.Y) >= abs(state.Position.Y) {
		t.Fatalf("expected retreat to reduce Y distance")
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
