package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	staticskills "clawverse/internal/adapter/skills/static"
	"clawverse/internal/app/action"
	"clawverse/internal/app/skills"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route/param"
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

func TestWriteError_ActionInvalidPosition(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, action.ErrActionInvalidPosition)

	if got, want := ctx.Response.StatusCode(), consts.StatusConflict; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "action_invalid_position"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}

func TestWriteError_ActionCooldownActive(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, action.ErrActionCooldownActive)

	if got, want := ctx.Response.StatusCode(), consts.StatusConflict; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "action_cooldown_active"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}

func TestSkillsIndex_OK(t *testing.T) {
	h := Handler{
		SkillsUC: skills.UseCase{Provider: fakeSkillsProvider{
			index: []byte(`{"skills":[{"name":"demo"}]}`),
		}},
	}
	ctx := &app.RequestContext{}

	h.skillsIndex(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusOK; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	if got, want := string(ctx.Response.Body()), `{"skills":[{"name":"demo"}]}`; got != want {
		t.Fatalf("body mismatch: got=%q want=%q", got, want)
	}
}

func TestSkillsIndex_Error(t *testing.T) {
	h := Handler{
		SkillsUC: skills.UseCase{Provider: fakeSkillsProvider{
			err: errors.New("io failure"),
		}},
	}
	ctx := &app.RequestContext{}

	h.skillsIndex(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusInternalServerError; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
}

func TestSkillsFile_RejectsEmptyPath(t *testing.T) {
	h := Handler{
		SkillsUC: skills.UseCase{Provider: fakeSkillsProvider{}},
	}
	ctx := &app.RequestContext{}
	ctx.Params = param.Params{{Key: "filepath", Value: "/"}}

	h.skillsFile(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusBadRequest; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
}

func TestSkillsFile_OK(t *testing.T) {
	h := Handler{
		SkillsUC: skills.UseCase{Provider: fakeSkillsProvider{
			files: map[string][]byte{"demo.md": []byte("hello")},
		}},
	}
	ctx := &app.RequestContext{}
	ctx.Params = param.Params{{Key: "filepath", Value: "/demo.md"}}

	h.skillsFile(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusOK; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	if got, want := string(ctx.Response.Body()), "hello"; got != want {
		t.Fatalf("body mismatch: got=%q want=%q", got, want)
	}
}

func TestSkillsFile_PathTraversalBlocked(t *testing.T) {
	h := Handler{
		SkillsUC: skills.UseCase{Provider: staticskills.Provider{Root: t.TempDir()}},
	}
	ctx := &app.RequestContext{}
	ctx.Params = param.Params{{Key: "filepath", Value: "/../outside.txt"}}

	h.skillsFile(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusInternalServerError; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
}

type fakeSkillsProvider struct {
	index []byte
	files map[string][]byte
	err   error
}

func (p fakeSkillsProvider) Index(_ context.Context) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.index, nil
}

func (p fakeSkillsProvider) File(_ context.Context, path string) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	if b, ok := p.files[path]; ok {
		return b, nil
	}
	return nil, errors.New("not found")
}
