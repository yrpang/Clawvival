package observe

import (
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type Request struct {
	AgentID string
}

type Response struct {
	State            survival.AgentStateAggregate `json:"state"`
	Snapshot         world.Snapshot               `json:"snapshot"`
	View             View                         `json:"view"`
	World            WorldMeta                    `json:"world"`
	ActionCosts      map[string]ActionCost        `json:"action_costs"`
	Tiles            []ObservedTile               `json:"tiles"`
	Objects          []ObservedObject             `json:"objects"`
	Resources        []ObservedResource           `json:"resources"`
	Threats          []ObservedThreat             `json:"threats"`
	LocalThreatLevel int                          `json:"local_threat_level"`
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

type ObservedTile struct {
	Pos         world.Point `json:"pos"`
	TerrainType string      `json:"terrain_type"`
	IsWalkable  bool        `json:"is_walkable"`
	IsLit       bool        `json:"is_lit"`
	IsVisible   bool        `json:"is_visible"`
}

type ObservedObject struct {
	ID   string      `json:"id"`
	Type string      `json:"type"`
	Pos  world.Point `json:"pos"`
}

type ObservedResource struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Pos        world.Point `json:"pos"`
	IsDepleted bool        `json:"is_depleted"`
}

type ObservedThreat struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Pos         world.Point `json:"pos"`
	DangerScore int         `json:"danger_score"`
}
