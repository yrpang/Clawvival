package httpadapter

import (
	"encoding/json"
	"testing"

	"clawverse/internal/app/action"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func TestRequireAgentID_FromHeader(t *testing.T) {
	ctx := &app.RequestContext{}
	ctx.Request.Header.Set(agentIDHeader, "agent-1")

	agentID, err := requireAgentID(ctx)
	if err != nil {
		t.Fatalf("requireAgentID error: %v", err)
	}
	if agentID != "agent-1" {
		t.Fatalf("unexpected agent id: %q", agentID)
	}
}

func TestRequireAgentID_MissingHeader(t *testing.T) {
	ctx := &app.RequestContext{}

	_, err := requireAgentID(ctx)
	if err == nil {
		t.Fatalf("expected error when header is missing")
	}
	if err != ErrMissingAgentIDHeader {
		t.Fatalf("expected ErrMissingAgentIDHeader, got %v", err)
	}
}

func TestWriteError_InvalidActionParams(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, action.ErrInvalidActionParams)

	if got, want := ctx.Response.StatusCode(), consts.StatusBadRequest; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "invalid_action_params"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}

func TestWriteError_ActionPreconditionFailed(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, action.ErrActionPreconditionFailed)

	if got, want := ctx.Response.StatusCode(), consts.StatusConflict; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "action_precondition_failed"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}
