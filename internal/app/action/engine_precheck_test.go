package action

import (
	"context"
	"errors"
	"testing"
	"time"

	"clawvival/internal/domain/survival"
)

type stubSessionRepo struct {
	err       error
	called    bool
	sessionID string
	agentID   string
	startTick int64
}

func (r *stubSessionRepo) EnsureActive(_ context.Context, sessionID, agentID string, startTick int64) error {
	r.called = true
	r.sessionID = sessionID
	r.agentID = agentID
	r.startTick = startTick
	return r.err
}

func (r *stubSessionRepo) Close(context.Context, string, survival.DeathCause, time.Time) error {
	return nil
}

func TestRunStandardActionPrecheck_EnsuresSessionAndChecksCooldown(t *testing.T) {
	now := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)
	sessionRepo := &stubSessionRepo{}
	uc := UseCase{SessionRepo: sessionRepo}
	ac := &ActionContext{
		In: ActionInput{
			SessionID: "session-agent-1",
			AgentID:   "agent-1",
			NowAt:     now,
		},
		View: ActionView{
			StateWorking: survival.AgentStateAggregate{Version: 42},
			EventsBefore: []survival.DomainEvent{{
				Type:       "action_settled",
				OccurredAt: now.Add(-30 * time.Second),
				Payload: map[string]any{
					"decision": map[string]any{"intent": string(survival.ActionMove)},
				},
			}},
		},
		Tmp: ActionTmp{ResolvedIntent: survival.ActionIntent{Type: survival.ActionMove}},
	}

	err := runStandardActionPrecheck(context.Background(), uc, ac)
	if !errors.Is(err, ErrActionCooldownActive) {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	if !sessionRepo.called {
		t.Fatal("expected session EnsureActive to be called")
	}
	if sessionRepo.sessionID != "session-agent-1" || sessionRepo.agentID != "agent-1" || sessionRepo.startTick != 42 {
		t.Fatalf("unexpected EnsureActive params: session=%q agent=%q start=%d", sessionRepo.sessionID, sessionRepo.agentID, sessionRepo.startTick)
	}
}

func TestRunStandardActionPrecheck_ReturnsSessionErrorFirst(t *testing.T) {
	sessionErr := errors.New("session closed")
	sessionRepo := &stubSessionRepo{err: sessionErr}
	uc := UseCase{SessionRepo: sessionRepo}
	ac := &ActionContext{
		In:   ActionInput{SessionID: "s1", AgentID: "a1", NowAt: time.Now()},
		View: ActionView{StateWorking: survival.AgentStateAggregate{Version: 1}},
		Tmp:  ActionTmp{ResolvedIntent: survival.ActionIntent{Type: survival.ActionMove}},
	}

	err := runStandardActionPrecheck(context.Background(), uc, ac)
	if !errors.Is(err, sessionErr) {
		t.Fatalf("expected session error, got %v", err)
	}
}
