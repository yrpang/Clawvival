package memory

import (
	"sync"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

type Store struct {
	mu        sync.RWMutex
	state     map[string]survival.AgentStateAggregate
	execution map[string]ports.ActionExecutionRecord
	events    map[string][]survival.DomainEvent
}

func NewStore() *Store {
	return &Store{
		state:     make(map[string]survival.AgentStateAggregate),
		execution: make(map[string]ports.ActionExecutionRecord),
		events:    make(map[string][]survival.DomainEvent),
	}
}

func execKey(agentID, key string) string {
	return agentID + "::" + key
}

func (s *Store) SeedState(state survival.AgentStateAggregate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[state.AgentID] = state
}
