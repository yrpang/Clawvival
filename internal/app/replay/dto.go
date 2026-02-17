package replay

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID      string
	Limit        int
	OccurredFrom int64
	OccurredTo   int64
	SessionID    string
}

type Response struct {
	Events      []survival.DomainEvent       `json:"events"`
	LatestState survival.AgentStateAggregate `json:"latest_state"`
}
