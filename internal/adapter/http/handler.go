package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"clawverse/internal/app/action"
	"clawverse/internal/app/observe"
	"clawverse/internal/app/ports"
	"clawverse/internal/app/replay"
	"clawverse/internal/app/skills"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"strconv"
)

const agentIDHeader = "X-Agent-ID"

type Handler struct {
	ObserveUC observe.UseCase
	ActionUC  action.UseCase
	StatusUC  status.UseCase
	ReplayUC  replay.UseCase
	SkillsUC  skills.UseCase
	KPI       kpiSnapshotProvider
}

func (h Handler) RegisterRoutes(s *server.Hertz) {
	agent := s.Group("/api/agent")
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
	DT             int          `json:"dt"`
	StrategyHash   string       `json:"strategy_hash,omitempty"`
}

type actionIntent struct {
	Type   string         `json:"type"`
	Params map[string]int `json:"params,omitempty"`
}

func (h Handler) observe(c context.Context, ctx *app.RequestContext) {
	agentID, err := requireAgentID(ctx)
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
	agentID, err := requireAgentID(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}

	var body actionRequest
	if err := decodeJSON(ctx, &body); err != nil {
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	resp, err := h.ActionUC.Execute(c, action.Request{
		AgentID:        agentID,
		IdempotencyKey: body.IdempotencyKey,
		Intent: survival.ActionIntent{
			Type:   survival.ActionType(body.Intent.Type),
			Params: body.Intent.Params,
		},
		DeltaMinutes: body.DT,
		StrategyHash: body.StrategyHash,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) status(c context.Context, ctx *app.RequestContext) {
	agentID, err := requireAgentID(ctx)
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
	agentID, err := requireAgentID(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	limit, _ := strconv.Atoi(string(ctx.Query("limit")))
	occurredFrom, _ := strconv.ParseInt(string(ctx.Query("occurred_from")), 10, 64)
	occurredTo, _ := strconv.ParseInt(string(ctx.Query("occurred_to")), 10, 64)
	resp, err := h.ReplayUC.Execute(c, replay.Request{
		AgentID:      agentID,
		Limit:        limit,
		OccurredFrom: occurredFrom,
		OccurredTo:   occurredTo,
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

var ErrMissingAgentIDHeader = errors.New("missing x-agent-id header")

func requireAgentID(ctx *app.RequestContext) (string, error) {
	if fromHeader := strings.TrimSpace(string(ctx.GetHeader(agentIDHeader))); fromHeader != "" {
		return fromHeader, nil
	}
	return "", ErrMissingAgentIDHeader
}

func writeError(ctx *app.RequestContext, err error) {
	switch {
	case errors.Is(err, ErrMissingAgentIDHeader):
		writeErrorBody(ctx, consts.StatusBadRequest, "missing_agent_id", err.Error())
	case errors.Is(err, action.ErrActionPreconditionFailed):
		writeErrorBody(ctx, consts.StatusConflict, "action_precondition_failed", err.Error())
	case errors.Is(err, action.ErrInvalidActionParams):
		writeErrorBody(ctx, consts.StatusBadRequest, "invalid_action_params", err.Error())
	case errors.Is(err, action.ErrInvalidRequest),
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
