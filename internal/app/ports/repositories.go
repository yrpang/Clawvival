package ports

import (
	"context"
	"time"

	"clawverse/internal/domain/survival"
)

type ActionResult struct {
	UpdatedState survival.AgentStateAggregate
	Events       []survival.DomainEvent
	ResultCode   survival.ResultCode
}

type ActionExecutionRecord struct {
	AgentID        string
	IdempotencyKey string
	IntentType     string
	DT             int
	Result         ActionResult
	AppliedAt      time.Time
}

type AgentStateRepository interface {
	GetByAgentID(ctx context.Context, agentID string) (survival.AgentStateAggregate, error)
	SaveWithVersion(ctx context.Context, state survival.AgentStateAggregate, expectedVersion int64) error
}

type ActionExecutionRepository interface {
	GetByIdempotencyKey(ctx context.Context, agentID, key string) (*ActionExecutionRecord, error)
	SaveExecution(ctx context.Context, execution ActionExecutionRecord) error
}

type EventRepository interface {
	Append(ctx context.Context, agentID string, events []survival.DomainEvent) error
	ListByAgentID(ctx context.Context, agentID string, limit int) ([]survival.DomainEvent, error)
}

type WorldObjectRecord struct {
	ObjectID string
	Kind     int
	X        int
	Y        int
	HP       int
}

type WorldObjectRepository interface {
	Save(ctx context.Context, agentID string, obj WorldObjectRecord) error
}

type AgentSessionRepository interface {
	EnsureActive(ctx context.Context, sessionID, agentID string, startTick int64) error
	Close(ctx context.Context, sessionID string, cause survival.DeathCause, endedAt time.Time) error
}

type AgentCredentialRecord struct {
	AgentID   string
	KeySalt   []byte
	KeyHash   []byte
	Status    string
	CreatedAt time.Time
}

type AgentCredentialRepository interface {
	Create(ctx context.Context, credential AgentCredentialRecord) error
	GetByAgentID(ctx context.Context, agentID string) (AgentCredentialRecord, error)
}
