package cooldown

import (
	"fmt"
	"strings"
	"time"

	"clawvival/internal/domain/survival"
)

var ActionDurations = map[survival.ActionType]time.Duration{
	survival.ActionBuild:     5 * time.Minute,
	survival.ActionCraft:     5 * time.Minute,
	survival.ActionFarmPlant: 3 * time.Minute,
	survival.ActionMove:      1 * time.Minute,
	survival.ActionSleep:     5 * time.Minute,
}

func RemainingForAction(events []survival.DomainEvent, intentType survival.ActionType, now time.Time) (int, bool) {
	cooldown, ok := ActionDurations[intentType]
	if !ok {
		return 0, false
	}
	lastAt := latestActionAt(events, intentType)
	if lastAt.IsZero() {
		return 0, false
	}
	remaining := cooldown - now.Sub(lastAt)
	if remaining <= 0 {
		return 0, false
	}
	remainingSeconds := int((remaining + time.Second - 1) / time.Second)
	if remainingSeconds < 1 {
		remainingSeconds = 1
	}
	return remainingSeconds, true
}

func RemainingByAction(events []survival.DomainEvent, now time.Time) map[string]int {
	out := map[string]int{}
	for actionType := range ActionDurations {
		if remaining, ok := RemainingForAction(events, actionType, now); ok {
			out[string(actionType)] = remaining
		}
	}
	return out
}

func latestActionAt(events []survival.DomainEvent, intentType survival.ActionType) time.Time {
	lastAt := time.Time{}
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		decision, ok := evt.Payload["decision"].(map[string]any)
		if !ok {
			continue
		}
		intent, _ := decision["intent"].(string)
		if strings.TrimSpace(intent) != string(intentType) {
			continue
		}
		if evt.OccurredAt.After(lastAt) {
			lastAt = evt.OccurredAt
		}
	}
	return lastAt
}

func eventForIntent(intentType survival.ActionType, occurredAt time.Time) survival.DomainEvent {
	return survival.DomainEvent{
		Type:       "action_settled",
		OccurredAt: occurredAt,
		Payload: map[string]any{
			"decision": map[string]any{
				"intent": fmt.Sprint(intentType),
			},
		},
	}
}

func RemainingByActionWithCurrent(events []survival.DomainEvent, now time.Time, current survival.ActionType) map[string]int {
	extended := make([]survival.DomainEvent, 0, len(events)+1)
	extended = append(extended, events...)
	extended = append(extended, eventForIntent(current, now))
	return RemainingByAction(extended, now)
}
