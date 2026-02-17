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
)

const (
	defaultHeartbeatDeltaMinutes = 30
	minHeartbeatDeltaMinutes     = 1
	maxHeartbeatDeltaMinutes     = 120
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
		nowAt := nowFn()
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

		execution := ports.ActionExecutionRecord{
			AgentID:        req.AgentID,
			IdempotencyKey: req.IdempotencyKey,
			IntentType:     string(req.Intent.Type),
			DT:             deltaMinutes,
			Result: ports.ActionResult{
				UpdatedState: result.UpdatedState,
				Events:       result.Events,
				ResultCode:   result.ResultCode,
			},
			AppliedAt: nowAt,
		}
		if err := u.ActionRepo.SaveExecution(txCtx, execution); err != nil {
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

func resourcePreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent) bool {
	switch intent.Type {
	case survival.ActionBuild:
		return survival.CanBuild(state, survival.BuildKind(intent.Params["kind"]))
	case survival.ActionCraft:
		return survival.CanCraft(state, survival.RecipeID(intent.Params["recipe"]))
	case survival.ActionFarm:
		if intent.Params["seed"] > 0 {
			return survival.CanPlantSeed(state)
		}
		return true
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
	survival.ActionCombat: 10 * time.Minute,
	survival.ActionBuild:  5 * time.Minute,
	survival.ActionCraft:  5 * time.Minute,
	survival.ActionFarm:   3 * time.Minute,
	survival.ActionMove:   1 * time.Minute,
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
