package resourcestate

import (
	"fmt"
	"strings"
	"time"

	"clawvival/internal/domain/survival"
)

const (
	defaultRespawnMinutes = 60
)

var respawnByResource = map[string]time.Duration{
	"wood":  60 * time.Minute,
	"stone": 60 * time.Minute,
	"berry": 30 * time.Minute,
	"seed":  30 * time.Minute,
}

func ParseResourceTargetID(targetID string) (x int, y int, resource string, ok bool) {
	var prefix string
	if _, err := fmt.Sscanf(strings.TrimSpace(targetID), "%3s_%d_%d_%s", &prefix, &x, &y, &resource); err != nil {
		return 0, 0, "", false
	}
	if prefix != "res" {
		return 0, 0, "", false
	}
	resource = strings.ToLower(strings.TrimSpace(resource))
	if resource == "" {
		return 0, 0, "", false
	}
	return x, y, resource, true
}

func BuildResourceTargetID(x, y int, resource string) string {
	return fmt.Sprintf("res_%d_%d_%s", x, y, strings.ToLower(strings.TrimSpace(resource)))
}

func DepletedTargets(events []survival.DomainEvent, now time.Time) map[string]int {
	latestGather := map[string]time.Time{}
	respawn := map[string]time.Duration{}
	for _, evt := range events {
		targetID, resource, ok := gatherTargetFromEvent(evt)
		if !ok {
			continue
		}
		targetID = strings.TrimSpace(targetID)
		lastAt, seen := latestGather[targetID]
		if seen && !evt.OccurredAt.After(lastAt) {
			continue
		}
		latestGather[targetID] = evt.OccurredAt
		respawn[targetID] = RespawnDuration(resource)
	}

	out := map[string]int{}
	for targetID, gatheredAt := range latestGather {
		remaining := respawn[targetID] - now.Sub(gatheredAt)
		if remaining <= 0 {
			continue
		}
		remainingSeconds := int(remaining.Seconds())
		if remainingSeconds < 1 {
			remainingSeconds = 1
		}
		out[targetID] = remainingSeconds
	}
	return out
}

func gatherTargetFromEvent(evt survival.DomainEvent) (targetID, resource string, ok bool) {
	if evt.Type != "action_settled" || evt.Payload == nil {
		return "", "", false
	}
	decision, _ := evt.Payload["decision"].(map[string]any)
	if decision == nil {
		return "", "", false
	}
	if strings.TrimSpace(fmt.Sprint(decision["intent"])) != "gather" {
		return "", "", false
	}
	params, _ := decision["params"].(map[string]any)
	if params == nil {
		return "", "", false
	}
	targetID = strings.TrimSpace(fmt.Sprint(params["target_id"]))
	if targetID == "" {
		return "", "", false
	}
	_, _, resource, ok = ParseResourceTargetID(targetID)
	if !ok {
		return "", "", false
	}
	return targetID, resource, true
}

func RespawnDuration(resource string) time.Duration {
	if d, ok := respawnByResource[strings.ToLower(strings.TrimSpace(resource))]; ok {
		return d
	}
	return defaultRespawnMinutes * time.Minute
}
