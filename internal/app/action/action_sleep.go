package action

import (
	"context"
	"strings"
	"time"

	"clawvival/internal/domain/survival"
)

type sleepActionHandler struct{ BaseHandler }

func validateSleepActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.BedID) != ""
}

func (h sleepActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h sleepActionHandler) ExecuteActionAndPlan(_ context.Context, _ UseCase, ac *ActionContext) (ExecuteMode, error) {
	if ac.View.PreparedObj == nil {
		return ExecuteModeContinue, ErrActionPreconditionFailed
	}
	next := ac.View.StateWorking
	sleepMinutes := survival.StandardTickMinutes
	next.OngoingAction = &survival.OngoingActionInfo{
		Type:    survival.ActionSleep,
		Minutes: sleepMinutes,
		EndAt:   ac.In.NowAt.Add(time.Duration(sleepMinutes) * time.Minute),
		BedID:   ac.Tmp.ResolvedIntent.BedID,
		Quality: ac.Tmp.ResolvedIntent.BedQuality,
	}
	next.Version++
	next.UpdatedAt = ac.In.NowAt
	event := survival.DomainEvent{
		Type:       "sleep_started",
		OccurredAt: ac.In.NowAt,
		Payload: map[string]any{
			"agent_id":                  ac.In.AgentID,
			"session_id":                ac.In.SessionID,
			"sleep_minutes":             sleepMinutes,
			"bed_id":                    ac.Tmp.ResolvedIntent.BedID,
			"bed_quality":               strings.ToUpper(strings.TrimSpace(ac.Tmp.ResolvedIntent.BedQuality)),
			"end_at":                    next.OngoingAction.EndAt,
			"world_time_before_seconds": ac.View.Snapshot.WorldTimeSeconds,
			"world_time_after_seconds":  ac.View.Snapshot.WorldTimeSeconds,
		},
	}
	if ac.In.Req.StrategyHash != "" {
		event.Payload["strategy_hash"] = ac.In.Req.StrategyHash
	}
	ac.Plan.StateToSave = &next
	ac.Plan.StateVersion = ac.View.StateWorking.Version
	ac.Plan.EventsToAppend = []survival.DomainEvent{event}
	ac.Plan.ExecutionToSave = &portsActionExecutionRecord{
		AgentID:        ac.In.AgentID,
		IdempotencyKey: ac.In.IdempotencyKey,
		IntentType:     string(ac.Tmp.ResolvedIntent.Type),
		Result: actionResult{
			UpdatedState: next,
			Events:       []survival.DomainEvent{event},
			ResultCode:   survival.ResultOK,
		},
		AppliedAt: ac.In.NowAt,
	}
	ac.Plan.ResultCode = survival.ResultOK
	ac.Plan.ShouldPersist = true
	ac.Tmp.Completed = true
	ac.Tmp.Response = Response{
		WorldTimeBeforeSeconds: ac.View.Snapshot.WorldTimeSeconds,
		WorldTimeAfterSeconds:  ac.View.Snapshot.WorldTimeSeconds,
		UpdatedState:           next,
		Events:                 []survival.DomainEvent{event},
		Settlement:             settlementSummary([]survival.DomainEvent{event}),
		ResultCode:             survival.ResultOK,
	}
	return ExecuteModeCompleted, nil
}
