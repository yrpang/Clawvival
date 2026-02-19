package action

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/shared/resourcestate"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type gatherActionHandler struct{ BaseHandler }

func validateGatherActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.TargetID) != ""
}

func (h gatherActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if err := runStandardActionPrecheck(ctx, uc, ac); err != nil {
		return err
	}
	if err := validateTargetVisibility(ac.View.StateWorking.Position, ac.Tmp.ResolvedIntent, ac.View.Snapshot); err != nil {
		return err
	}
	return validateGatherTargetState(ctx, uc.ResourceRepo, ac.In.AgentID, ac.Tmp.ResolvedIntent, ac.In.NowAt)
}

func (h gatherActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{
		filterGatherNearby: true,
		applySeedPity:      true,
		applyGatherDeplete: true,
		applyObjectAction:  true,
		createBuiltObjects: true,
	})
}

func applySeedPityIfNeeded(ctx context.Context, intent survival.ActionIntent, result *survival.SettlementResult, before survival.AgentStateAggregate, repo ports.EventRepository, agentID string) {
	if intent.Type != survival.ActionGather || result == nil {
		return
	}
	beforeSeed := before.Inventory["seed"]
	afterSeed := result.UpdatedState.Inventory["seed"]
	seedGained := afterSeed > beforeSeed

	if evt := findActionSettledEvent(result.Events); evt != nil {
		if evt.Payload == nil {
			evt.Payload = map[string]any{}
		}
		res, _ := evt.Payload["result"].(map[string]any)
		if res == nil {
			res = map[string]any{}
		}
		res["seed_gained"] = seedGained
		res["seed_pity_triggered"] = false
		evt.Payload["result"] = res
	}
	if seedGained || repo == nil {
		return
	}

	fails := consecutiveGatherSeedFails(ctx, repo, agentID)
	if fails < seedPityMaxFails-1 {
		return
	}

	result.UpdatedState.AddItem("seed", 1)
	if evt := findActionSettledEvent(result.Events); evt != nil {
		res, _ := evt.Payload["result"].(map[string]any)
		if res == nil {
			res = map[string]any{}
		}
		res["seed_gained"] = true
		res["seed_pity_triggered"] = true
		evt.Payload["result"] = res
	}
	result.Events = append(result.Events, survival.DomainEvent{
		Type:       "seed_pity_triggered",
		OccurredAt: result.UpdatedState.UpdatedAt,
		Payload: map[string]any{
			"agent_id": agentID,
			"granted":  1,
		},
	})
}

func consecutiveGatherSeedFails(ctx context.Context, repo ports.EventRepository, agentID string) int {
	events, err := repo.ListByAgentID(ctx, agentID, 100)
	if err != nil {
		return 0
	}
	fails := 0
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		decision, _ := evt.Payload["decision"].(map[string]any)
		if decision == nil || strings.TrimSpace(fmt.Sprint(decision["intent"])) != string(survival.ActionGather) {
			continue
		}
		result, _ := evt.Payload["result"].(map[string]any)
		if result == nil {
			break
		}
		if gained, ok := result["seed_gained"].(bool); ok && gained {
			break
		}
		fails++
	}
	return fails
}

func findActionSettledEvent(events []survival.DomainEvent) *survival.DomainEvent {
	for i := range events {
		if events[i].Type == "action_settled" {
			return &events[i]
		}
	}
	return nil
}

func filterGatherNearbyResource(targetID string, nearby map[string]int) map[string]int {
	_, _, resource, ok := resourcestate.ParseResourceTargetID(targetID)
	if !ok || strings.TrimSpace(resource) == "" {
		return map[string]int{}
	}
	resource = strings.ToLower(strings.TrimSpace(resource))
	return map[string]int{resource: 1}
}

func validateTargetVisibility(center survival.Position, intent survival.ActionIntent, snapshot world.Snapshot) error {
	if intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" {
		return nil
	}
	tx, ty, resource, ok := resourcestate.ParseResourceTargetID(intent.TargetID)
	if !ok {
		return ErrActionPreconditionFailed
	}
	viewRadius := snapshot.ViewRadius
	if viewRadius <= 0 {
		viewRadius = 5
	}
	if tx < center.X-viewRadius || tx > center.X+viewRadius || ty < center.Y-viewRadius || ty > center.Y+viewRadius {
		return ErrTargetOutOfView
	}
	if strings.EqualFold(snapshot.TimeOfDay, "night") {
		dist := abs(tx-center.X) + abs(ty-center.Y)
		if dist > actionNightVisionRadius {
			return ErrTargetNotVisible
		}
	}
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == tx && tile.Y == ty {
			if resource != "" && !strings.EqualFold(strings.TrimSpace(tile.Resource), strings.TrimSpace(resource)) {
				return ErrActionPreconditionFailed
			}
			return nil
		}
	}
	return ErrTargetNotVisible
}

func validateGatherTargetState(ctx context.Context, repo ports.AgentResourceNodeRepository, agentID string, intent survival.ActionIntent, now time.Time) error {
	if intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" || repo == nil {
		return nil
	}
	record, err := repo.GetByTargetID(ctx, agentID, strings.TrimSpace(intent.TargetID))
	if errors.Is(err, ports.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !record.DepletedUntil.After(now) {
		return nil
	}
	remaining := int(record.DepletedUntil.Sub(now).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	return &ResourceDepletedError{TargetID: intent.TargetID, RemainingSeconds: remaining}
}

func persistGatherDepletion(ctx context.Context, repo ports.AgentResourceNodeRepository, agentID string, intent survival.ActionIntent, now time.Time) error {
	if repo == nil || intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" {
		return nil
	}
	x, y, resource, ok := resourcestate.ParseResourceTargetID(intent.TargetID)
	if !ok {
		return nil
	}
	return repo.Upsert(ctx, ports.AgentResourceNodeRecord{
		AgentID:       agentID,
		TargetID:      strings.TrimSpace(intent.TargetID),
		ResourceType:  resource,
		X:             x,
		Y:             y,
		DepletedUntil: now.Add(resourcestate.RespawnDuration(resource)),
	})
}
