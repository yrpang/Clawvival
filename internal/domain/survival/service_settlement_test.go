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
