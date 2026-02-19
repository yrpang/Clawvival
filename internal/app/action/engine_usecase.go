package action

import (
	"context"
	"errors"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

var (
	ErrInvalidRequest           = errors.New("invalid action request")
	ErrInvalidActionParams      = errors.New("invalid action params")
	ErrActionPreconditionFailed = errors.New("action precondition failed")
	ErrActionInvalidPosition    = errors.New("action invalid position")
	ErrActionCooldownActive     = errors.New("action cooldown active")
	ErrActionInProgress         = errors.New("action in progress")
	ErrTargetOutOfView          = errors.New("target out of view")
	ErrTargetNotVisible         = errors.New("target not visible")
	ErrResourceDepleted         = errors.New("resource depleted")
	ErrInventoryFull            = errors.New("inventory full")
	ErrContainerFull            = errors.New("container full")
)

type ResourceDepletedError struct {
	TargetID         string
	RemainingSeconds int
}

func (e *ResourceDepletedError) Error() string {
	return ErrResourceDepleted.Error()
}

func (e *ResourceDepletedError) Unwrap() error {
	return ErrResourceDepleted
}

type ActionCooldownActiveError struct {
	IntentType       survival.ActionType
	RemainingSeconds int
}

func (e *ActionCooldownActiveError) Error() string {
	return ErrActionCooldownActive.Error()
}

func (e *ActionCooldownActiveError) Unwrap() error {
	return ErrActionCooldownActive
}

type ActionInvalidPositionError struct {
	TargetPos       *survival.Position
	BlockingTilePos *survival.Position
}

func (e *ActionInvalidPositionError) Error() string {
	return ErrActionInvalidPosition.Error()
}

func (e *ActionInvalidPositionError) Unwrap() error {
	return ErrActionInvalidPosition
}

type UseCase struct {
	TxManager    ports.TxManager
	StateRepo    ports.AgentStateRepository
	ActionRepo   ports.ActionExecutionRepository
	EventRepo    ports.EventRepository
	ObjectRepo   ports.WorldObjectRepository
	ResourceRepo ports.AgentResourceNodeRepository
	SessionRepo  ports.AgentSessionRepository
	World        ports.WorldProvider
	Metrics      ports.ActionMetrics
	Settle       survival.SettlementService
	Now          func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	ac, err := u.ValidateRequest(req)
	if err != nil {
		return Response{}, err
	}

	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	ac.In.NowAt = nowFn()

	var out Response
	err = u.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		replay, ok, err := u.ReplayIdempotent(txCtx, &ac)
		if err != nil {
			return err
		}
		if ok {
			out = replay
			return nil
		}
		if err := u.LoadStateAndFinalizeOngoing(txCtx, &ac); err != nil {
			return err
		}
		if err := u.ResolveSpec(&ac); err != nil {
			return err
		}
		if err := u.BuildContext(txCtx, &ac); err != nil {
			return err
		}
		if err := u.RunPrechecks(txCtx, &ac); err != nil {
			return err
		}
		mode, err := u.ExecuteActionAndPlan(txCtx, &ac)
		if err != nil {
			return err
		}
		if err := u.PersistAndRespond(txCtx, &ac); err != nil {
			return err
		}
		if mode == ExecuteModeCompleted {
			out = u.BuildCompletedResponse(&ac)
			return nil
		}
		out = u.BuildSettledResponse(&ac)
		return nil
	})
	if err != nil {
		if u.Metrics != nil {
			if errors.Is(err, ports.ErrConflict) {
				u.Metrics.RecordConflict()
			} else {
				u.Metrics.RecordFailure()
			}
		}
		return Response{}, err
	}
	if u.Metrics != nil {
		u.Metrics.RecordSuccess(out.ResultCode)
	}

	return out, nil
}
