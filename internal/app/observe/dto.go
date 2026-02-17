package observe

import (
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type Request struct {
	AgentID string
}

type Response struct {
	State       survival.AgentStateAggregate `json:"state"`
	Snapshot    world.Snapshot               `json:"snapshot"`
	View        View                         `json:"view"`
	World       WorldMeta                    `json:"world"`
	ActionCosts map[string]ActionCost        `json:"action_costs"`
}

type View struct {
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Center world.Point `json:"center"`
	Radius int         `json:"radius"`
}

type WorldMeta struct {
	Rules Rules `json:"rules"`
}

type Rules struct {
	StandardTickMinutes int `json:"standard_tick_minutes"`
}

type ActionCost struct {
	BaseMinutes int `json:"base_minutes"`
}
