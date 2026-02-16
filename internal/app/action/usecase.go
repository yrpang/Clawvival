package action

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

var (
	ErrInvalidRequest = errors.New("invalid action request")
)

type UseCase struct {
	TxManager  ports.TxManager
	StateRepo  ports.AgentStateRepository
	ActionRepo ports.ActionExecutionRepository
	EventRepo  ports.EventRepository
	World      ports.WorldProvider
	Settle     survival.SettlementService
	Now        func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" || strings.TrimSpace(req.IdempotencyKey) == "" || req.DeltaMinutes <= 0 {
		return Response{}, ErrInvalidRequest
	}

	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	var out Response
	err := u.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		exec, err := u.ActionRepo.GetByIdempotencyKey(txCtx, req.AgentID, req.IdempotencyKey)
		if err == nil && exec != nil {
			out = Response{
				UpdatedState: exec.Result.UpdatedState,
				Events:       exec.Result.Events,
				ResultCode:   exec.Result.ResultCode,
			}
			return nil
		}
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return err
		}

		state, err := u.StateRepo.GetByAgentID(txCtx, req.AgentID)
		if err != nil {
			return err
		}

		snapshot, err := u.World.SnapshotForAgent(txCtx, req.AgentID)
		if err != nil {
			return err
		}

		result, err := u.Settle.Settle(
			state,
			req.Intent,
			survival.HeartbeatDelta{Minutes: req.DeltaMinutes},
			nowFn(),
			survival.WorldSnapshot{
				TimeOfDay:      snapshot.TimeOfDay,
				ThreatLevel:    snapshot.ThreatLevel,
				NearbyResource: snapshot.NearbyResource,
			},
		)
		if err != nil {
			return err
		}

		if err := u.StateRepo.SaveWithVersion(txCtx, result.UpdatedState, state.Version); err != nil {
			return err
		}

		execution := ports.ActionExecutionRecord{
			AgentID:        req.AgentID,
			IdempotencyKey: req.IdempotencyKey,
			Result: ports.ActionResult{
				UpdatedState: result.UpdatedState,
				Events:       result.Events,
				ResultCode:   result.ResultCode,
			},
			AppliedAt: nowFn(),
		}
		if err := u.ActionRepo.SaveExecution(txCtx, execution); err != nil {
			return err
		}

		if err := u.EventRepo.Append(txCtx, result.Events); err != nil {
			return err
		}

		out = Response{
			UpdatedState: result.UpdatedState,
			Events:       result.Events,
			ResultCode:   result.ResultCode,
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}

	return out, nil
}
