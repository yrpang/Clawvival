package memory

import (
	"context"

	"clawverse/internal/app/ports"
)

type ActionExecutionRepo struct {
	store *Store
}

func NewActionExecutionRepo(store *Store) ActionExecutionRepo {
	return ActionExecutionRepo{store: store}
}

func (r ActionExecutionRepo) GetByIdempotencyKey(_ context.Context, agentID, key string) (*ports.ActionExecutionRecord, error) {
	rec, ok := r.store.execution[execKey(agentID, key)]
	if !ok {
		return nil, ports.ErrNotFound
	}
	copy := rec
	return &copy, nil
}

func (r ActionExecutionRepo) SaveExecution(_ context.Context, execution ports.ActionExecutionRecord) error {
	k := execKey(execution.AgentID, execution.IdempotencyKey)
	if _, exists := r.store.execution[k]; exists {
		return ports.ErrConflict
	}
	r.store.execution[k] = execution
	return nil
}
