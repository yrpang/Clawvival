package action

import (
	"context"
	"strings"

	"clawvival/internal/domain/survival"
)

type sleepActionHandler struct{ BaseHandler }

func validateSleepActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.BedID) != ""
}

func (h sleepActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h sleepActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}
