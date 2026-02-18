package resourcestate

import (
	"testing"
	"time"

	"clawvival/internal/domain/survival"
)

func TestDepletedTargets_TracksLatestGatherPerTarget(t *testing.T) {
	now := time.Unix(2000, 0)
	events := []survival.DomainEvent{
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-70 * time.Minute),
			Payload: map[string]any{
				"decision": map[string]any{
					"intent": "gather",
					"params": map[string]any{"target_id": "res_0_0_wood"},
				},
			},
		},
		{
			Type:       "action_settled",
			OccurredAt: now.Add(-10 * time.Minute),
			Payload: map[string]any{
				"decision": map[string]any{
					"intent": "gather",
					"params": map[string]any{"target_id": "res_1_0_stone"},
				},
			},
		},
	}

	depleted := DepletedTargets(events, now)
	if _, ok := depleted["res_0_0_wood"]; ok {
		t.Fatalf("wood should have respawned, got depleted=%v", depleted)
	}
	if remaining := depleted["res_1_0_stone"]; remaining < 2900 || remaining > 3600 {
		t.Fatalf("stone remaining seconds out of expected range, got=%d", remaining)
	}
}

func TestParseResourceTargetID(t *testing.T) {
	x, y, resource, ok := ParseResourceTargetID("res_-3_5_wood")
	if !ok {
		t.Fatalf("expected target id parse success")
	}
	if x != -3 || y != 5 || resource != "wood" {
		t.Fatalf("unexpected parse result: x=%d y=%d resource=%s", x, y, resource)
	}
}
