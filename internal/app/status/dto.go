package status

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID string
}

type Response struct {
	State survival.AgentStateAggregate
}
