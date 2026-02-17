package action

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

var (
	ErrInvalidRequest           = errors.New("invalid action request")
	ErrInvalidActionParams      = errors.New("invalid action params")
	ErrActionPreconditionFailed = errors.New("action precondition failed")
	ErrActionInvalidPosition    = errors.New("action invalid position")
	ErrActionCooldownActive     = errors.New("action cooldown active")
	ErrActionInProgress         = errors.New("action in progress")
)

const (
	defaultHeartbeatDeltaMinutes = 30
	minHeartbeatDeltaMinutes     = 1
	maxHeartbeatDeltaMinutes     = 120
	minRestMinutes               = 1
	maxRestMinutes               = 120
)

type UseCase struct {
	TxManager   ports.TxManager
	StateRepo   ports.AgentStateRepository
	ActionRepo  ports.ActionExecutionRepository
	EventRepo   ports.EventRepository
	ObjectRepo  ports.WorldObjectRepository
	SessionRepo ports.AgentSessionRepository
	World       ports.WorldProvider
	Metrics     ports.ActionMetrics
	Settle      survival.SettlementService
	Now         func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Intent.Type = survival.ActionType(strings.TrimSpace(string(req.Intent.Type)))
	if req.AgentID == "" || req.IdempotencyKey == "" || !isSupportedActionType(req.Intent.Type) {
		return Response{}, ErrInvalidRequest
	}
	if !hasValidActionParams(req.Intent) {
		return Response{}, ErrInvalidActionParams
	}

	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	var out Response
	err := u.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
		exec, err := u.ActionRepo.GetByIdempotencyKey(txCtx, req.AgentID, req.IdempotencyKey)
		if err == nil && exec != nil {
			before, after := worldTimeWindow(0, exec.DT)
			out = Response{
				SettledDTMinutes:       exec.DT,
				WorldTimeBeforeSeconds: before,
				WorldTimeAfterSeconds:  after,
				UpdatedState:           exec.Result.UpdatedState,
				Events:                 exec.Result.Events,
				ResultCode:             exec.Result.ResultCode,
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
		nowAt := nowFn()
		finalized, err := finalizeOngoingAction(txCtx, u, req.AgentID, state, nowAt, req.Intent.Type == survival.ActionTerminate)
		if err != nil {
			return err
		}
		if finalized.Settled {
			state = finalized.UpdatedState
		}
		if req.Intent.Type == survival.ActionTerminate {
			if !finalized.Settled {
				return ErrActionPreconditionFailed
			}
			if err := saveActionExecution(txCtx, u.ActionRepo, req.AgentID, req.IdempotencyKey, req.Intent.Type, finalized.DTMinutes, ports.ActionResult{
				UpdatedState: finalized.UpdatedState,
				Events:       finalized.Events,
				ResultCode:   finalized.ResultCode,
			}, nowAt); err != nil {
				return err
			}
			out = Response{
				SettledDTMinutes:       finalized.DTMinutes,
				WorldTimeBeforeSeconds: 0,
				WorldTimeAfterSeconds:  int64(finalized.DTMinutes * 60),
				UpdatedState:           finalized.UpdatedState,
				Events:                 finalized.Events,
				ResultCode:             finalized.ResultCode,
			}
			return nil
		}
		if state.OngoingAction != nil {
			return ErrActionInProgress
		}
		if req.Intent.Type == survival.ActionRest {
			out, err = startRestAction(txCtx, u, req, state, nowAt)
			return err
		}
		if !resourcePreconditionsSatisfied(state, req.Intent) {
			return ErrActionPreconditionFailed
		}
		sessionID := "session-" + req.AgentID
		if u.SessionRepo != nil {
			if err := u.SessionRepo.EnsureActive(txCtx, sessionID, req.AgentID, state.Version); err != nil {
				return err
			}
		}

		snapshot, err := u.World.SnapshotForAgent(txCtx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
		if err != nil {
			return err
		}
		if !positionPreconditionsSatisfied(state, req.Intent, snapshot) {
			return ErrActionInvalidPosition
		}
		if err := ensureCooldownReady(txCtx, u.EventRepo, req.AgentID, req.Intent.Type, nowAt); err != nil {
			return err
		}
		deltaMinutes, err := resolveHeartbeatDeltaMinutes(txCtx, u.EventRepo, req.AgentID, nowAt)
		if err != nil {
			return err
		}

		result, err := u.Settle.Settle(
			state,
			req.Intent,
			survival.HeartbeatDelta{Minutes: deltaMinutes},
			nowAt,
			survival.WorldSnapshot{
				TimeOfDay:         snapshot.TimeOfDay,
				ThreatLevel:       snapshot.ThreatLevel,
				VisibilityPenalty: snapshot.VisibilityPenalty,
				NearbyResource:    snapshot.NearbyResource,
			},
		)
		if err != nil {
			return err
		}
		if snapshot.PhaseChanged {
			result.Events = append(result.Events, survival.DomainEvent{
				Type:       "world_phase_changed",
				OccurredAt: nowAt,
				Payload: map[string]any{
					"from": snapshot.PhaseFrom,
					"to":   snapshot.PhaseTo,
				},
			})
		}

		if err := u.StateRepo.SaveWithVersion(txCtx, result.UpdatedState, state.Version); err != nil {
			return err
		}

		for i := range result.Events {
			if result.Events[i].Payload == nil {
				result.Events[i].Payload = map[string]any{}
			}
			result.Events[i].Payload["agent_id"] = req.AgentID
			result.Events[i].Payload["session_id"] = sessionID
			if req.StrategyHash != "" {
				result.Events[i].Payload["strategy_hash"] = req.StrategyHash
			}
		}

		if err := saveActionExecution(txCtx, u.ActionRepo, req.AgentID, req.IdempotencyKey, req.Intent.Type, deltaMinutes, ports.ActionResult{
			UpdatedState: result.UpdatedState,
			Events:       result.Events,
			ResultCode:   result.ResultCode,
		}, nowAt); err != nil {
			return err
		}

		if err := u.EventRepo.Append(txCtx, req.AgentID, result.Events); err != nil {
			return err
		}
		if u.ObjectRepo != nil {
			for _, evt := range result.Events {
				if evt.Type != "build_completed" || evt.Payload == nil {
					continue
				}
				obj := ports.WorldObjectRecord{
					ObjectID: "obj-" + req.AgentID + "-" + req.IdempotencyKey,
					Kind:     int(toNum(evt.Payload["kind"])),
					X:        int(toNum(evt.Payload["x"])),
					Y:        int(toNum(evt.Payload["y"])),
					HP:       int(toNum(evt.Payload["hp"])),
				}
				if obj.HP <= 0 {
					obj.HP = 100
				}
				if err := u.ObjectRepo.Save(txCtx, req.AgentID, obj); err != nil {
					return err
				}
			}
		}
		if u.SessionRepo != nil && result.ResultCode == survival.ResultGameOver {
			if err := u.SessionRepo.Close(txCtx, sessionID, result.UpdatedState.DeathCause, nowAt); err != nil {
				return err
			}
		}

		before, after := worldTimeWindow(snapshot.WorldTimeSeconds, deltaMinutes)
		out = Response{
			SettledDTMinutes:       deltaMinutes,
			WorldTimeBeforeSeconds: before,
			WorldTimeAfterSeconds:  after,
			UpdatedState:           result.UpdatedState,
			Events:                 result.Events,
			ResultCode:             result.ResultCode,
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

func worldTimeWindow(beforeSeconds int64, dtMinutes int) (int64, int64) {
	return beforeSeconds, beforeSeconds + int64(dtMinutes*60)
}

func resourcePreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionBuild:
		return survival.CanBuild(state, survival.BuildKind(intent.Params["kind"]))
	case survival.ActionCraft:
		return survival.CanCraft(state, survival.RecipeID(intent.Params["recipe"]))
	case survival.ActionFarm, survival.ActionFarmPlant:
		if intent.Params["seed"] > 0 {
			return survival.CanPlantSeed(state)
		}
		return survival.CanPlantSeed(state)
	case survival.ActionEat:
		return survival.CanEat(state, survival.FoodID(intent.Params["food"]))
	default:
		return true
	}
}

func positionPreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent, snapshot world.Snapshot) bool {
	if intent.Type != survival.ActionMove {
		return true
	}
	dx := intent.Params["dx"]
	dy := intent.Params["dy"]
	if abs(dx) > 1 || abs(dy) > 1 {
		return false
	}
	targetX := state.Position.X + dx
	targetY := state.Position.Y + dy
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == targetX && tile.Y == targetY {
			return tile.Passable
		}
	}
	return false
}

var actionCooldowns = map[survival.ActionType]time.Duration{
	survival.ActionBuild:     5 * time.Minute,
	survival.ActionCraft:     5 * time.Minute,
	survival.ActionFarm:      3 * time.Minute,
	survival.ActionFarmPlant: 3 * time.Minute,
	survival.ActionMove:      1 * time.Minute,
}

func ensureCooldownReady(ctx context.Context, repo ports.EventRepository, agentID string, intentType survival.ActionType, now time.Time) error {
	cooldown, ok := actionCooldowns[intentType]
	if !ok || repo == nil {
		return nil
	}
	events, err := repo.ListByAgentID(ctx, agentID, 50)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return err
	}
	lastAt := time.Time{}
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		decision, ok := evt.Payload["decision"].(map[string]any)
		if !ok {
			continue
		}
		intent, _ := decision["intent"].(string)
		if intent != string(intentType) {
			continue
		}
		if evt.OccurredAt.After(lastAt) {
			lastAt = evt.OccurredAt
		}
	}
	if lastAt.IsZero() {
		return nil
	}
	if now.Sub(lastAt) < cooldown {
		return ErrActionCooldownActive
	}
	return nil
}

func resolveHeartbeatDeltaMinutes(ctx context.Context, repo ports.EventRepository, agentID string, now time.Time) (int, error) {
	if repo == nil {
		return defaultHeartbeatDeltaMinutes, nil
	}
	events, err := repo.ListByAgentID(ctx, agentID, 50)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return defaultHeartbeatDeltaMinutes, nil
		}
		return 0, err
	}
	lastAt := time.Time{}
	for _, evt := range events {
		if evt.Type != "action_settled" {
			continue
		}
		if evt.OccurredAt.After(lastAt) {
			lastAt = evt.OccurredAt
		}
	}
	if lastAt.IsZero() {
		return defaultHeartbeatDeltaMinutes, nil
	}
	delta := int(now.Sub(lastAt).Minutes())
	if delta < minHeartbeatDeltaMinutes {
		return minHeartbeatDeltaMinutes, nil
	}
	if delta > maxHeartbeatDeltaMinutes {
		return maxHeartbeatDeltaMinutes, nil
	}
	return delta, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func isSupportedActionType(t survival.ActionType) bool {
	switch t {
	case survival.ActionGather, survival.ActionRest, survival.ActionSleep, survival.ActionMove:
		return true
	case survival.ActionBuild, survival.ActionFarm, survival.ActionFarmPlant, survival.ActionFarmHarvest, survival.ActionContainerDeposit, survival.ActionContainerWithdraw, survival.ActionRetreat, survival.ActionCraft, survival.ActionEat, survival.ActionTerminate:
		return true
	default:
		return false
	}
}

func hasValidActionParams(intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionRest:
		restMinutes := intent.Params["rest_minutes"]
		return restMinutes >= minRestMinutes && restMinutes <= maxRestMinutes
	case survival.ActionSleep:
		return true
	case survival.ActionMove:
		return intent.Params["dx"] != 0 || intent.Params["dy"] != 0
	case survival.ActionBuild:
		return intent.Params["kind"] > 0
	case survival.ActionFarm, survival.ActionFarmPlant:
		return intent.Params["seed"] > 0 || intent.Params["farm_id"] > 0
	case survival.ActionFarmHarvest:
		return intent.Params["farm_id"] > 0 || len(intent.Params) == 0
	case survival.ActionCraft:
		return intent.Params["recipe"] > 0
	case survival.ActionEat:
		return intent.Params["food"] > 0
	case survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
		return intent.Params["container_id"] > 0 || len(intent.Params) == 0
	case survival.ActionTerminate:
		return true
	default:
		return true
	}
}

type ongoingFinalizeResult struct {
	Settled      bool
	UpdatedState survival.AgentStateAggregate
	Events       []survival.DomainEvent
	ResultCode   survival.ResultCode
	DTMinutes    int
}

func finalizeOngoingAction(ctx context.Context, u UseCase, agentID string, state survival.AgentStateAggregate, nowAt time.Time, forceTerminate bool) (ongoingFinalizeResult, error) {
	ongoing := state.OngoingAction
	if ongoing == nil {
		return ongoingFinalizeResult{}, nil
	}
	if nowAt.Before(ongoing.EndAt) && !forceTerminate {
		return ongoingFinalizeResult{}, nil
	}
	startAt := ongoing.EndAt.Add(-time.Duration(ongoing.Minutes) * time.Minute)
	deltaMinutes := int(nowAt.Sub(startAt).Minutes())
	if deltaMinutes < 0 {
		deltaMinutes = 0
	}
	if deltaMinutes > ongoing.Minutes {
		deltaMinutes = ongoing.Minutes
	}
	if deltaMinutes < minHeartbeatDeltaMinutes && !nowAt.Before(ongoing.EndAt) {
		deltaMinutes = minHeartbeatDeltaMinutes
	}

	snapshot, err := u.World.SnapshotForAgent(ctx, agentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return ongoingFinalizeResult{}, err
	}

	var result survival.SettlementResult
	if deltaMinutes > 0 {
		result, err = u.Settle.Settle(
			state,
			survival.ActionIntent{Type: ongoing.Type},
			survival.HeartbeatDelta{Minutes: deltaMinutes},
			nowAt,
			survival.WorldSnapshot{
				TimeOfDay:         snapshot.TimeOfDay,
				ThreatLevel:       snapshot.ThreatLevel,
				VisibilityPenalty: snapshot.VisibilityPenalty,
				NearbyResource:    snapshot.NearbyResource,
			},
		)
		if err != nil {
			return ongoingFinalizeResult{}, err
		}
	} else {
		result = survival.SettlementResult{
			UpdatedState: state,
			Events:       []survival.DomainEvent{},
			ResultCode:   survival.ResultOK,
		}
	}
	result.UpdatedState.OngoingAction = nil
	result.UpdatedState.UpdatedAt = nowAt

	sessionID := "session-" + agentID
	for i := range result.Events {
		if result.Events[i].Payload == nil {
			result.Events[i].Payload = map[string]any{}
		}
		result.Events[i].Payload["agent_id"] = agentID
		result.Events[i].Payload["session_id"] = sessionID
	}
	result.Events = append(result.Events, survival.DomainEvent{
		Type:       "ongoing_action_ended",
		OccurredAt: nowAt,
		Payload: map[string]any{
			"agent_id":        agentID,
			"session_id":      sessionID,
			"action_type":     string(ongoing.Type),
			"planned_minutes": ongoing.Minutes,
			"actual_minutes":  deltaMinutes,
			"forced":          forceTerminate,
		},
	})
	if err := u.StateRepo.SaveWithVersion(ctx, result.UpdatedState, state.Version); err != nil {
		return ongoingFinalizeResult{}, err
	}
	if err := u.EventRepo.Append(ctx, agentID, result.Events); err != nil {
		return ongoingFinalizeResult{}, err
	}
	if u.SessionRepo != nil && result.ResultCode == survival.ResultGameOver {
		if err := u.SessionRepo.Close(ctx, sessionID, result.UpdatedState.DeathCause, nowAt); err != nil {
			return ongoingFinalizeResult{}, err
		}
	}
	return ongoingFinalizeResult{
		Settled:      true,
		UpdatedState: result.UpdatedState,
		Events:       result.Events,
		ResultCode:   result.ResultCode,
		DTMinutes:    deltaMinutes,
	}, nil
}

func startRestAction(ctx context.Context, u UseCase, req Request, state survival.AgentStateAggregate, nowAt time.Time) (Response, error) {
	restMinutes := req.Intent.Params["rest_minutes"]
	next := state
	next.OngoingAction = &survival.OngoingActionInfo{
		Type:    survival.ActionRest,
		Minutes: restMinutes,
		EndAt:   nowAt.Add(time.Duration(restMinutes) * time.Minute),
	}
	next.Version++
	next.UpdatedAt = nowAt

	event := survival.DomainEvent{
		Type:       "rest_started",
		OccurredAt: nowAt,
		Payload: map[string]any{
			"agent_id":     req.AgentID,
			"session_id":   "session-" + req.AgentID,
			"rest_minutes": restMinutes,
			"end_at":       next.OngoingAction.EndAt,
		},
	}
	if req.StrategyHash != "" {
		event.Payload["strategy_hash"] = req.StrategyHash
	}
	if err := u.StateRepo.SaveWithVersion(ctx, next, state.Version); err != nil {
		return Response{}, err
	}
	if err := saveActionExecution(ctx, u.ActionRepo, req.AgentID, req.IdempotencyKey, req.Intent.Type, 0, ports.ActionResult{
		UpdatedState: next,
		Events:       []survival.DomainEvent{event},
		ResultCode:   survival.ResultOK,
	}, nowAt); err != nil {
		return Response{}, err
	}
	if err := u.EventRepo.Append(ctx, req.AgentID, []survival.DomainEvent{event}); err != nil {
		return Response{}, err
	}
	return Response{
		SettledDTMinutes:       0,
		WorldTimeBeforeSeconds: 0,
		WorldTimeAfterSeconds:  0,
		UpdatedState:           next,
		Events:                 []survival.DomainEvent{event},
		ResultCode:             survival.ResultOK,
	}, nil
}

func saveActionExecution(ctx context.Context, repo ports.ActionExecutionRepository, agentID, idempotencyKey string, intentType survival.ActionType, dt int, result ports.ActionResult, appliedAt time.Time) error {
	execution := ports.ActionExecutionRecord{
		AgentID:        agentID,
		IdempotencyKey: idempotencyKey,
		IntentType:     string(intentType),
		DT:             dt,
		Result:         result,
		AppliedAt:      appliedAt,
	}
	if err := repo.SaveExecution(ctx, execution); err != nil {
		return err
	}
	return nil
}

func toNum(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}
