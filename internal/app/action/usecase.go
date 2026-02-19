package action

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawvival/internal/app/cooldown"
	"clawvival/internal/app/ports"
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
	sleepBaseEnergyRecovery      = 24
	sleepBaseHPRecovery          = 8
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

func isInterruptibleOngoingActionType(t survival.ActionType) bool {
	return t == survival.ActionRest
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
		return intent.Pos != nil || intent.DX != 0 || intent.DY != 0
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
