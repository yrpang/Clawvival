package replay

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID      string
	Limit        int
	OccurredFrom int64
	OccurredTo   int64
}

type Response struct {
	Events      []survival.DomainEvent
	LatestState survival.AgentStateAggregate
}
