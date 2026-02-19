package action

import (
	"context"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type ActionMode int

const (
	ActionModeSettle ActionMode = iota
	ActionModeStartOngoing
	ActionModeFinalizeOnly
)

type ExecuteMode int

const (
	ExecuteModeContinue ExecuteMode = iota
	ExecuteModeCompleted
)

type ActionSpec struct {
	Type         survival.ActionType
	Mode         ActionMode
	CanTerminate bool
	Handler      ActionHandler
}

type ActionHandler interface {
	Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error
	ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error)
}

type BaseHandler struct{}

func (BaseHandler) Precheck(context.Context, UseCase, *ActionContext) error { return nil }
func (BaseHandler) ExecuteActionAndPlan(context.Context, UseCase, *ActionContext) (ExecuteMode, error) {
	return ExecuteModeContinue, nil
}

type ActionInput struct {
	Req            Request
	NowAt          time.Time
	AgentID        string
	IdempotencyKey string
	SessionID      string
}

type ActionView struct {
	Spec         ActionSpec
	StateBefore  survival.AgentStateAggregate
	StateWorking survival.AgentStateAggregate
	EventsBefore []survival.DomainEvent
	Snapshot     world.Snapshot
	PreparedObj  *preparedObjectAction
	Finalized    ongoingFinalizeResult
}

type ActionWritePlan struct {
	StateToSave     *survival.AgentStateAggregate
	StateVersion    int64
	EventsToAppend  []survival.DomainEvent
	ExecutionToSave *ports.ActionExecutionRecord
	ResultCode      survival.ResultCode
	ShouldPersist   bool

	ApplyGatherDepletion bool
	ApplyObjectAction    bool
	CreateBuiltObjects   bool
	CloseSession         bool
	CloseSessionCause    survival.DeathCause
}

type ActionTmp struct {
	ResolvedIntent survival.ActionIntent
	DeltaMinutes   int
	SettleResult   survival.SettlementResult
	Completed      bool
	Response       Response
}

type ActionContext struct {
	In   ActionInput
	View ActionView
	Plan ActionWritePlan
	Tmp  ActionTmp
}

func actionRegistry() map[survival.ActionType]ActionSpec {
	return map[survival.ActionType]ActionSpec{
		survival.ActionGather:            {Type: survival.ActionGather, Mode: ActionModeSettle, Handler: gatherActionHandler{}},
		survival.ActionRest:              {Type: survival.ActionRest, Mode: ActionModeStartOngoing, CanTerminate: true, Handler: restActionHandler{}},
		survival.ActionSleep:             {Type: survival.ActionSleep, Mode: ActionModeSettle, Handler: sleepActionHandler{}},
		survival.ActionMove:              {Type: survival.ActionMove, Mode: ActionModeSettle, Handler: moveActionHandler{}},
		survival.ActionBuild:             {Type: survival.ActionBuild, Mode: ActionModeSettle, Handler: buildActionHandler{}},
		survival.ActionFarmPlant:         {Type: survival.ActionFarmPlant, Mode: ActionModeSettle, Handler: farmPlantActionHandler{}},
		survival.ActionFarmHarvest:       {Type: survival.ActionFarmHarvest, Mode: ActionModeSettle, Handler: farmHarvestActionHandler{}},
		survival.ActionContainerDeposit:  {Type: survival.ActionContainerDeposit, Mode: ActionModeSettle, Handler: containerDepositActionHandler{}},
		survival.ActionContainerWithdraw: {Type: survival.ActionContainerWithdraw, Mode: ActionModeSettle, Handler: containerWithdrawActionHandler{}},
		survival.ActionRetreat:           {Type: survival.ActionRetreat, Mode: ActionModeSettle, Handler: retreatActionHandler{}},
		survival.ActionCraft:             {Type: survival.ActionCraft, Mode: ActionModeSettle, Handler: craftActionHandler{}},
		survival.ActionEat:               {Type: survival.ActionEat, Mode: ActionModeSettle, Handler: eatActionHandler{}},
		survival.ActionTerminate:         {Type: survival.ActionTerminate, Mode: ActionModeFinalizeOnly, Handler: terminateActionHandler{}},
	}
}

func supportedActionTypes() []survival.ActionType {
	return []survival.ActionType{
		survival.ActionGather,
		survival.ActionRest,
		survival.ActionSleep,
		survival.ActionMove,
		survival.ActionBuild,
		survival.ActionFarmPlant,
		survival.ActionFarmHarvest,
		survival.ActionContainerDeposit,
		survival.ActionContainerWithdraw,
		survival.ActionRetreat,
		survival.ActionCraft,
		survival.ActionEat,
		survival.ActionTerminate,
	}
}

func isSupportedActionType(t survival.ActionType) bool {
	for _, actionType := range supportedActionTypes() {
		if t == actionType {
			return true
		}
	}
	return false
}

func actionParamValidators() map[survival.ActionType]func(survival.ActionIntent) bool {
	return map[survival.ActionType]func(survival.ActionIntent) bool{
		survival.ActionGather:            validateGatherActionParams,
		survival.ActionRest:              validateRestActionParams,
		survival.ActionSleep:             validateSleepActionParams,
		survival.ActionMove:              validateMoveActionParams,
		survival.ActionBuild:             validateBuildActionParams,
		survival.ActionFarmPlant:         validateFarmPlantActionParams,
		survival.ActionFarmHarvest:       validateFarmHarvestActionParams,
		survival.ActionContainerDeposit:  validateContainerActionParams,
		survival.ActionContainerWithdraw: validateContainerActionParams,
		survival.ActionRetreat:           validateRetreatActionParams,
		survival.ActionCraft:             validateCraftActionParams,
		survival.ActionEat:               validateEatActionParams,
		survival.ActionTerminate:         validateTerminateActionParams,
	}
}
