package action

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
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
	Metrics    ports.ActionMetrics
	Settle     survival.SettlementService
	Now        func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Intent.Type = survival.ActionType(strings.TrimSpace(string(req.Intent.Type)))
	if req.AgentID == "" || req.IdempotencyKey == "" || req.DeltaMinutes <= 0 || !isSupportedActionType(req.Intent.Type) || !hasValidActionParams(req.Intent) {
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

		snapshot, err := u.World.SnapshotForAgent(txCtx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
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

		for i := range result.Events {
			if result.Events[i].Payload == nil {
				result.Events[i].Payload = map[string]any{}
			}
			result.Events[i].Payload["agent_id"] = req.AgentID
			if req.StrategyHash != "" {
				result.Events[i].Payload["strategy_hash"] = req.StrategyHash
			}
		}

		execution := ports.ActionExecutionRecord{
			AgentID:        req.AgentID,
			IdempotencyKey: req.IdempotencyKey,
			IntentType:     string(req.Intent.Type),
			DT:             req.DeltaMinutes,
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

		if err := u.EventRepo.Append(txCtx, req.AgentID, result.Events); err != nil {
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

func isSupportedActionType(t survival.ActionType) bool {
	switch t {
	case survival.ActionGather, survival.ActionRest, survival.ActionMove:
		return true
	case survival.ActionCombat, survival.ActionBuild, survival.ActionFarm, survival.ActionRetreat, survival.ActionCraft:
		return true
	default:
		return false
	}
}

func hasValidActionParams(intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionMove:
		return intent.Params["dx"] != 0 || intent.Params["dy"] != 0
	case survival.ActionCombat:
		return intent.Params["target_level"] > 0
	case survival.ActionBuild:
		return intent.Params["kind"] > 0
	case survival.ActionFarm:
		return intent.Params["seed"] > 0
	case survival.ActionCraft:
		return intent.Params["recipe"] > 0
	default:
		return true
	}
}
