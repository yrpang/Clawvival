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
	View     View                         `json:"view"`
}

type View struct {
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Center world.Point `json:"center"`
	Radius int         `json:"radius"`
}
