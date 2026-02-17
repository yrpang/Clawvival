package replay

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID string
	Limit   int
}

type Response struct {
	Events      []survival.DomainEvent
	LatestState survival.AgentStateAggregate
}
