package observe

import (
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type Request struct {
	AgentID string
}

type Response struct {
	State    survival.AgentStateAggregate `json:"state"`
	Snapshot world.Snapshot               `json:"snapshot"`
}
