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
	"clawverse/internal/app/skills"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

const agentIDHeader = "X-Agent-ID"

type Handler struct {
	ObserveUC observe.UseCase
	ActionUC  action.UseCase
	StatusUC  status.UseCase
	SkillsUC  skills.UseCase
}

func (h Handler) RegisterRoutes(s *server.Hertz) {
	agent := s.Group("/api/agent")
	agent.POST("/observe", h.observe)
	agent.POST("/action", h.action)
	agent.POST("/status", h.status)

	s.GET("/skills/index.json", h.skillsIndex)
	s.GET("/skills/*filepath", h.skillsFile)
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
	var body observeRequest
	if err := decodeJSON(ctx, &body); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	resp, err := h.ObserveUC.Execute(c, observe.Request{AgentID: resolveAgentID(ctx, body.AgentID)})
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(consts.StatusOK, resp)
}

func (h Handler) action(c context.Context, ctx *app.RequestContext) {
	var body actionRequest
	if err := decodeJSON(ctx, &body); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	resp, err := h.ActionUC.Execute(c, action.Request{
		AgentID:        resolveAgentID(ctx, body.AgentID),
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
	var body statusRequest
	if err := decodeJSON(ctx, &body); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	resp, err := h.StatusUC.Execute(c, status.Request{AgentID: resolveAgentID(ctx, body.AgentID)})
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
		ctx.JSON(consts.StatusBadRequest, map[string]any{"error": "invalid filepath"})
		return
	}

	b, err := h.SkillsUC.File(c, path)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", b)
}

func decodeJSON(ctx *app.RequestContext, out any) error {
	body := ctx.Request.Body()
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func resolveAgentID(ctx *app.RequestContext, bodyAgentID string) string {
	if fromHeader := strings.TrimSpace(string(ctx.GetHeader(agentIDHeader))); fromHeader != "" {
		return fromHeader
	}
	return strings.TrimSpace(bodyAgentID)
}

func writeError(ctx *app.RequestContext, err error) {
	switch {
	case errors.Is(err, action.ErrInvalidRequest),
		errors.Is(err, observe.ErrInvalidRequest),
		errors.Is(err, status.ErrInvalidRequest),
		errors.Is(err, survival.ErrInvalidDelta):
		ctx.JSON(consts.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, ports.ErrNotFound):
		ctx.JSON(consts.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, ports.ErrConflict):
		ctx.JSON(consts.StatusConflict, map[string]any{"error": err.Error()})
	default:
		ctx.JSON(consts.StatusInternalServerError, map[string]any{"error": "internal error"})
	}
}
