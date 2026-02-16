package httpadapter

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
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
