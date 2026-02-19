package action

import (
	"context"
	"errors"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/shared/stateview"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func listRecentEvents(ctx context.Context, repo ports.EventRepository, agentID string) ([]survival.DomainEvent, error) {
	if repo == nil {
		return nil, nil
	}
	events, err := repo.ListByAgentID(ctx, agentID, 50)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, err
	}
	return events, nil
}

type ongoingFinalizeResult struct {
	Settled                bool
	UpdatedState           survival.AgentStateAggregate
	Events                 []survival.DomainEvent
	ResultCode             survival.ResultCode
	DTMinutes              int
	WorldTimeBeforeSeconds int64
	WorldTimeAfterSeconds  int64
}

func isInterruptibleOngoingActionType(t survival.ActionType) bool {
	return t == survival.ActionRest
}

func finalizeOngoingAction(ctx context.Context, u UseCase, agentID string, state survival.AgentStateAggregate, nowAt time.Time, forceTerminate bool) (ongoingFinalizeResult, error) {
	ongoing := state.OngoingAction
	if ongoing == nil {
		return ongoingFinalizeResult{}, nil
	}
	if forceTerminate && !isInterruptibleOngoingActionType(ongoing.Type) {
		return ongoingFinalizeResult{}, ErrActionPreconditionFailed
	}
	if nowAt.Before(ongoing.EndAt) && !forceTerminate {
		return ongoingFinalizeResult{}, nil
	}
	startAt := ongoing.EndAt.Add(-time.Duration(ongoing.Minutes) * time.Minute)
	deltaMinutes := int(nowAt.Sub(startAt).Minutes())
	if deltaMinutes < 0 {
		deltaMinutes = 0
	}
	if deltaMinutes > ongoing.Minutes {
		deltaMinutes = ongoing.Minutes
	}
	if deltaMinutes < minHeartbeatDeltaMinutes && !nowAt.Before(ongoing.EndAt) {
		deltaMinutes = minHeartbeatDeltaMinutes
	}

	snapshot, err := u.World.SnapshotForAgent(ctx, agentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return ongoingFinalizeResult{}, err
	}

	var result survival.SettlementResult
	if deltaMinutes > 0 {
		result, err = u.Settle.Settle(
			state,
			survival.ActionIntent{Type: ongoing.Type},
			survival.HeartbeatDelta{Minutes: deltaMinutes},
			nowAt,
			survival.WorldSnapshot{
				TimeOfDay:         snapshot.TimeOfDay,
				ThreatLevel:       snapshot.ThreatLevel,
				VisibilityPenalty: snapshot.VisibilityPenalty,
				NearbyResource:    snapshot.NearbyResource,
				WorldTimeSeconds:  snapshot.WorldTimeSeconds,
			},
		)
		if err != nil {
			return ongoingFinalizeResult{}, err
		}
	} else {
		result = survival.SettlementResult{
			UpdatedState: state,
			Events:       []survival.DomainEvent{},
			ResultCode:   survival.ResultOK,
		}
	}
	result.UpdatedState.OngoingAction = nil
	result.UpdatedState.UpdatedAt = nowAt
	result.UpdatedState = stateview.Enrich(result.UpdatedState, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))

	sessionID := "session-" + agentID
	for i := range result.Events {
		if result.Events[i].Payload == nil {
			result.Events[i].Payload = map[string]any{}
		}
		result.Events[i].Payload["agent_id"] = agentID
		result.Events[i].Payload["session_id"] = sessionID
	}
	result.Events = append(result.Events, survival.DomainEvent{
		Type:       "ongoing_action_ended",
		OccurredAt: nowAt,
		Payload: map[string]any{
			"agent_id":        agentID,
			"session_id":      sessionID,
			"action_type":     string(ongoing.Type),
			"planned_minutes": ongoing.Minutes,
			"actual_minutes":  deltaMinutes,
			"forced":          forceTerminate,
		},
	})
	if err := u.StateRepo.SaveWithVersion(ctx, result.UpdatedState, state.Version); err != nil {
		return ongoingFinalizeResult{}, err
	}
	if err := u.EventRepo.Append(ctx, agentID, result.Events); err != nil {
		return ongoingFinalizeResult{}, err
	}
	if u.SessionRepo != nil && result.ResultCode == survival.ResultGameOver {
		if err := u.SessionRepo.Close(ctx, sessionID, result.UpdatedState.DeathCause, nowAt); err != nil {
			return ongoingFinalizeResult{}, err
		}
	}
	worldTimeBefore, worldTimeAfter := worldTimeWindowFromEvents(result.Events, snapshot.WorldTimeSeconds, deltaMinutes)
	return ongoingFinalizeResult{
		Settled:                true,
		UpdatedState:           result.UpdatedState,
		Events:                 result.Events,
		ResultCode:             result.ResultCode,
		DTMinutes:              deltaMinutes,
		WorldTimeBeforeSeconds: worldTimeBefore,
		WorldTimeAfterSeconds:  worldTimeAfter,
	}, nil
}

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
