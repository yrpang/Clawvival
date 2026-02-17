package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrTargetOutOfView          = errors.New("target out of view")
	ErrTargetNotVisible         = errors.New("target not visible")
)

const (
	defaultHeartbeatDeltaMinutes = 30
	minHeartbeatDeltaMinutes     = 1
	maxHeartbeatDeltaMinutes     = 120
	minRestMinutes               = 1
	maxRestMinutes               = 120
	targetViewRadius             = 5
	defaultFarmGrowMinutes       = 60
	seedPityMaxFails             = 8
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
		if err := validateTargetVisibility(state.Position, req.Intent, snapshot); err != nil {
			return err
		}
		preparedObj, err := prepareObjectAction(txCtx, nowAt, state, req.Intent, u.ObjectRepo, req.AgentID)
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
		req.Intent = resolveRetreatIntent(req.Intent, state.Position, snapshot.VisibleTiles)

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
				WorldTimeSeconds:  snapshot.WorldTimeSeconds,
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
		applySeedPityIfNeeded(txCtx, req.Intent, &result, state, u.EventRepo, req.AgentID)

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
				obj.ObjectType, obj.Quality, obj.CapacitySlots, obj.ObjectState = buildObjectDefaults(req.Intent.ObjectType)
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
		_, ok := buildKindFromObjectType(intent.ObjectType)
		return ok && survival.CanBuildObjectType(state, intent.ObjectType)
	case survival.ActionCraft:
		return survival.CanCraft(state, survival.RecipeID(intent.RecipeID))
	case survival.ActionFarmPlant:
		return survival.CanPlantSeed(state)
	case survival.ActionEat:
		foodID, ok := foodIDFromItemType(intent.ItemType)
		return ok && survival.CanEat(state, foodID)
	default:
		return true
	}
}

func positionPreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent, snapshot world.Snapshot) bool {
	if intent.Type != survival.ActionMove {
		return true
	}
	dx := intent.DX
	dy := intent.DY
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
		return true
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
	restMinutes := req.Intent.RestMinutes
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
	tx, ty, _, ok := parseResourceTargetID(intent.TargetID)
	if !ok {
		return ErrActionPreconditionFailed
	}
	if tx < center.X-targetViewRadius || tx > center.X+targetViewRadius || ty < center.Y-targetViewRadius || ty > center.Y+targetViewRadius {
		return ErrTargetOutOfView
	}
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == tx && tile.Y == ty {
			return nil
		}
	}
	return ErrTargetNotVisible
}

func parseResourceTargetID(targetID string) (x int, y int, resource string, ok bool) {
	var prefix string
	if _, err := fmt.Sscanf(strings.TrimSpace(targetID), "%3s_%d_%d_%s", &prefix, &x, &y, &resource); err != nil {
		return 0, 0, "", false
	}
	if prefix != "res" {
		return 0, 0, "", false
	}
	return x, y, resource, true
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
		return nil, nil
	}
	switch intent.Type {
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
		for _, item := range intent.Items {
			total += item.Count
			switch intent.Type {
			case survival.ActionContainerDeposit:
				if state.Inventory[item.ItemType] < item.Count {
					return nil, ErrActionPreconditionFailed
				}
			case survival.ActionContainerWithdraw:
				if box.Inventory[item.ItemType] < item.Count {
					return nil, ErrActionPreconditionFailed
				}
			}
		}
		if intent.Type == survival.ActionContainerDeposit && obj.CapacitySlots > 0 && obj.UsedSlots+total > obj.CapacitySlots {
			return nil, ErrActionPreconditionFailed
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

func resolveRetreatIntent(intent survival.ActionIntent, pos survival.Position, tiles []world.Tile) survival.ActionIntent {
	if intent.Type != survival.ActionRetreat {
		return intent
	}
	target, ok := highestThreatTile(pos, tiles)
	if !ok {
		return intent
	}
	intent.DX = stepAway(pos.X, target.X)
	intent.DY = stepAway(pos.Y, target.Y)
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
		if !bestFound || t.BaseThreat > bestThreat || (t.BaseThreat == bestThreat && dist < bestDist) {
			best = t
			bestFound = true
			bestThreat = t.BaseThreat
			bestDist = dist
		}
	}
	return best, bestFound
}

func stepAway(from, threat int) int {
	switch {
	case threat > from:
		return -1
	case threat < from:
		return 1
	default:
		return 0
	}
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
