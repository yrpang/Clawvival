package memory

import (
	"context"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

type AgentStateRepo struct {
	store *Store
}

func NewAgentStateRepo(store *Store) AgentStateRepo {
	return AgentStateRepo{store: store}
}

func (r AgentStateRepo) GetByAgentID(_ context.Context, agentID string) (survival.AgentStateAggregate, error) {
	state, ok := r.store.state[agentID]
	if !ok {
		return survival.AgentStateAggregate{}, ports.ErrNotFound
	}
	return state, nil
}

func (r AgentStateRepo) SaveWithVersion(_ context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	current, ok := r.store.state[state.AgentID]
	if !ok {
		if expectedVersion != 0 {
			return ports.ErrConflict
		}
		r.store.state[state.AgentID] = state
		return nil
	}
	if current.Version != expectedVersion {
		return ports.ErrConflict
	}
	r.store.state[state.AgentID] = state
	return nil
}
