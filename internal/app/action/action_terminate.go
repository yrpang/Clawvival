package action

import (
	"context"

	"clawvival/internal/domain/survival"
)

type terminateActionHandler struct{ BaseHandler }

func validateTerminateActionParams(survival.ActionIntent) bool {
	return true
}

func (h terminateActionHandler) Precheck(context.Context, UseCase, *ActionContext) error {
	return nil
}

func (h terminateActionHandler) ExecuteActionAndPlan(_ context.Context, _ UseCase, ac *ActionContext) (ExecuteMode, error) {
	if !ac.View.Finalized.Settled {
		return ExecuteModeContinue, ErrActionPreconditionFailed
	}
	ac.Plan.ExecutionToSave = &portsActionExecutionRecord{
		AgentID:        ac.In.AgentID,
		IdempotencyKey: ac.In.IdempotencyKey,
		IntentType:     string(ac.Tmp.ResolvedIntent.Type),
		Result: actionResult{
			UpdatedState: ac.View.Finalized.UpdatedState,
			Events:       ac.View.Finalized.Events,
			ResultCode:   ac.View.Finalized.ResultCode,
		},
		AppliedAt: ac.In.NowAt,
	}
	ac.Plan.ResultCode = ac.View.Finalized.ResultCode
	ac.Plan.ShouldPersist = true
	ac.Tmp.Completed = true
	ac.Tmp.Response = Response{
		WorldTimeBeforeSeconds: ac.View.Finalized.WorldTimeBeforeSeconds,
		WorldTimeAfterSeconds:  ac.View.Finalized.WorldTimeAfterSeconds,
		UpdatedState:           ac.View.Finalized.UpdatedState,
		Events:                 ac.View.Finalized.Events,
		Settlement:             settlementSummary(ac.View.Finalized.Events),
		ResultCode:             ac.View.Finalized.ResultCode,
	}
	return ExecuteModeCompleted, nil
}
