package action

import (
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

func settlementSummary(events []survival.DomainEvent) map[string]any {
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		result, ok := evt.Payload["result"].(map[string]any)
		if !ok || result == nil {
			return nil
		}
		return map[string]any{
			"hp_loss":               result["hp_loss"],
			"inventory_delta":       result["inventory_delta"],
			"vitals_delta":          result["vitals_delta"],
			"vitals_change_reasons": result["vitals_change_reasons"],
		}
	}
	return nil
}

func worldTimeWindow(beforeSeconds int64, dtMinutes int) (int64, int64) {
	return beforeSeconds, beforeSeconds + int64(dtMinutes*60)
}

func worldTimeWindowFromExecution(exec *ports.ActionExecutionRecord) (int64, int64) {
	if exec == nil {
		return 0, 0
	}
	if before, after, ok := worldTimeWindowFromEventPayload(exec.Result.Events); ok {
		return before, after
	}
	return worldTimeWindow(0, exec.DT)
}

func worldTimeWindowFromEvents(events []survival.DomainEvent, fallbackBefore int64, dtMinutes int) (int64, int64) {
	if before, after, ok := worldTimeWindowFromEventPayload(events); ok {
		return before, after
	}
	return worldTimeWindow(fallbackBefore, dtMinutes)
}

func worldTimeWindowFromEventPayload(events []survival.DomainEvent) (int64, int64, bool) {
	for _, evt := range events {
		if evt.Payload == nil {
			continue
		}
		beforeRaw, hasBefore := evt.Payload["world_time_before_seconds"]
		afterRaw, hasAfter := evt.Payload["world_time_after_seconds"]
		if !hasBefore || !hasAfter {
			continue
		}
		return int64(toNum(beforeRaw)), int64(toNum(afterRaw)), true
	}
	return 0, 0, false
}

func toNum(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}
