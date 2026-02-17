package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

const (
	CredentialStatusActive = "active"
)

var (
	ErrInvalidRequest     = errors.New("invalid auth request")
	ErrInvalidCredentials = errors.New("invalid agent credentials")
)

type RegisterRequest struct{}

type RegisterResponse struct {
	AgentID  string `json:"agent_id"`
	AgentKey string `json:"agent_key"`
	IssuedAt string `json:"issued_at"`
}

type VerifyRequest struct {
	AgentID  string
	AgentKey string
}

type RegisterUseCase struct {
	Credentials ports.AgentCredentialRepository
	StateRepo   ports.AgentStateRepository
	TxManager   ports.TxManager
	Now         func() time.Time
}

type VerifyUseCase struct {
	Credentials ports.AgentCredentialRepository
}

func (u RegisterUseCase) Execute(ctx context.Context, _ RegisterRequest) (RegisterResponse, error) {
	if u.Credentials == nil || u.StateRepo == nil || u.TxManager == nil {
		return RegisterResponse{}, ErrInvalidRequest
	}
	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()

	for i := 0; i < 3; i++ {
		agentID, err := newAgentID(now)
		if err != nil {
			return RegisterResponse{}, err
		}
		agentKey, err := randomToken(32)
		if err != nil {
			return RegisterResponse{}, err
		}
		salt, err := randomBytes(16)
		if err != nil {
			return RegisterResponse{}, err
		}
		hash := credentialHash(salt, agentKey)

		err = u.TxManager.RunInTx(ctx, func(txCtx context.Context) error {
			if err := u.Credentials.Create(txCtx, ports.AgentCredentialRecord{
				AgentID:   agentID,
				KeySalt:   salt,
				KeyHash:   hash,
				Status:    CredentialStatusActive,
				CreatedAt: now,
			}); err != nil {
				return err
			}
			seed := survival.AgentStateAggregate{
				AgentID:    agentID,
				Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
				Position:   survival.Position{X: 0, Y: 0},
				Home:       survival.Position{X: 0, Y: 0},
				Inventory:  map[string]int{},
				Dead:       false,
				DeathCause: survival.DeathCauseUnknown,
				Version:    1,
				UpdatedAt:  now,
			}
			return u.StateRepo.SaveWithVersion(txCtx, seed, 0)
		})
		if err == ports.ErrConflict {
			continue
		}
		if err != nil {
			return RegisterResponse{}, err
		}
		return RegisterResponse{
			AgentID:  agentID,
			AgentKey: agentKey,
			IssuedAt: now.Format(time.RFC3339),
		}, nil
	}

	return RegisterResponse{}, ports.ErrConflict
}

func (u VerifyUseCase) Execute(ctx context.Context, req VerifyRequest) error {
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.AgentKey = strings.TrimSpace(req.AgentKey)
	if req.AgentID == "" || req.AgentKey == "" || u.Credentials == nil {
		return ErrInvalidRequest
	}

	cred, err := u.Credentials.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		if err == ports.ErrNotFound {
			return ErrInvalidCredentials
		}
		return err
	}
	if cred.Status != CredentialStatusActive {
		return ErrInvalidCredentials
	}

	got := credentialHash(cred.KeySalt, req.AgentKey)
	if subtle.ConstantTimeCompare(got, cred.KeyHash) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

func credentialHash(salt []byte, key string) []byte {
	b := make([]byte, 0, len(salt)+len(key))
	b = append(b, salt...)
	b = append(b, key...)
	sum := sha256.Sum256(b)
	return sum[:]
}

func newAgentID(now time.Time) (string, error) {
	randPart, err := randomToken(9)
	if err != nil {
		return "", err
	}
	return "agt_" + now.Format("20060102") + "_" + randPart, nil
}

func randomToken(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
