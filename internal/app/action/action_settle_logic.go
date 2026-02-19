package action

import (
	"context"
	"strings"

	"clawvival/internal/app/cooldown"
	"clawvival/internal/app/ports"
	"clawvival/internal/app/stateview"
	"clawvival/internal/domain/survival"
)

type settleOptions struct {
	filterGatherNearby bool
	applySeedPity      bool
	applyGatherDeplete bool
	applyObjectAction  bool
	createBuiltObjects bool
}

func settleViaDomainOrInstant(ctx context.Context, uc UseCase, ac *ActionContext, opts settleOptions) (ExecuteMode, error) {
	intent := ac.Tmp.ResolvedIntent
	deltaMinutes := 0
	var result survival.SettlementResult

	if intent.Type == survival.ActionSleep {
		if ac.View.PreparedObj == nil {
			return ExecuteModeContinue, ErrActionPreconditionFailed
		}
		result = settleInstantSleepAction(ac.View.StateWorking, intent, ac.View.PreparedObj.record, ac.In.NowAt, ac.View.Snapshot)
	} else {
		var err error
		deltaMinutes, err = resolveHeartbeatDeltaMinutes(ctx, uc.EventRepo, ac.In.AgentID, ac.In.NowAt)
		if err != nil {
			return ExecuteModeContinue, err
		}
		settleNearby := ac.View.Snapshot.NearbyResource
		if opts.filterGatherNearby {
			settleNearby = filterGatherNearbyResource(intent.TargetID, ac.View.Snapshot.NearbyResource)
		}
		result, err = uc.Settle.Settle(
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
		DT:             deltaMinutes,
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
		SettledDTMinutes:       deltaMinutes,
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
