package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"clawvival/internal/app/action"
	"clawvival/internal/app/auth"
	"clawvival/internal/app/observe"
	"clawvival/internal/app/ports"
	"clawvival/internal/app/replay"
	"clawvival/internal/app/skills"
	"clawvival/internal/app/status"
	"clawvival/internal/domain/survival"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"strconv"
)

const agentIDHeader = "X-Agent-ID"
const agentKeyHeader = "X-Agent-Key"

type Handler struct {
	RegisterUC auth.RegisterUseCase
	AuthUC     auth.VerifyUseCase
	ObserveUC  observe.UseCase
	ActionUC   action.UseCase
	StatusUC   status.UseCase
	ReplayUC   replay.UseCase
	SkillsUC   skills.UseCase
	KPI        kpiSnapshotProvider
}

func (h Handler) RegisterRoutes(s *server.Hertz) {
	agent := s.Group("/api/agent")
	agent.POST("/register", h.register)
	agent.POST("/observe", h.observe)
	agent.POST("/action", h.action)
	agent.POST("/status", h.status)
	agent.GET("/replay", h.replay)

	s.GET("/skills/index.json", h.skillsIndex)
	s.GET("/skills/*filepath", h.skillsFile)
	s.GET("/ops/kpi", h.kpi)
}

type observeRequest struct {
	AgentID string `json:"agent_id"`
}

type statusRequest struct {
	AgentID string `json:"agent_id"`
}

type actionRequest struct {
	AgentID        string       `json:"agent_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	Intent         actionIntent `json:"intent"`
	StrategyHash   string       `json:"strategy_hash,omitempty"`
}

type actionIntent struct {
	Type        string                `json:"type"`
	Direction   string                `json:"direction,omitempty"`
	TargetID    string                `json:"target_id,omitempty"`
	RecipeID    int                   `json:"recipe_id,omitempty"`
	Count       int                   `json:"count,omitempty"`
	ObjectType  string                `json:"object_type,omitempty"`
	Pos         *survival.Position    `json:"pos,omitempty"`
	ItemType    string                `json:"item_type,omitempty"`
	RestMinutes int                   `json:"rest_minutes,omitempty"`
	BedID       string                `json:"bed_id,omitempty"`
	FarmID      string                `json:"farm_id,omitempty"`
	ContainerID string                `json:"container_id,omitempty"`
	Items       []survival.ItemAmount `json:"items,omitempty"`
}

func (h Handler) observe(c context.Context, ctx *app.RequestContext) {
	agentID, err := h.requireAuthenticatedAgent(c, ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}

	var body observeRequest
	if err := decodeJSON(ctx, &body); err != nil {
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	resp, err := h.ObserveUC.Execute(c, observe.Request{AgentID: agentID})
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) action(c context.Context, ctx *app.RequestContext) {
	agentID, err := h.requireAuthenticatedAgent(c, ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}

	var body actionRequest
	if err := decodeJSON(ctx, &body); err != nil {
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	if hasJSONField(ctx.Request.Body(), "dt") {
		writeActionRejected(ctx, consts.StatusBadRequest, "dt_managed_by_server", "dt is managed by server", false, []string{"REQUIREMENT_NOT_MET"}, map[string]any{"field": "dt"})
		return
	}

	resp, err := h.ActionUC.Execute(c, action.Request{
		AgentID:        agentID,
		IdempotencyKey: body.IdempotencyKey,
		Intent: survival.ActionIntent{
			Type:        survival.ActionType(body.Intent.Type),
			Direction:   body.Intent.Direction,
			TargetID:    body.Intent.TargetID,
			RecipeID:    body.Intent.RecipeID,
			Count:       body.Intent.Count,
			ObjectType:  body.Intent.ObjectType,
			Pos:         body.Intent.Pos,
			ItemType:    body.Intent.ItemType,
			RestMinutes: body.Intent.RestMinutes,
			BedID:       body.Intent.BedID,
			FarmID:      body.Intent.FarmID,
			ContainerID: body.Intent.ContainerID,
			Items:       body.Intent.Items,
		},
		StrategyHash: body.StrategyHash,
	})
	if err != nil {
		if writeActionRejectedFromErr(ctx, err) {
			return
		}
		writeError(ctx, err)
		return
	}

	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) status(c context.Context, ctx *app.RequestContext) {
	agentID, err := h.requireAuthenticatedAgent(c, ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}

	var body statusRequest
	if err := decodeJSON(ctx, &body); err != nil {
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	resp, err := h.StatusUC.Execute(c, status.Request{AgentID: agentID})
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) replay(c context.Context, ctx *app.RequestContext) {
	agentID, err := h.requireAuthenticatedAgent(c, ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	limit, _ := strconv.Atoi(string(ctx.Query("limit")))
	occurredFrom, _ := strconv.ParseInt(string(ctx.Query("occurred_from")), 10, 64)
	occurredTo, _ := strconv.ParseInt(string(ctx.Query("occurred_to")), 10, 64)
	sessionID := string(ctx.Query("session_id"))
	resp, err := h.ReplayUC.Execute(c, replay.Request{
		AgentID:      agentID,
		Limit:        limit,
		OccurredFrom: occurredFrom,
		OccurredTo:   occurredTo,
		SessionID:    sessionID,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) skillsIndex(c context.Context, ctx *app.RequestContext) {
	b, err := h.SkillsUC.Index(c)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Data(http.StatusOK, "application/json", b)
}

func (h Handler) skillsFile(c context.Context, ctx *app.RequestContext) {
	path := strings.TrimPrefix(string(ctx.Param("filepath")), "/")
	if path == "" {
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_filepath", "invalid filepath")
		return
	}

	b, err := h.SkillsUC.File(c, path)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", b)
}

func (h Handler) register(c context.Context, ctx *app.RequestContext) {
	resp, err := h.RegisterUC.Execute(c, auth.RegisterRequest{})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(consts.StatusCreated, resp)
}

type kpiSnapshotProvider interface {
	SnapshotAny() any
}

func (h Handler) kpi(_ context.Context, ctx *app.RequestContext) {
	if h.KPI == nil {
		writeErrorBody(ctx, consts.StatusNotFound, "not_configured", "kpi provider not configured")
		return
	}
	ctx.JSON(consts.StatusOK, h.KPI.SnapshotAny())
}

func decodeJSON(ctx *app.RequestContext, out any) error {
	body := ctx.Request.Body()
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func hasJSONField(body []byte, key string) bool {
	if len(body) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

var ErrMissingAgentIDHeader = errors.New("missing x-agent-id header")
var ErrMissingAgentKeyHeader = errors.New("missing x-agent-key header")
var ErrMissingAgentCredentials = errors.New("missing agent credentials")

func (h Handler) requireAuthenticatedAgent(c context.Context, ctx *app.RequestContext) (string, error) {
	agentID := strings.TrimSpace(string(ctx.GetHeader(agentIDHeader)))
	agentKey := strings.TrimSpace(string(ctx.GetHeader(agentKeyHeader)))
	if agentID == "" && agentKey == "" {
		return "", ErrMissingAgentCredentials
	}
	if agentID == "" {
		return "", ErrMissingAgentIDHeader
	}
	if agentKey == "" {
		return "", ErrMissingAgentKeyHeader
	}
	if err := h.AuthUC.Execute(c, auth.VerifyRequest{
		AgentID:  agentID,
		AgentKey: agentKey,
	}); err != nil {
		return "", err
	}
	return agentID, nil
}

func writeError(ctx *app.RequestContext, err error) {
	switch {
	case errors.Is(err, ErrMissingAgentCredentials):
		writeErrorBody(ctx, consts.StatusBadRequest, "missing_agent_credentials", err.Error())
	case errors.Is(err, ErrMissingAgentIDHeader):
		writeErrorBody(ctx, consts.StatusBadRequest, "missing_agent_id", err.Error())
	case errors.Is(err, ErrMissingAgentKeyHeader):
		writeErrorBody(ctx, consts.StatusBadRequest, "missing_agent_key", err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeErrorBody(ctx, consts.StatusUnauthorized, "invalid_agent_credentials", err.Error())
	case errors.Is(err, action.ErrActionInvalidPosition):
		writeErrorBody(ctx, consts.StatusConflict, "action_invalid_position", err.Error())
	case errors.Is(err, action.ErrActionCooldownActive):
		writeErrorBody(ctx, consts.StatusConflict, "action_cooldown_active", err.Error())
	case errors.Is(err, action.ErrActionInProgress):
		writeErrorBody(ctx, consts.StatusConflict, "action_in_progress", err.Error())
	case errors.Is(err, action.ErrActionPreconditionFailed):
		writeErrorBody(ctx, consts.StatusConflict, "action_precondition_failed", err.Error())
	case errors.Is(err, action.ErrInventoryFull):
		writeErrorBody(ctx, consts.StatusConflict, "INVENTORY_FULL", err.Error())
	case errors.Is(err, action.ErrContainerFull):
		writeErrorBody(ctx, consts.StatusConflict, "CONTAINER_FULL", err.Error())
	case errors.Is(err, action.ErrInvalidActionParams):
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_action_params", err.Error())
	case errors.Is(err, action.ErrInvalidRequest),
		errors.Is(err, auth.ErrInvalidRequest),
		errors.Is(err, observe.ErrInvalidRequest),
		errors.Is(err, replay.ErrInvalidRequest),
		errors.Is(err, status.ErrInvalidRequest),
		errors.Is(err, survival.ErrInvalidDelta):
		writeErrorBody(ctx, consts.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, ports.ErrNotFound):
		writeErrorBody(ctx, consts.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ports.ErrConflict):
		writeErrorBody(ctx, consts.StatusConflict, "conflict", err.Error())
	default:
		writeErrorBody(ctx, consts.StatusInternalServerError, "internal_error", "internal error")
	}
}

func writeErrorBody(ctx *app.RequestContext, status int, code, message string) {
	ctx.JSON(status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeActionRejectedFromErr(ctx *app.RequestContext, err error) bool {
	switch {
	case errors.Is(err, action.ErrActionInvalidPosition):
		details := map[string]any{}
		var posErr *action.ActionInvalidPositionError
		if errors.As(err, &posErr) && posErr != nil {
			if posErr.TargetPos != nil {
				details["target_pos"] = map[string]int{
					"x": posErr.TargetPos.X,
					"y": posErr.TargetPos.Y,
				}
			}
			if posErr.BlockingTilePos != nil {
				details["blocking_tile_pos"] = map[string]int{
					"x": posErr.BlockingTilePos.X,
					"y": posErr.BlockingTilePos.Y,
				}
			}
		}
		if len(details) == 0 {
			details = nil
		}
		writeActionRejected(ctx, consts.StatusConflict, "action_invalid_position", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, details)
		return true
	case errors.Is(err, action.ErrActionCooldownActive):
		details := map[string]any{}
		var cooldownErr *action.ActionCooldownActiveError
		if errors.As(err, &cooldownErr) && cooldownErr != nil {
			details["intent"] = string(cooldownErr.IntentType)
			details["remaining_seconds"] = cooldownErr.RemainingSeconds
		}
		writeActionRejected(ctx, consts.StatusConflict, "action_cooldown_active", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, details)
		return true
	case errors.Is(err, action.ErrActionInProgress):
		writeActionRejected(ctx, consts.StatusConflict, "action_in_progress", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, nil)
		return true
	case errors.Is(err, action.ErrTargetOutOfView):
		writeActionRejected(ctx, consts.StatusConflict, "TARGET_OUT_OF_VIEW", err.Error(), false, []string{"NOT_VISIBLE"}, map[string]any{
			"in_window": false,
		})
		return true
	case errors.Is(err, action.ErrTargetNotVisible):
		writeActionRejected(ctx, consts.StatusConflict, "TARGET_NOT_VISIBLE", err.Error(), false, []string{"NOT_VISIBLE"}, map[string]any{
			"in_window":  true,
			"is_visible": false,
		})
		return true
	case errors.Is(err, action.ErrResourceDepleted):
		details := map[string]any{}
		var depletedErr *action.ResourceDepletedError
		if errors.As(err, &depletedErr) && depletedErr != nil {
			details["target_id"] = depletedErr.TargetID
			details["remaining_seconds"] = depletedErr.RemainingSeconds
		}
		writeActionRejected(ctx, consts.StatusConflict, "RESOURCE_DEPLETED", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, details)
		return true
	case errors.Is(err, action.ErrActionPreconditionFailed):
		writeActionRejected(ctx, consts.StatusConflict, "action_precondition_failed", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, nil)
		return true
	case errors.Is(err, action.ErrInventoryFull):
		writeActionRejected(ctx, consts.StatusConflict, "INVENTORY_FULL", err.Error(), false, []string{"INVENTORY_FULL"}, nil)
		return true
	case errors.Is(err, action.ErrContainerFull):
		writeActionRejected(ctx, consts.StatusConflict, "CONTAINER_FULL", err.Error(), false, []string{"CONTAINER_FULL"}, nil)
		return true
	case errors.Is(err, action.ErrInvalidActionParams):
		writeActionRejected(ctx, consts.StatusBadRequest, "invalid_action_params", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, nil)
		return true
	case errors.Is(err, action.ErrInvalidRequest):
		writeActionRejected(ctx, consts.StatusBadRequest, "bad_request", err.Error(), false, []string{"REQUIREMENT_NOT_MET"}, nil)
		return true
	default:
		return false
	}
}

func writeActionRejected(ctx *app.RequestContext, status int, code, message string, retryable bool, blockedBy []string, details map[string]any) {
	ctx.JSON(status, map[string]any{
		"result_code":               "REJECTED",
		"settled_dt_minutes":        0,
		"world_time_before_seconds": 0,
		"world_time_after_seconds":  0,
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"retryable":  retryable,
			"blocked_by": blockedBy,
			"details":    details,
		},
		"action_error": map[string]any{
			"code":       code,
			"message":    message,
			"retryable":  retryable,
			"blocked_by": blockedBy,
			"details":    details,
		},
	})
}
