package survival

import "time"

func (s *AgentStateAggregate) AddItem(item string, amount int) {
	if amount <= 0 || item == "" {
		return
	}
	if s.Inventory == nil {
		s.Inventory = map[string]int{}
	}
	s.Inventory[item] += amount
}

func (s *AgentStateAggregate) ConsumeItem(item string, amount int) bool {
	if amount <= 0 || item == "" || s.Inventory == nil {
		return false
	}
	current := s.Inventory[item]
	if current < amount {
		return false
	}
	s.Inventory[item] = current - amount
	return true
}

func (s *AgentStateAggregate) MarkDead(cause DeathCause) {
	if cause == "" {
		cause = DeathCauseUnknown
	}
	s.Dead = true
	s.DeathCause = cause
}

type SessionStatus string

const (
	SessionAlive SessionStatus = "alive"
	SessionDead  SessionStatus = "dead"
)

type AgentSession struct {
	ID         string
	AgentID    string
	StartTick  int64
	EndedAt    *time.Time
	Status     SessionStatus
	DeathCause DeathCause
}

func NewSession(agentID string, startTick int64) AgentSession {
	return AgentSession{
		ID:        "session-" + agentID,
		AgentID:   agentID,
		StartTick: startTick,
		Status:    SessionAlive,
	}
}

func (s *AgentSession) Close(cause DeathCause) {
	now := time.Now()
	s.EndedAt = &now
	s.Status = SessionDead
	if cause == "" {
		cause = DeathCauseUnknown
	}
	s.DeathCause = cause
}
