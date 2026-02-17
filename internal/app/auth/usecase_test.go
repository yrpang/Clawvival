package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

func TestRegisterUseCase_CreatesCredentialAndSeedState(t *testing.T) {
	creds := &fakeCredentialRepo{}
	state := &fakeStateRepo{}
	uc := RegisterUseCase{
		Credentials: creds,
		StateRepo:   state,
		TxManager:   fakeTxManager{},
		Now:         func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}

	resp, err := uc.Execute(context.Background(), RegisterRequest{})
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if resp.AgentID == "" || resp.AgentKey == "" || resp.IssuedAt == "" {
		t.Fatalf("expected non-empty register response: %+v", resp)
	}
	if creds.last.AgentID != resp.AgentID {
		t.Fatalf("credential agent mismatch: %s != %s", creds.last.AgentID, resp.AgentID)
	}
	if len(creds.last.KeySalt) == 0 || len(creds.last.KeyHash) == 0 {
		t.Fatalf("expected credential salt/hash stored")
	}
	if state.last.AgentID != resp.AgentID {
		t.Fatalf("state seed agent mismatch: %s != %s", state.last.AgentID, resp.AgentID)
	}
	if state.last.Version != 1 {
		t.Fatalf("expected seed version=1, got %d", state.last.Version)
	}
}

func TestVerifyUseCase_AcceptsValidCredentials(t *testing.T) {
	salt := []byte("salt")
	key := "agent-secret"
	repo := &fakeCredentialRepo{
		getResult: ports.AgentCredentialRecord{
			AgentID: "agt_1",
			KeySalt: salt,
			KeyHash: credentialHash(salt, key),
			Status:  CredentialStatusActive,
		},
	}
	uc := VerifyUseCase{Credentials: repo}

	if err := uc.Execute(context.Background(), VerifyRequest{AgentID: "agt_1", AgentKey: key}); err != nil {
		t.Fatalf("verify error: %v", err)
	}
}

func TestVerifyUseCase_RejectsInvalidCredentials(t *testing.T) {
	salt := []byte("salt")
	repo := &fakeCredentialRepo{
		getResult: ports.AgentCredentialRecord{
			AgentID: "agt_1",
			KeySalt: salt,
			KeyHash: credentialHash(salt, "correct"),
			Status:  CredentialStatusActive,
		},
	}
	uc := VerifyUseCase{Credentials: repo}

	err := uc.Execute(context.Background(), VerifyRequest{AgentID: "agt_1", AgentKey: "wrong"})
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestRegisterUseCase_RollsBackOnStateSaveError(t *testing.T) {
	creds := &fakeCredentialRepo{}
	state := &fakeStateRepo{saveErr: errors.New("state save failed")}
	tx := rollbackOnErrTxManager{creds: creds}
	uc := RegisterUseCase{
		Credentials: creds,
		StateRepo:   state,
		TxManager:   tx,
		Now:         func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}

	_, err := uc.Execute(context.Background(), RegisterRequest{})
	if err == nil {
		t.Fatalf("expected register error")
	}
	if creds.last.AgentID != "" {
		t.Fatalf("expected credential write rolled back on state failure")
	}
}

type fakeCredentialRepo struct {
	last      ports.AgentCredentialRecord
	createErr error
	getResult ports.AgentCredentialRecord
	getErr    error
}

func (f *fakeCredentialRepo) Create(_ context.Context, credential ports.AgentCredentialRecord) error {
	f.last = credential
	return f.createErr
}

func (f *fakeCredentialRepo) GetByAgentID(_ context.Context, _ string) (ports.AgentCredentialRecord, error) {
	if f.getErr != nil {
		return ports.AgentCredentialRecord{}, f.getErr
	}
	return f.getResult, nil
}

type fakeStateRepo struct {
	last      survival.AgentStateAggregate
	saveErr   error
	getResult survival.AgentStateAggregate
	getErr    error
}

func (f *fakeStateRepo) GetByAgentID(_ context.Context, _ string) (survival.AgentStateAggregate, error) {
	if f.getErr != nil {
		return survival.AgentStateAggregate{}, f.getErr
	}
	return f.getResult, nil
}

func (f *fakeStateRepo) SaveWithVersion(_ context.Context, state survival.AgentStateAggregate, _ int64) error {
	f.last = state
	return f.saveErr
}

type fakeTxManager struct{}

func (fakeTxManager) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type rollbackOnErrTxManager struct {
	creds *fakeCredentialRepo
}

func (m rollbackOnErrTxManager) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	snapshot := m.creds.last
	if err := fn(ctx); err != nil {
		m.creds.last = snapshot
		return err
	}
	return nil
}
