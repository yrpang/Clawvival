package action

import (
	"context"
	"strings"

	"clawvival/internal/domain/survival"
)

type craftActionHandler struct{ BaseHandler }
type eatActionHandler struct{ BaseHandler }

func (h craftActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if err := runStandardActionPrecheck(ctx, uc, ac); err != nil {
		return err
	}
	if !survival.CanCraft(ac.View.StateWorking, survival.RecipeID(ac.Tmp.ResolvedIntent.RecipeID)) {
		return ErrActionPreconditionFailed
	}
	return nil
}

func (h craftActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func (h eatActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if err := runStandardActionPrecheck(ctx, uc, ac); err != nil {
		return err
	}
	if !eatPreconditionsSatisfied(ac.View.StateWorking, ac.Tmp.ResolvedIntent) {
		return ErrActionPreconditionFailed
	}
	return nil
}

func (h eatActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func eatPreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent) bool {
	if intent.Type != survival.ActionEat {
		return true
	}
	foodID, ok := foodIDFromItemType(intent.ItemType)
	if !ok || !survival.CanEat(state, foodID) {
		return false
	}
	count := intent.Count
	if count <= 0 {
		count = 1
	}
	return state.Inventory[strings.ToLower(strings.TrimSpace(intent.ItemType))] >= count
}

func foodIDFromItemType(itemType string) (survival.FoodID, bool) {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "berry":
		return survival.FoodBerry, true
	case "bread":
		return survival.FoodBread, true
	case "wheat":
		return survival.FoodWheat, true
	default:
		return 0, false
	}
}
