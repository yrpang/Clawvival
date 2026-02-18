package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"clawvival/internal/app/cooldown"
	"clawvival/internal/app/ports"
	"clawvival/internal/app/resourcestate"
	"clawvival/internal/app/stateview"
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

const (
	defaultHeartbeatDeltaMinutes = 30
	minHeartbeatDeltaMinutes     = 1
	maxHeartbeatDeltaMinutes     = 120
	minRestMinutes               = 1
	maxRestMinutes               = 120
	defaultFarmGrowMinutes       = 60
	seedPityMaxFails             = 8
	actionNightVisionRadius      = 3
	defaultInventoryCapacity     = 30
)

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
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Intent.Type = survival.ActionType(strings.TrimSpace(string(req.Intent.Type)))
	req.Intent = normalizeIntent(req.Intent)
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
			before, after := worldTimeWindowFromExecution(exec)
			updatedState := exec.Result.UpdatedState
			if strings.TrimSpace(updatedState.SessionID) == "" {
				updatedState.SessionID = "session-" + req.AgentID
			}
			out = Response{
				SettledDTMinutes:       exec.DT,
				WorldTimeBeforeSeconds: before,
				WorldTimeAfterSeconds:  after,
				UpdatedState:           updatedState,
				Events:                 exec.Result.Events,
				Settlement:             settlementSummary(exec.Result.Events),
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
		sessionID := "session-" + req.AgentID
		state.SessionID = sessionID
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
				WorldTimeBeforeSeconds: finalized.WorldTimeBeforeSeconds,
				WorldTimeAfterSeconds:  finalized.WorldTimeAfterSeconds,
				UpdatedState:           finalized.UpdatedState,
				Events:                 finalized.Events,
				Settlement:             settlementSummary(finalized.Events),
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
		if u.SessionRepo != nil {
			if err := u.SessionRepo.EnsureActive(txCtx, sessionID, req.AgentID, state.Version); err != nil {
				return err
			}
		}

		snapshot, err := u.World.SnapshotForAgent(txCtx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
		if err != nil {
			return err
		}
		if err := validateTargetVisibility(state.Position, req.Intent, snapshot); err != nil {
			return err
		}
		if err := validateGatherTargetState(txCtx, u.ResourceRepo, req.AgentID, req.Intent, nowAt); err != nil {
			return err
		}
		preparedObj, err := prepareObjectAction(txCtx, nowAt, state, req.Intent, u.ObjectRepo, req.AgentID)
		if err != nil {
			return err
		}
		if moveErr := validateMovePosition(state, req.Intent, snapshot); moveErr != nil {
			return moveErr
		}
		eventsBeforeAction, err := listRecentEvents(txCtx, u.EventRepo, req.AgentID)
		if err != nil {
			return err
		}
		if err := ensureCooldownReady(eventsBeforeAction, req.Intent.Type, nowAt); err != nil {
			return err
		}
		deltaMinutes, err := resolveHeartbeatDeltaMinutes(txCtx, u.EventRepo, req.AgentID, nowAt)
		if err != nil {
			return err
		}
		req.Intent = resolveRetreatIntent(req.Intent, state.Position, snapshot.VisibleTiles)
		settleNearby := snapshot.NearbyResource
		if req.Intent.Type == survival.ActionGather {
			settleNearby = filterGatherNearbyResource(req.Intent.TargetID, snapshot.NearbyResource)
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
				NearbyResource:    settleNearby,
				WorldTimeSeconds:  snapshot.WorldTimeSeconds,
			},
		)
		if err != nil {
			return err
		}
		result.UpdatedState = stateview.Enrich(result.UpdatedState, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))
		result.UpdatedState.CurrentZone = stateview.CurrentZoneAtPosition(result.UpdatedState.Position, snapshot.VisibleTiles)
		result.UpdatedState.ActionCooldowns = cooldown.RemainingByActionWithCurrent(eventsBeforeAction, nowAt, req.Intent.Type)
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
		applySeedPityIfNeeded(txCtx, req.Intent, &result, state, u.EventRepo, req.AgentID)
		attachLastKnownThreat(&result, snapshot)
		if err := persistGatherDepletion(txCtx, u.ResourceRepo, req.AgentID, req.Intent, nowAt); err != nil {
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

		if err := persistObjectAction(txCtx, nowAt, req.Intent, preparedObj, u.ObjectRepo, req.AgentID); err != nil {
			return err
		}

		if err := u.EventRepo.Append(txCtx, req.AgentID, result.Events); err != nil {
			return err
		}
		builtObjectIDs := make([]string, 0, 1)
		if u.ObjectRepo != nil {
			for _, evt := range result.Events {
				if evt.Type != "build_completed" || evt.Payload == nil {
					continue
				}
				objectID := "obj-" + req.AgentID + "-" + req.IdempotencyKey
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
				obj.ObjectType, obj.Quality, obj.CapacitySlots, obj.ObjectState = buildObjectDefaults(req.Intent.ObjectType)
				if err := u.ObjectRepo.Save(txCtx, req.AgentID, obj); err != nil {
					return err
				}
				builtObjectIDs = append(builtObjectIDs, objectID)
			}
		}
		attachBuiltObjectIDs(result.Events, builtObjectIDs)
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
			Settlement:             settlementSummary(result.Events),
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

func settlementSummary(events []survival.DomainEvent) map[string]any {
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		result, ok := evt.Payload["result"].(map[string]any)
		if !ok || result == nil {
			return nil
		}
		return map[string]any{
			"hp_loss":               result["hp_loss"],
			"inventory_delta":       result["inventory_delta"],
			"vitals_delta":          result["vitals_delta"],
			"vitals_change_reasons": result["vitals_change_reasons"],
		}
	}
	return nil
}

func worldTimeWindow(beforeSeconds int64, dtMinutes int) (int64, int64) {
	return beforeSeconds, beforeSeconds + int64(dtMinutes*60)
}

func worldTimeWindowFromExecution(exec *ports.ActionExecutionRecord) (int64, int64) {
	if exec == nil {
		return 0, 0
	}
	if before, after, ok := worldTimeWindowFromEventPayload(exec.Result.Events); ok {
		return before, after
	}
	return worldTimeWindow(0, exec.DT)
}

func worldTimeWindowFromEvents(events []survival.DomainEvent, fallbackBefore int64, dtMinutes int) (int64, int64) {
	if before, after, ok := worldTimeWindowFromEventPayload(events); ok {
		return before, after
	}
	return worldTimeWindow(fallbackBefore, dtMinutes)
}

func worldTimeWindowFromEventPayload(events []survival.DomainEvent) (int64, int64, bool) {
	for _, evt := range events {
		if evt.Payload == nil {
			continue
		}
		beforeRaw, hasBefore := evt.Payload["world_time_before_seconds"]
		afterRaw, hasAfter := evt.Payload["world_time_after_seconds"]
		if !hasBefore || !hasAfter {
			continue
		}
		return int64(toNum(beforeRaw)), int64(toNum(afterRaw)), true
	}
	return 0, 0, false
}

func isInterruptibleOngoingActionType(t survival.ActionType) bool {
	return t == survival.ActionRest
}

func resourcePreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionBuild:
		_, ok := buildKindFromObjectType(intent.ObjectType)
		return ok && survival.CanBuildObjectType(state, intent.ObjectType)
	case survival.ActionCraft:
		return survival.CanCraft(state, survival.RecipeID(intent.RecipeID))
	case survival.ActionFarmPlant:
		return survival.CanPlantSeed(state)
	case survival.ActionEat:
		foodID, ok := foodIDFromItemType(intent.ItemType)
		if !ok || !survival.CanEat(state, foodID) {
			return false
		}
		count := intent.Count
		if count <= 0 {
			count = 1
		}
		return state.Inventory[strings.ToLower(strings.TrimSpace(intent.ItemType))] >= count
	default:
		return true
	}
}

func validateMovePosition(state survival.AgentStateAggregate, intent survival.ActionIntent, snapshot world.Snapshot) error {
	if intent.Type != survival.ActionMove {
		return nil
	}
	dx := intent.DX
	dy := intent.DY
	targetX := state.Position.X + dx
	targetY := state.Position.Y + dy
	targetPos := &survival.Position{X: targetX, Y: targetY}
	if abs(dx) > 1 || abs(dy) > 1 {
		return &ActionInvalidPositionError{TargetPos: targetPos}
	}
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == targetX && tile.Y == targetY {
			if tile.Passable {
				return nil
			}
			return &ActionInvalidPositionError{
				TargetPos:       targetPos,
				BlockingTilePos: &survival.Position{X: targetX, Y: targetY},
			}
		}
	}
	return &ActionInvalidPositionError{TargetPos: targetPos}
}

func ensureCooldownReady(events []survival.DomainEvent, intentType survival.ActionType, now time.Time) error {
	remaining, ok := cooldown.RemainingForAction(events, intentType, now)
	if !ok {
		return nil
	}
	return &ActionCooldownActiveError{
		IntentType:       intentType,
		RemainingSeconds: remaining,
	}
}

func listRecentEvents(ctx context.Context, repo ports.EventRepository, agentID string) ([]survival.DomainEvent, error) {
	if repo == nil {
		return nil, nil
	}
	events, err := repo.ListByAgentID(ctx, agentID, 50)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, err
	}
	return events, nil
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
	case survival.ActionBuild, survival.ActionFarmPlant, survival.ActionFarmHarvest, survival.ActionContainerDeposit, survival.ActionContainerWithdraw, survival.ActionRetreat, survival.ActionCraft, survival.ActionEat, survival.ActionTerminate:
		return true
	default:
		return false
	}
}

func hasValidActionParams(intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionRest:
		restMinutes := intent.RestMinutes
		return restMinutes >= minRestMinutes && restMinutes <= maxRestMinutes
	case survival.ActionSleep:
		return strings.TrimSpace(intent.BedID) != ""
	case survival.ActionMove:
		return intent.DX != 0 || intent.DY != 0
	case survival.ActionGather:
		return strings.TrimSpace(intent.TargetID) != ""
	case survival.ActionBuild:
		_, ok := buildKindFromObjectType(intent.ObjectType)
		return ok && intent.Pos != nil
	case survival.ActionFarmPlant:
		return strings.TrimSpace(intent.FarmID) != ""
	case survival.ActionFarmHarvest:
		return strings.TrimSpace(intent.FarmID) != ""
	case survival.ActionCraft:
		return intent.RecipeID > 0
	case survival.ActionEat:
		_, ok := foodIDFromItemType(intent.ItemType)
		return ok && intent.Count > 0
	case survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
		return strings.TrimSpace(intent.ContainerID) != "" && hasValidItems(intent.Items)
	case survival.ActionTerminate:
		return true
	default:
		return true
	}
}

type ongoingFinalizeResult struct {
	Settled                bool
	UpdatedState           survival.AgentStateAggregate
	Events                 []survival.DomainEvent
	ResultCode             survival.ResultCode
	DTMinutes              int
	WorldTimeBeforeSeconds int64
	WorldTimeAfterSeconds  int64
}

func finalizeOngoingAction(ctx context.Context, u UseCase, agentID string, state survival.AgentStateAggregate, nowAt time.Time, forceTerminate bool) (ongoingFinalizeResult, error) {
	ongoing := state.OngoingAction
	if ongoing == nil {
		return ongoingFinalizeResult{}, nil
	}
	if forceTerminate && !isInterruptibleOngoingActionType(ongoing.Type) {
		return ongoingFinalizeResult{}, ErrActionPreconditionFailed
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
				WorldTimeSeconds:  snapshot.WorldTimeSeconds,
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
	result.UpdatedState = stateview.Enrich(result.UpdatedState, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))

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
	worldTimeBefore, worldTimeAfter := worldTimeWindowFromEvents(result.Events, snapshot.WorldTimeSeconds, deltaMinutes)
	return ongoingFinalizeResult{
		Settled:                true,
		UpdatedState:           result.UpdatedState,
		Events:                 result.Events,
		ResultCode:             result.ResultCode,
		DTMinutes:              deltaMinutes,
		WorldTimeBeforeSeconds: worldTimeBefore,
		WorldTimeAfterSeconds:  worldTimeAfter,
	}, nil
}

func startRestAction(ctx context.Context, u UseCase, req Request, state survival.AgentStateAggregate, nowAt time.Time) (Response, error) {
	restMinutes := req.Intent.RestMinutes
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return Response{}, err
	}
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
			"agent_id":                  req.AgentID,
			"session_id":                "session-" + req.AgentID,
			"rest_minutes":              restMinutes,
			"end_at":                    next.OngoingAction.EndAt,
			"world_time_before_seconds": snapshot.WorldTimeSeconds,
			"world_time_after_seconds":  snapshot.WorldTimeSeconds,
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
		WorldTimeBeforeSeconds: snapshot.WorldTimeSeconds,
		WorldTimeAfterSeconds:  snapshot.WorldTimeSeconds,
		UpdatedState:           next,
		Events:                 []survival.DomainEvent{event},
		ResultCode:             survival.ResultOK,
	}, nil
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

func validateTargetVisibility(center survival.Position, intent survival.ActionIntent, snapshot world.Snapshot) error {
	if intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" {
		return nil
	}
	tx, ty, resource, ok := resourcestate.ParseResourceTargetID(intent.TargetID)
	if !ok {
		return ErrActionPreconditionFailed
	}
	viewRadius := snapshot.ViewRadius
	if viewRadius <= 0 {
		viewRadius = 5
	}
	if tx < center.X-viewRadius || tx > center.X+viewRadius || ty < center.Y-viewRadius || ty > center.Y+viewRadius {
		return ErrTargetOutOfView
	}
	if strings.EqualFold(snapshot.TimeOfDay, "night") {
		dist := abs(tx-center.X) + abs(ty-center.Y)
		if dist > actionNightVisionRadius {
			return ErrTargetNotVisible
		}
	}
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == tx && tile.Y == ty {
			if resource != "" && !strings.EqualFold(strings.TrimSpace(tile.Resource), strings.TrimSpace(resource)) {
				return ErrActionPreconditionFailed
			}
			return nil
		}
	}
	return ErrTargetNotVisible
}

func buildKindFromObjectType(objectType string) (survival.BuildKind, bool) {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "bed", "bed_rough", "bed_good":
		return survival.BuildBed, true
	case "box":
		return survival.BuildBox, true
	case "farm_plot":
		return survival.BuildFarm, true
	case "torch":
		return survival.BuildTorch, true
	default:
		return 0, false
	}
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

func hasValidItems(items []survival.ItemAmount) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item.ItemType) == "" || item.Count <= 0 {
			return false
		}
	}
	return true
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

type preparedObjectAction struct {
	record ports.WorldObjectRecord
	box    boxObjectState
	farm   farmObjectState
}

type boxObjectState struct {
	Inventory map[string]int `json:"inventory"`
}

type farmObjectState struct {
	State         string `json:"state"`
	PlantedAtUnix int64  `json:"planted_at_unix,omitempty"`
	ReadyAtUnix   int64  `json:"ready_at_unix,omitempty"`
}

func prepareObjectAction(ctx context.Context, nowAt time.Time, state survival.AgentStateAggregate, intent survival.ActionIntent, repo ports.WorldObjectRepository, agentID string) (*preparedObjectAction, error) {
	if repo == nil {
		switch intent.Type {
		case survival.ActionSleep, survival.ActionFarmPlant, survival.ActionFarmHarvest, survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
			return nil, ErrActionPreconditionFailed
		}
		return nil, nil
	}
	switch intent.Type {
	case survival.ActionSleep:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.BedID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isBedObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		if obj.X != state.Position.X || obj.Y != state.Position.Y {
			return nil, ErrActionPreconditionFailed
		}
		return &preparedObjectAction{record: obj}, nil
	case survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.ContainerID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isBoxObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		box, err := parseBoxObjectState(obj.ObjectState)
		if err != nil {
			return nil, ErrActionPreconditionFailed
		}
		total := 0
		requested := aggregateItemCounts(intent.Items)
		for _, item := range intent.Items {
			total += item.Count
		}
		for itemType, need := range requested {
			switch intent.Type {
			case survival.ActionContainerDeposit:
				if state.Inventory[itemType] < need {
					return nil, ErrActionPreconditionFailed
				}
			case survival.ActionContainerWithdraw:
				if box.Inventory[itemType] < need {
					return nil, ErrActionPreconditionFailed
				}
			}
		}
		if intent.Type == survival.ActionContainerDeposit && obj.CapacitySlots > 0 && obj.UsedSlots+total > obj.CapacitySlots {
			return nil, ErrContainerFull
		}
		if intent.Type == survival.ActionContainerWithdraw {
			capacity := state.InventoryCapacity
			if capacity <= 0 {
				capacity = defaultInventoryCapacity
			}
			if inventoryUsed(state.Inventory)+total > capacity {
				return nil, ErrInventoryFull
			}
		}
		return &preparedObjectAction{record: obj, box: box}, nil
	case survival.ActionFarmPlant, survival.ActionFarmHarvest:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.FarmID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isFarmObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		farm, err := parseFarmObjectState(obj.ObjectState)
		if err != nil {
			return nil, ErrActionPreconditionFailed
		}
		switch intent.Type {
		case survival.ActionFarmPlant:
			if strings.ToUpper(strings.TrimSpace(farm.State)) != "IDLE" {
				return nil, ErrActionPreconditionFailed
			}
		case survival.ActionFarmHarvest:
			ready := strings.ToUpper(strings.TrimSpace(farm.State)) == "READY"
			if strings.ToUpper(strings.TrimSpace(farm.State)) == "GROWING" && farm.ReadyAtUnix > 0 && nowAt.Unix() >= farm.ReadyAtUnix {
				ready = true
			}
			if !ready {
				return nil, ErrActionPreconditionFailed
			}
		}
		return &preparedObjectAction{record: obj, farm: farm}, nil
	default:
		return nil, nil
	}
}

func persistObjectAction(ctx context.Context, nowAt time.Time, intent survival.ActionIntent, prepared *preparedObjectAction, repo ports.WorldObjectRepository, agentID string) error {
	if repo == nil || prepared == nil {
		return nil
	}
	obj := prepared.record
	switch intent.Type {
	case survival.ActionContainerDeposit:
		if prepared.box.Inventory == nil {
			prepared.box.Inventory = map[string]int{}
		}
		for _, item := range intent.Items {
			prepared.box.Inventory[item.ItemType] += item.Count
			obj.UsedSlots += item.Count
		}
		encoded, err := json.Marshal(prepared.box)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionContainerWithdraw:
		if prepared.box.Inventory == nil {
			prepared.box.Inventory = map[string]int{}
		}
		for _, item := range intent.Items {
			prepared.box.Inventory[item.ItemType] -= item.Count
			if prepared.box.Inventory[item.ItemType] <= 0 {
				delete(prepared.box.Inventory, item.ItemType)
			}
			obj.UsedSlots -= item.Count
		}
		if obj.UsedSlots < 0 {
			obj.UsedSlots = 0
		}
		encoded, err := json.Marshal(prepared.box)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionFarmPlant:
		prepared.farm.State = "GROWING"
		prepared.farm.PlantedAtUnix = nowAt.Unix()
		prepared.farm.ReadyAtUnix = nowAt.Add(defaultFarmGrowMinutes * time.Minute).Unix()
		encoded, err := json.Marshal(prepared.farm)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionFarmHarvest:
		prepared.farm.State = "IDLE"
		prepared.farm.PlantedAtUnix = 0
		prepared.farm.ReadyAtUnix = 0
		encoded, err := json.Marshal(prepared.farm)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	default:
		return nil
	}
}

func parseBoxObjectState(raw string) (boxObjectState, error) {
	out := boxObjectState{Inventory: map[string]int{}}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return boxObjectState{}, err
	}
	if out.Inventory == nil {
		out.Inventory = map[string]int{}
	}
	return out, nil
}

func parseFarmObjectState(raw string) (farmObjectState, error) {
	out := farmObjectState{State: "IDLE"}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return farmObjectState{}, err
	}
	if strings.TrimSpace(out.State) == "" {
		out.State = "IDLE"
	}
	return out, nil
}

func isBoxObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "box" || obj.Kind == int(survival.BuildBox)
}

func isFarmObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "farm_plot" || obj.Kind == int(survival.BuildFarm)
}

func isBedObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "bed" || obj.Kind == int(survival.BuildBed)
}

func buildObjectDefaults(intentObjectType string) (objectType, quality string, capacitySlots int, objectState string) {
	switch strings.ToLower(strings.TrimSpace(intentObjectType)) {
	case "box":
		return "box", "", 60, `{"inventory":{}}`
	case "farm_plot":
		return "farm_plot", "", 0, `{"state":"IDLE"}`
	case "bed_good":
		return "bed", "GOOD", 0, ""
	case "bed_rough":
		return "bed", "ROUGH", 0, ""
	case "bed":
		return "bed", "ROUGH", 0, ""
	default:
		return strings.ToLower(strings.TrimSpace(intentObjectType)), "", 0, ""
	}
}

func applySeedPityIfNeeded(ctx context.Context, intent survival.ActionIntent, result *survival.SettlementResult, before survival.AgentStateAggregate, repo ports.EventRepository, agentID string) {
	if intent.Type != survival.ActionGather || result == nil {
		return
	}
	beforeSeed := before.Inventory["seed"]
	afterSeed := result.UpdatedState.Inventory["seed"]
	seedGained := afterSeed > beforeSeed

	if evt := findActionSettledEvent(result.Events); evt != nil {
		if evt.Payload == nil {
			evt.Payload = map[string]any{}
		}
		res, _ := evt.Payload["result"].(map[string]any)
		if res == nil {
			res = map[string]any{}
		}
		res["seed_gained"] = seedGained
		res["seed_pity_triggered"] = false
		evt.Payload["result"] = res
	}
	if seedGained || repo == nil {
		return
	}

	fails := consecutiveGatherSeedFails(ctx, repo, agentID)
	if fails < seedPityMaxFails-1 {
		return
	}

	result.UpdatedState.AddItem("seed", 1)
	if evt := findActionSettledEvent(result.Events); evt != nil {
		res, _ := evt.Payload["result"].(map[string]any)
		if res == nil {
			res = map[string]any{}
		}
		res["seed_gained"] = true
		res["seed_pity_triggered"] = true
		evt.Payload["result"] = res
	}
	result.Events = append(result.Events, survival.DomainEvent{
		Type:       "seed_pity_triggered",
		OccurredAt: result.UpdatedState.UpdatedAt,
		Payload: map[string]any{
			"agent_id": agentID,
			"granted":  1,
		},
	})
}

func consecutiveGatherSeedFails(ctx context.Context, repo ports.EventRepository, agentID string) int {
	events, err := repo.ListByAgentID(ctx, agentID, 100)
	if err != nil {
		return 0
	}
	fails := 0
	for _, evt := range events {
		if evt.Type != "action_settled" || evt.Payload == nil {
			continue
		}
		decision, _ := evt.Payload["decision"].(map[string]any)
		if decision == nil || strings.TrimSpace(fmt.Sprint(decision["intent"])) != string(survival.ActionGather) {
			continue
		}
		result, _ := evt.Payload["result"].(map[string]any)
		if result == nil {
			break
		}
		if gained, ok := result["seed_gained"].(bool); ok && gained {
			break
		}
		fails++
	}
	return fails
}

func findActionSettledEvent(events []survival.DomainEvent) *survival.DomainEvent {
	for i := range events {
		if events[i].Type == "action_settled" {
			return &events[i]
		}
	}
	return nil
}

func attachLastKnownThreat(result *survival.SettlementResult, snapshot world.Snapshot) {
	if result == nil {
		return
	}
	threat, ok := strongestVisibleThreat(snapshot)
	if !ok {
		return
	}
	for i := range result.Events {
		if result.Events[i].Type != "game_over" || result.Events[i].Payload == nil {
			continue
		}
		result.Events[i].Payload["last_known_threat"] = map[string]any{
			"id":           fmt.Sprintf("thr_%d_%d", threat.X, threat.Y),
			"type":         "wild",
			"pos":          map[string]int{"x": threat.X, "y": threat.Y},
			"danger_score": min(100, threat.BaseThreat*25),
		}
	}
}

func strongestVisibleThreat(snapshot world.Snapshot) (world.Tile, bool) {
	best := world.Tile{}
	found := false
	for _, tile := range snapshot.VisibleTiles {
		if tile.BaseThreat <= 0 {
			continue
		}
		if !found || tile.BaseThreat > best.BaseThreat {
			found = true
			best = tile
		}
	}
	return best, found
}

func resolveRetreatIntent(intent survival.ActionIntent, pos survival.Position, tiles []world.Tile) survival.ActionIntent {
	if intent.Type != survival.ActionRetreat {
		return intent
	}
	target, ok := highestThreatTile(pos, tiles)
	if !ok {
		return intent
	}
	dx, dy, ok := bestRetreatStep(pos, target, tiles)
	if !ok {
		return intent
	}
	intent.DX = dx
	intent.DY = dy
	return intent
}

func highestThreatTile(pos survival.Position, tiles []world.Tile) (world.Tile, bool) {
	best := world.Tile{}
	bestFound := false
	bestThreat := -1
	bestDist := 0
	for _, t := range tiles {
		if t.BaseThreat <= 0 {
			continue
		}
		dist := abs(t.X-pos.X) + abs(t.Y-pos.Y)
		if dist == 0 {
			continue
		}
		if !bestFound || t.BaseThreat > bestThreat || (t.BaseThreat == bestThreat && dist < bestDist) {
			best = t
			bestFound = true
			bestThreat = t.BaseThreat
			bestDist = dist
		}
	}
	return best, bestFound
}

func bestRetreatStep(pos survival.Position, threat world.Tile, tiles []world.Tile) (int, int, bool) {
	type dir struct {
		dx int
		dy int
	}
	candidates := []dir{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	visible := make(map[string]world.Tile, len(tiles))
	for _, t := range tiles {
		visible[posKey(t.X, t.Y)] = t
	}

	bestDX, bestDY := 0, 0
	bestFound := false
	bestDist := -1
	bestRisk := 9999
	for _, c := range candidates {
		tx := pos.X + c.dx
		ty := pos.Y + c.dy
		tile, ok := visible[posKey(tx, ty)]
		if !ok || !tile.Passable {
			continue
		}
		dist := abs(tx-threat.X) + abs(ty-threat.Y)
		risk := tile.BaseThreat
		if !bestFound || dist > bestDist || (dist == bestDist && risk < bestRisk) {
			bestFound = true
			bestDist = dist
			bestRisk = risk
			bestDX = c.dx
			bestDY = c.dy
		}
	}
	return bestDX, bestDY, bestFound
}

func posKey(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isCurrentTileLit(timeOfDay string) bool {
	return strings.EqualFold(strings.TrimSpace(timeOfDay), "day")
}

func inventoryUsed(items map[string]int) int {
	total := 0
	for _, c := range items {
		if c > 0 {
			total += c
		}
	}
	return total
}

func aggregateItemCounts(items []survival.ItemAmount) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ItemType))
		if key == "" || item.Count <= 0 {
			continue
		}
		out[key] += item.Count
	}
	return out
}

func filterGatherNearbyResource(targetID string, nearby map[string]int) map[string]int {
	_, _, resource, ok := resourcestate.ParseResourceTargetID(targetID)
	if !ok || strings.TrimSpace(resource) == "" {
		return map[string]int{}
	}
	resource = strings.ToLower(strings.TrimSpace(resource))
	// A gather action targets a single resource node by id.
	// Keep per-action yield stable regardless of how many same-type nodes are in view.
	return map[string]int{resource: 1}
}

func validateGatherTargetState(ctx context.Context, repo ports.AgentResourceNodeRepository, agentID string, intent survival.ActionIntent, now time.Time) error {
	if intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" || repo == nil {
		return nil
	}
	record, err := repo.GetByTargetID(ctx, agentID, strings.TrimSpace(intent.TargetID))
	if errors.Is(err, ports.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !record.DepletedUntil.After(now) {
		return nil
	}
	remaining := int(record.DepletedUntil.Sub(now).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	return &ResourceDepletedError{TargetID: intent.TargetID, RemainingSeconds: remaining}
}

func persistGatherDepletion(ctx context.Context, repo ports.AgentResourceNodeRepository, agentID string, intent survival.ActionIntent, now time.Time) error {
	if repo == nil || intent.Type != survival.ActionGather || strings.TrimSpace(intent.TargetID) == "" {
		return nil
	}
	x, y, resource, ok := resourcestate.ParseResourceTargetID(intent.TargetID)
	if !ok {
		return nil
	}
	return repo.Upsert(ctx, ports.AgentResourceNodeRecord{
		AgentID:       agentID,
		TargetID:      strings.TrimSpace(intent.TargetID),
		ResourceType:  resource,
		X:             x,
		Y:             y,
		DepletedUntil: now.Add(resourcestate.RespawnDuration(resource)),
	})
}

func attachBuiltObjectIDs(events []survival.DomainEvent, ids []string) {
	if len(ids) == 0 {
		return
	}
	for i := range events {
		if events[i].Type != "action_settled" || events[i].Payload == nil {
			continue
		}
		result, _ := events[i].Payload["result"].(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		result["built_object_ids"] = ids
		events[i].Payload["result"] = result
	}
}
