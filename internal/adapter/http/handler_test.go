package httpadapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	staticskills "clawvival/internal/adapter/skills/static"
	"clawvival/internal/app/action"
	"clawvival/internal/app/auth"
	"clawvival/internal/app/ports"
	"clawvival/internal/app/skills"
	"clawvival/internal/domain/survival"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route/param"
)

func TestRequireAuthenticatedAgent_FromHeaders(t *testing.T) {
	salt := []byte("salt")
	key := "k1"
	h := Handler{
		AuthUC: auth.VerifyUseCase{Credentials: fakeCredentialStore{
			cred: ports.AgentCredentialRecord{
				AgentID: "agent-1",
				KeySalt: salt,
				KeyHash: hashForTest(salt, key),
				Status:  auth.CredentialStatusActive,
			},
		}},
	}
	ctx := &app.RequestContext{}
	ctx.Request.Header.Set(agentIDHeader, "agent-1")
	ctx.Request.Header.Set(agentKeyHeader, key)

	agentID, err := h.requireAuthenticatedAgent(context.Background(), ctx)
	if err != nil {
		t.Fatalf("requireAuthenticatedAgent error: %v", err)
	}
	if agentID != "agent-1" {
		t.Fatalf("unexpected agent id: %q", agentID)
	}
}

func TestRequireAuthenticatedAgent_MissingHeader(t *testing.T) {
	h := Handler{}
	ctx := &app.RequestContext{}

	_, err := h.requireAuthenticatedAgent(context.Background(), ctx)
	if err == nil {
		t.Fatalf("expected error when header is missing")
	}
	if err != ErrMissingAgentCredentials {
		t.Fatalf("expected ErrMissingAgentCredentials, got %v", err)
	}
}

func TestRequireAuthenticatedAgent_MissingKeyHeader(t *testing.T) {
	h := Handler{}
	ctx := &app.RequestContext{}
	ctx.Request.Header.Set(agentIDHeader, "agent-1")

	_, err := h.requireAuthenticatedAgent(context.Background(), ctx)
	if err != ErrMissingAgentKeyHeader {
		t.Fatalf("expected ErrMissingAgentKeyHeader, got %v", err)
	}
}

func TestRequireAuthenticatedAgent_InvalidCredentials(t *testing.T) {
	h := Handler{
		AuthUC: auth.VerifyUseCase{Credentials: fakeCredentialStore{}},
	}
	ctx := &app.RequestContext{}
	ctx.Request.Header.Set(agentIDHeader, "agent-1")
	ctx.Request.Header.Set(agentKeyHeader, "wrong")

	_, err := h.requireAuthenticatedAgent(context.Background(), ctx)
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestWriteError_InvalidActionParams(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, action.ErrInvalidActionParams)

	if got, want := ctx.Response.StatusCode(), consts.StatusBadRequest; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}

	var body map[string]map[string]any
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

	var body map[string]map[string]any
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
	var body map[string]map[string]any
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
	var body map[string]map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "action_cooldown_active"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}

func TestWriteActionRejectedFromErr_TargetNotVisible(t *testing.T) {
	ctx := &app.RequestContext{}
	if ok := writeActionRejectedFromErr(ctx, action.ErrTargetNotVisible); !ok {
		t.Fatalf("expected handled error")
	}
	if got, want := ctx.Response.StatusCode(), consts.StatusConflict; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["result_code"], "REJECTED"; got != want {
		t.Fatalf("result_code mismatch: got=%v want=%v", got, want)
	}
	errObj, _ := body["error"].(map[string]any)
	if got, want := errObj["code"], "TARGET_NOT_VISIBLE"; got != want {
		t.Fatalf("error code mismatch: got=%v want=%v", got, want)
	}
	if got, want := errObj["retryable"], false; got != want {
		t.Fatalf("error.retryable mismatch: got=%v want=%v", got, want)
	}
}

func TestWriteError_InvalidCredentials(t *testing.T) {
	ctx := &app.RequestContext{}
	writeError(ctx, auth.ErrInvalidCredentials)

	if got, want := ctx.Response.StatusCode(), consts.StatusUnauthorized; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["error"]["code"], "invalid_agent_credentials"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
}

func TestAction_RejectsClientDTField(t *testing.T) {
	salt := []byte("salt")
	key := "k1"
	h := Handler{
		AuthUC: auth.VerifyUseCase{Credentials: fakeCredentialStore{
			cred: ports.AgentCredentialRecord{
				AgentID: "agent-1",
				KeySalt: salt,
				KeyHash: hashForTest(salt, key),
				Status:  auth.CredentialStatusActive,
			},
		}},
	}
	ctx := &app.RequestContext{}
	ctx.Request.SetBody([]byte(`{"idempotency_key":"k1","intent":{"type":"gather"},"dt":30}`))
	ctx.Request.Header.Set(agentIDHeader, "agent-1")
	ctx.Request.Header.Set(agentKeyHeader, key)

	h.action(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusBadRequest; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["result_code"], "REJECTED"; got != want {
		t.Fatalf("result_code mismatch: got=%v want=%v", got, want)
	}
	errObj, _ := body["error"].(map[string]any)
	if got, want := errObj["code"], "dt_managed_by_server"; got != want {
		t.Fatalf("error code mismatch: got=%q want=%q", got, want)
	}
	if got, want := errObj["retryable"], false; got != want {
		t.Fatalf("error.retryable mismatch: got=%v want=%v", got, want)
	}
	actionErr, _ := body["action_error"].(map[string]any)
	if got, want := actionErr["retryable"], false; got != want {
		t.Fatalf("action_error.retryable mismatch: got=%v want=%v", got, want)
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

func TestRegister_OK(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	h := Handler{
		RegisterUC: auth.RegisterUseCase{
			Credentials: &fakeCredentialStore{},
			StateRepo:   &fakeStateStore{},
			TxManager:   fakeTxManager{},
			Now:         func() time.Time { return now },
		},
	}
	ctx := &app.RequestContext{}

	h.register(context.Background(), ctx)

	if got, want := ctx.Response.StatusCode(), consts.StatusCreated; got != want {
		t.Fatalf("status mismatch: got=%d want=%d", got, want)
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := body["agent_id"]; !ok {
		t.Fatalf("expected agent_id in response")
	}
	if _, ok := body["agent_key"]; !ok {
		t.Fatalf("expected agent_key in response")
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

type fakeCredentialStore struct {
	cred ports.AgentCredentialRecord
}

func (s fakeCredentialStore) Create(_ context.Context, credential ports.AgentCredentialRecord) error {
	return nil
}

func (s fakeCredentialStore) GetByAgentID(_ context.Context, _ string) (ports.AgentCredentialRecord, error) {
	if s.cred.AgentID == "" {
		return ports.AgentCredentialRecord{}, ports.ErrNotFound
	}
	return s.cred, nil
}

type fakeStateStore struct{}

func (fakeStateStore) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	return survival.AgentStateAggregate{}, ports.ErrNotFound
}

func (fakeStateStore) SaveWithVersion(_ context.Context, _ survival.AgentStateAggregate, _ int64) error {
	return nil
}

type fakeTxManager struct{}

func (fakeTxManager) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func hashForTest(salt []byte, key string) []byte {
	b := make([]byte, 0, len(salt)+len(key))
	b = append(b, salt...)
	b = append(b, key...)
	sum := sha256.Sum256(b)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}
