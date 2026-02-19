package status

import "clawvival/internal/domain/survival"

type Request struct {
	AgentID string
}

type Response struct {
	State              survival.AgentStateAggregate `json:"agent_state"`
	WorldTimeSeconds   int64                        `json:"world_time_seconds"`
	TimeOfDay          string                       `json:"time_of_day"`
	NextPhaseInSeconds int                          `json:"next_phase_in_seconds"`
	HPDrainFeedback    HPDrainFeedback              `json:"hp_drain_feedback"`
	World              WorldMeta                    `json:"world"`
	ActionCosts        map[string]ActionCost        `json:"action_costs"`
}

type WorldMeta struct {
	Rules Rules `json:"rules"`
}

type Rules struct {
	StandardTickMinutes int          `json:"standard_tick_minutes"`
	DrainsPer30m        DrainsPer30m `json:"drains_per_30m"`
	Thresholds          Thresholds   `json:"thresholds"`
	Visibility          Visibility   `json:"visibility"`
	Farming             Farming      `json:"farming"`
	Seed                Seed         `json:"seed"`
}

type DrainsPer30m struct {
	HungerDrain            int     `json:"hunger_drain"`
	EnergyDrain            int     `json:"energy_drain"`
	HPDrainModel           string  `json:"hp_drain_model"`
	HPDrainFromHungerCoeff float64 `json:"hp_drain_from_hunger_coeff"`
	HPDrainFromEnergyCoeff float64 `json:"hp_drain_from_energy_coeff"`
	HPDrainCap             int     `json:"hp_drain_cap"`
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
	WheatYieldRange  []int   `json:"wheat_yield_range"`
	SeedReturnChance float64 `json:"seed_return_chance"`
}

type Seed struct {
	SeedDropChance   float64 `json:"seed_drop_chance"`
	SeedPityMaxFails int     `json:"seed_pity_max_fails"`
}

type ActionCost struct {
	BaseMinutes  int                          `json:"base_minutes"`
	DeltaHunger  int                          `json:"delta_hunger"`
	DeltaEnergy  int                          `json:"delta_energy"`
	DeltaHP      int                          `json:"delta_hp,omitempty"`
	Requirements []string                     `json:"requirements"`
	Variants     map[string]ActionCostVariant `json:"variants,omitempty"`
}

type ActionCostVariant struct {
	DeltaHunger int `json:"delta_hunger"`
	DeltaEnergy int `json:"delta_energy"`
	DeltaHP     int `json:"delta_hp,omitempty"`
}

type HPDrainFeedback struct {
	IsLosingHP         bool     `json:"is_losing_hp"`
	EstimatedLossPer30 int      `json:"estimated_loss_per_30m"`
	HungerComponent    int      `json:"hunger_component"`
	EnergyComponent    int      `json:"energy_component"`
	CapPer30           int      `json:"cap_per_30m"`
	Causes             []string `json:"causes"`
}
