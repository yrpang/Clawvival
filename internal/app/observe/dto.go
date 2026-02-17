package observe

import (
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

type Request struct {
	AgentID string
}

type Response struct {
	State    survival.AgentStateAggregate `json:"state"`
	Snapshot world.Snapshot               `json:"snapshot"`
}
