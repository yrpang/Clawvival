package status

import "clawvival/internal/domain/survival"

type Request struct {
	AgentID string
}

type Response struct {
	State              survival.AgentStateAggregate `json:"state"`
	WorldTimeSeconds   int64                        `json:"world_time_seconds"`
	TimeOfDay          string                       `json:"time_of_day"`
	NextPhaseInSeconds int                          `json:"next_phase_in_seconds"`
	World              WorldMeta                    `json:"world"`
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
	FarmGrowMinutes  int     `json:"farm_grow_minutes"`
	WheatYieldMin    int     `json:"wheat_yield_min"`
	WheatYieldMax    int     `json:"wheat_yield_max"`
	SeedReturnChance float64 `json:"seed_return_chance"`
}

type Seed struct {
	SeedDropChance   float64 `json:"seed_drop_chance"`
	SeedPityMaxFails int     `json:"seed_pity_max_fails"`
}
