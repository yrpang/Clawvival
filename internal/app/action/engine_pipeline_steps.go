package action

import (
	"context"
	"errors"
	"strings"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func (u UseCase) ValidateRequest(req Request) (ActionContext, error) {
	req.AgentID = normalizeAgentID(req.AgentID)
	req.IdempotencyKey = normalizeIdempotencyKey(req.IdempotencyKey)
	req.Intent = normalizeValidatedIntent(req.Intent)

	if req.AgentID == "" || req.IdempotencyKey == "" || !isSupportedActionType(req.Intent.Type) {
		return ActionContext{}, ErrInvalidRequest
	}
	if !hasValidActionParams(req.Intent) {
		return ActionContext{}, ErrInvalidActionParams
	}

	return ActionContext{
		In: ActionInput{
			Req:            req,
			AgentID:        req.AgentID,
			IdempotencyKey: req.IdempotencyKey,
			SessionID:      "session-" + req.AgentID,
		},
		Tmp: ActionTmp{ResolvedIntent: req.Intent},
	}, nil
}

func (u UseCase) ReplayIdempotent(ctx context.Context, ac *ActionContext) (Response, bool, error) {
	exec, err := u.ActionRepo.GetByIdempotencyKey(ctx, ac.In.AgentID, ac.In.IdempotencyKey)
	if err == nil && exec != nil {
		before, after := worldTimeWindowFromExecution(exec)
		updatedState := exec.Result.UpdatedState
		if strings.TrimSpace(updatedState.SessionID) == "" {
			updatedState.SessionID = ac.In.SessionID
		}
		return Response{
			SettledDTMinutes:       exec.DT,
			WorldTimeBeforeSeconds: before,
			WorldTimeAfterSeconds:  after,
			UpdatedState:           updatedState,
			Events:                 exec.Result.Events,
			Settlement:             settlementSummary(exec.Result.Events),
			ResultCode:             exec.Result.ResultCode,
		}, true, nil
	}
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return Response{}, false, err
	}
	return Response{}, false, nil
}

func (u UseCase) LoadStateAndFinalizeOngoing(ctx context.Context, ac *ActionContext) error {
	state, err := u.StateRepo.GetByAgentID(ctx, ac.In.AgentID)
	if err != nil {
		return err
	}
	state.SessionID = ac.In.SessionID
	ac.View.StateBefore = state
	ac.View.StateWorking = state

	finalized, err := finalizeOngoingAction(ctx, u, ac.In.AgentID, state, ac.In.NowAt, ac.Tmp.ResolvedIntent.Type == survival.ActionTerminate)
	if err != nil {
		return err
	}
	ac.View.Finalized = finalized
	if finalized.Settled {
		ac.View.StateWorking = finalized.UpdatedState
	}
	return nil
}

func (u UseCase) ResolveSpec(ac *ActionContext) error {
	spec, ok := actionRegistry()[ac.Tmp.ResolvedIntent.Type]
	if !ok {
		return ErrInvalidRequest
	}
	ac.View.Spec = spec
	return nil
}

func (u UseCase) BuildContext(ctx context.Context, ac *ActionContext) error {
	if ac.Tmp.ResolvedIntent.Type == survival.ActionTerminate {
		return nil
	}
	if ac.View.StateWorking.OngoingAction != nil {
		return ErrActionInProgress
	}

	snapshot, err := u.World.SnapshotForAgent(ctx, ac.In.AgentID, world.Point{
		X: ac.View.StateWorking.Position.X,
		Y: ac.View.StateWorking.Position.Y,
	})
	if err != nil {
		return err
	}
	ac.View.Snapshot = snapshot

	preparedObj, err := prepareObjectAction(ctx, ac.In.NowAt, ac.View.StateWorking, ac.Tmp.ResolvedIntent, u.ObjectRepo, ac.In.AgentID)
	if err != nil {
		return err
	}
	ac.View.PreparedObj = preparedObj
	if ac.Tmp.ResolvedIntent.Type == survival.ActionSleep && preparedObj != nil {
		ac.Tmp.ResolvedIntent.BedQuality = preparedObj.record.Quality
	}

	resolvedMoveIntent, moveErr := resolveMoveIntent(ac.View.StateWorking, ac.Tmp.ResolvedIntent, snapshot)
	if moveErr != nil {
		return moveErr
	}
	ac.Tmp.ResolvedIntent = resolveRetreatIntent(resolvedMoveIntent, ac.View.StateWorking.Position, snapshot.VisibleTiles)

	if ac.Tmp.ResolvedIntent.Type != survival.ActionRest {
		eventsBeforeAction, err := listRecentEvents(ctx, u.EventRepo, ac.In.AgentID)
		if err != nil {
			return err
		}
		ac.View.EventsBefore = eventsBeforeAction
	}

	return nil
}

func (u UseCase) RunPrechecks(ctx context.Context, ac *ActionContext) error {
	if ac.View.Spec.Handler != nil {
		return ac.View.Spec.Handler.Precheck(ctx, u, ac)
	}
	return nil
}

func (u UseCase) ExecuteActionAndPlan(ctx context.Context, ac *ActionContext) (ExecuteMode, error) {
	if ac.View.Spec.Handler == nil {
		return ExecuteModeContinue, nil
	}
	return ac.View.Spec.Handler.ExecuteActionAndPlan(ctx, u, ac)
}

func (u UseCase) PersistAndRespond(ctx context.Context, ac *ActionContext) error {
	if !ac.Plan.ShouldPersist {
		return nil
	}

	if ac.Plan.ApplyGatherDepletion {
		if err := persistGatherDepletion(ctx, u.ResourceRepo, ac.In.AgentID, ac.Tmp.ResolvedIntent, ac.In.NowAt); err != nil {
			return err
		}
	}
	if ac.Plan.StateToSave != nil {
		if err := u.StateRepo.SaveWithVersion(ctx, *ac.Plan.StateToSave, ac.Plan.StateVersion); err != nil {
			return err
		}
	}

	for i := range ac.Plan.EventsToAppend {
		if ac.Plan.EventsToAppend[i].Payload == nil {
			ac.Plan.EventsToAppend[i].Payload = map[string]any{}
		}
		ac.Plan.EventsToAppend[i].Payload["agent_id"] = ac.In.AgentID
		ac.Plan.EventsToAppend[i].Payload["session_id"] = ac.In.SessionID
		if ac.In.Req.StrategyHash != "" {
			ac.Plan.EventsToAppend[i].Payload["strategy_hash"] = ac.In.Req.StrategyHash
		}
	}

	if ac.Plan.ExecutionToSave != nil {
		exec := *ac.Plan.ExecutionToSave
		if ac.Plan.EventsToAppend != nil {
			exec.Result.Events = ac.Plan.EventsToAppend
		}
		if err := u.ActionRepo.SaveExecution(ctx, exec); err != nil {
			return err
		}
	}

	if ac.Plan.ApplyObjectAction {
		if err := persistObjectAction(ctx, ac.In.NowAt, ac.Tmp.ResolvedIntent, ac.View.PreparedObj, u.ObjectRepo, ac.In.AgentID); err != nil {
			return err
		}
	}

	if len(ac.Plan.EventsToAppend) > 0 {
		if err := u.EventRepo.Append(ctx, ac.In.AgentID, ac.Plan.EventsToAppend); err != nil {
			return err
		}
	}

	builtObjectIDs := make([]string, 0, 1)
	if ac.Plan.CreateBuiltObjects && u.ObjectRepo != nil {
		for _, evt := range ac.Plan.EventsToAppend {
			if evt.Type != "build_completed" || evt.Payload == nil {
				continue
			}
			objectID := "obj-" + ac.In.AgentID + "-" + ac.In.IdempotencyKey
			obj := ports.WorldObjectRecord{
				ObjectID: objectID,
				Kind:     int(toNum(evt.Payload["kind"])),
				X:        int(toNum(evt.Payload["x"])),
				Y:        int(toNum(evt.Payload["y"])),
				HP:       int(toNum(evt.Payload["hp"])),
			}
			if obj.HP <= 0 {
				obj.HP = 100
			}
			obj.ObjectType, obj.Quality, obj.CapacitySlots, obj.ObjectState = buildObjectDefaults(ac.Tmp.ResolvedIntent.ObjectType)
			if err := u.ObjectRepo.Save(ctx, ac.In.AgentID, obj); err != nil {
				return err
			}
			builtObjectIDs = append(builtObjectIDs, objectID)
		}
	}
	attachBuiltObjectIDs(ac.Plan.EventsToAppend, builtObjectIDs)

	if ac.Plan.CloseSession && u.SessionRepo != nil {
		if err := u.SessionRepo.Close(ctx, ac.In.SessionID, ac.Plan.CloseSessionCause, ac.In.NowAt); err != nil {
			return err
		}
	}
	return nil
}

func (u UseCase) BuildCompletedResponse(ac *ActionContext) Response {
	out := ac.Tmp.Response
	if ac.Plan.EventsToAppend != nil {
		out.Events = ac.Plan.EventsToAppend
		out.Settlement = settlementSummary(out.Events)
	}
	return out
}

func (u UseCase) BuildSettledResponse(ac *ActionContext) Response {
	out := ac.Tmp.Response
	out.Events = ac.Plan.EventsToAppend
	out.Settlement = settlementSummary(out.Events)
	return out
}

func normalizeAgentID(in string) string {
	return strings.TrimSpace(in)
}

func normalizeIdempotencyKey(in string) string {
	return strings.TrimSpace(in)
}

func normalizeValidatedIntent(in survival.ActionIntent) survival.ActionIntent {
	out := in
	out.Type = survival.ActionType(strings.TrimSpace(string(out.Type)))
	return normalizeIntent(out)
}

func hasValidActionParams(intent survival.ActionIntent) bool {
	validator, ok := actionParamValidators()[intent.Type]
	if !ok {
		return true
	}
	return validator(intent)
}

func normalizeIntent(in survival.ActionIntent) survival.ActionIntent {
	out := in
	out.Direction = strings.ToUpper(strings.TrimSpace(out.Direction))
	switch out.Direction {
	case "N":
		out.DX, out.DY = 0, 1
	case "S":
		out.DX, out.DY = 0, -1
	case "E":
		out.DX, out.DY = 1, 0
	case "W":
		out.DX, out.DY = -1, 0
	}
	if out.Count <= 0 {
		out.Count = 1
	}
	return out
}
