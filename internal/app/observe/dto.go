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
	DrainsPer30m        DrainsPer30m `json:"drains_per_30m"`
	Thresholds          Thresholds   `json:"thresholds"`
	Visibility          Visibility   `json:"visibility"`
	Farming             Farming      `json:"farming"`
	Seed                Seed         `json:"seed"`
}

type DrainsPer30m struct {
	HungerDrain     int `json:"hunger_drain"`
	EnergyDrain     int `json:"energy_drain"`
	HPDrainStarving int `json:"hp_drain_starving"`
}

type Thresholds struct {
	CriticalHP int `json:"critical_hp"`
	LowEnergy  int `json:"low_energy"`
}

type Visibility struct {
	VisionRadiusDay   int `json:"vision_radius_day"`
	VisionRadiusNight int `json:"vision_radius_night"`
	TorchLightRadius  int `json:"torch_light_radius"`
}

type Farming struct {
	FarmGrowMinutes int   `json:"farm_grow_minutes"`
	WheatYieldMin   int   `json:"wheat_yield_min"`
	WheatYieldMax   int   `json:"wheat_yield_max"`
	SeedReturnChance float64 `json:"seed_return_chance"`
}

type Seed struct {
	SeedDropChance  float64 `json:"seed_drop_chance"`
	SeedPityMaxFails int    `json:"seed_pity_max_fails"`
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
