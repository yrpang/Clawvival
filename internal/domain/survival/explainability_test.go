package survival

import (
	"testing"
	"time"
)

func TestSettlementEventContainsExplainabilityFields(t *testing.T) {
	svc := SettlementService{}
	state := AgentStateAggregate{
		AgentID:  "a-1",
		Vitals:   Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: Position{X: 1, Y: 2},
		Version:  1,
	}

	out, err := svc.Settle(state, ActionIntent{Type: ActionGather}, HeartbeatDelta{Minutes: 30}, time.Now(), WorldSnapshot{TimeOfDay: "day", ThreatLevel: 1})
	if err != nil {
		t.Fatalf("settle error: %v", err)
	}
	if len(out.Events) == 0 {
		t.Fatalf("expected events")
	}
	payload := out.Events[0].Payload
	if payload["state_before"] == nil {
		t.Fatalf("expected state_before in payload")
	}
	if payload["decision"] == nil {
		t.Fatalf("expected decision in payload")
	}
	if payload["state_after"] == nil {
		t.Fatalf("expected state_after in payload")
	}
}
