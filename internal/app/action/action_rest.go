package action

import (
	"context"
	"time"

	"clawvival/internal/domain/survival"
)

type restActionHandler struct{ BaseHandler }

func validateRestActionParams(intent survival.ActionIntent) bool {
	restMinutes := intent.RestMinutes
	return restMinutes >= survival.MinRestMinutes && restMinutes <= survival.MaxRestMinutes
}

func (h restActionHandler) Precheck(context.Context, UseCase, *ActionContext) error {
	return nil
}

func (h restActionHandler) ExecuteActionAndPlan(_ context.Context, _ UseCase, ac *ActionContext) (ExecuteMode, error) {
	next := ac.View.StateWorking
	restMinutes := ac.Tmp.ResolvedIntent.RestMinutes
	next.OngoingAction = &survival.OngoingActionInfo{
		Type:    survival.ActionRest,
		Minutes: restMinutes,
		EndAt:   ac.In.NowAt.Add(time.Duration(restMinutes) * time.Minute),
	}
	next.Version++
	next.UpdatedAt = ac.In.NowAt
	event := survival.DomainEvent{
		Type:       "rest_started",
		OccurredAt: ac.In.NowAt,
		Payload: map[string]any{
			"agent_id":                  ac.In.AgentID,
			"session_id":                ac.In.SessionID,
			"rest_minutes":              restMinutes,
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
		DT:             0,
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
		SettledDTMinutes:       0,
		WorldTimeBeforeSeconds: ac.View.Snapshot.WorldTimeSeconds,
		WorldTimeAfterSeconds:  ac.View.Snapshot.WorldTimeSeconds,
		UpdatedState:           next,
		Events:                 []survival.DomainEvent{event},
		Settlement:             settlementSummary([]survival.DomainEvent{event}),
		ResultCode:             survival.ResultOK,
	}
	return ExecuteModeCompleted, nil
}
