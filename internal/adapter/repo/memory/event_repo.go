package memory

import (
	"context"

	"clawverse/internal/domain/survival"
)

type EventRepo struct {
	store *Store
}

func NewEventRepo(store *Store) EventRepo {
	return EventRepo{store: store}
}

func (r EventRepo) Append(_ context.Context, events []survival.DomainEvent) error {
	for _, e := range events {
		t, _ := e.Payload["agent_id"].(string)
		if t == "" {
			t = "global"
		}
		r.store.events[t] = append(r.store.events[t], e)
	}
	return nil
}
