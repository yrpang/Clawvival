package action

import (
	"context"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/shared/cooldown"
	"clawvival/internal/app/shared/stateview"
	"clawvival/internal/domain/survival"
)

func runStandardActionPrecheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if uc.SessionRepo != nil {
		if err := uc.SessionRepo.EnsureActive(ctx, ac.In.SessionID, ac.In.AgentID, ac.View.StateWorking.Version); err != nil {
			return err
		}
	}
	intent := ac.Tmp.ResolvedIntent
	if err := ensureCooldownReady(ac.View.EventsBefore, intent.Type, ac.In.NowAt); err != nil {
		return err
	}
	return nil
}

func ensureCooldownReady(events []survival.DomainEvent, intentType survival.ActionType, now time.Time) error {
	remaining, ok := cooldown.RemainingForAction(events, intentType, now)
	if !ok {
		return nil
	}
	return &ActionCooldownActiveError{
		IntentType:       intentType,
		RemainingSeconds: remaining,
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type settleOptions struct {
	filterGatherNearby bool
	applySeedPity      bool
	applyGatherDeplete bool
	applyObjectAction  bool
	createBuiltObjects bool
}

func settleViaDomainOrInstant(ctx context.Context, uc UseCase, ac *ActionContext, opts settleOptions) (ExecuteMode, error) {
	intent := ac.Tmp.ResolvedIntent
	deltaMinutes := survival.StandardTickMinutes
	settleNearby := ac.View.Snapshot.NearbyResource
	if opts.filterGatherNearby {
		settleNearby = filterGatherNearbyResource(intent.TargetID, ac.View.Snapshot.NearbyResource)
	}
	result, err := uc.Settle.Settle(
		ac.View.StateWorking,
		intent,
		survival.HeartbeatDelta{Minutes: deltaMinutes},
		ac.In.NowAt,
		survival.WorldSnapshot{
			TimeOfDay:         ac.View.Snapshot.TimeOfDay,
			ThreatLevel:       ac.View.Snapshot.ThreatLevel,
			VisibilityPenalty: ac.View.Snapshot.VisibilityPenalty,
			NearbyResource:    settleNearby,
			WorldTimeSeconds:  ac.View.Snapshot.WorldTimeSeconds,
		},
	)
	if err != nil {
		return ExecuteModeContinue, err
	}

	result.UpdatedState = stateview.Enrich(result.UpdatedState, ac.View.Snapshot.TimeOfDay, isCurrentTileLit(ac.View.Snapshot.TimeOfDay))
	result.UpdatedState.CurrentZone = stateview.CurrentZoneAtPosition(result.UpdatedState.Position, ac.View.Snapshot.VisibleTiles)
	result.UpdatedState.ActionCooldowns = cooldown.RemainingByActionWithCurrent(ac.View.EventsBefore, ac.In.NowAt, intent.Type)
	if ac.View.Snapshot.PhaseChanged && deltaMinutes > 0 {
		result.Events = append(result.Events, survival.DomainEvent{
			Type:       "world_phase_changed",
			OccurredAt: ac.In.NowAt,
			Payload: map[string]any{
				"from": ac.View.Snapshot.PhaseFrom,
				"to":   ac.View.Snapshot.PhaseTo,
			},
		})
	}
	if opts.applySeedPity {
		applySeedPityIfNeeded(ctx, intent, &result, ac.View.StateWorking, uc.EventRepo, ac.In.AgentID)
	}
	attachLastKnownThreat(&result, ac.View.Snapshot)

	ac.Tmp.DeltaMinutes = deltaMinutes
	ac.Tmp.SettleResult = result
	ac.Plan.StateToSave = &result.UpdatedState
	ac.Plan.StateVersion = ac.View.StateWorking.Version
	ac.Plan.EventsToAppend = result.Events
	ac.Plan.ExecutionToSave = &portsActionExecutionRecord{
		AgentID:        ac.In.AgentID,
		IdempotencyKey: ac.In.IdempotencyKey,
		IntentType:     string(intent.Type),
		Result: actionResult{
			UpdatedState: result.UpdatedState,
			Events:       result.Events,
			ResultCode:   result.ResultCode,
		},
		AppliedAt: ac.In.NowAt,
	}
	ac.Plan.ResultCode = result.ResultCode
	ac.Plan.ShouldPersist = true
	ac.Plan.ApplyGatherDepletion = opts.applyGatherDeplete
	ac.Plan.ApplyObjectAction = opts.applyObjectAction
	ac.Plan.CreateBuiltObjects = opts.createBuiltObjects
	ac.Plan.CloseSession = result.ResultCode == survival.ResultGameOver
	ac.Plan.CloseSessionCause = result.UpdatedState.DeathCause

	before, after := worldTimeWindow(ac.View.Snapshot.WorldTimeSeconds, deltaMinutes)
	ac.Tmp.Response = Response{
		WorldTimeBeforeSeconds: before,
		WorldTimeAfterSeconds:  after,
		UpdatedState:           result.UpdatedState,
		Events:                 result.Events,
		Settlement:             settlementSummary(result.Events),
		ResultCode:             result.ResultCode,
	}
	return ExecuteModeContinue, nil
}

type portsActionExecutionRecord = ports.ActionExecutionRecord
type actionResult = ports.ActionResult

func isCurrentTileLit(timeOfDay string) bool {
	return strings.EqualFold(strings.TrimSpace(timeOfDay), "day")
}
