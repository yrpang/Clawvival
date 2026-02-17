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
	Type   string         `json:"type"`
	Params map[string]int `json:"params,omitempty"`
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
		writeErrorBody(ctx, consts.StatusBadRequest, "dt_managed_by_server", "dt is managed by server")
		return
	}

	resp, err := h.ActionUC.Execute(c, action.Request{
		AgentID:        agentID,
		IdempotencyKey: body.IdempotencyKey,
		Intent: survival.ActionIntent{
			Type:   survival.ActionType(body.Intent.Type),
			Params: body.Intent.Params,
		},
		StrategyHash: body.StrategyHash,
	})
	if err != nil {
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
	case errors.Is(err, action.ErrActionPreconditionFailed):
		writeErrorBody(ctx, consts.StatusConflict, "action_precondition_failed", err.Error())
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
