package survival

import "time"

type Vitals struct {
	HP     int `json:"hp"`
	Hunger int `json:"hunger"`
	Energy int `json:"energy"`
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type AgentStateAggregate struct {
	AgentID           string             `json:"agent_id"`
	Vitals            Vitals             `json:"vitals"`
	Position          Position           `json:"position"`
	Home              Position           `json:"home"`
	Inventory         map[string]int     `json:"inventory"`
	InventoryCapacity int                `json:"inventory_capacity"`
	InventoryUsed     int                `json:"inventory_used"`
	StatusEffects     []string           `json:"status_effects"`
	Dead              bool               `json:"dead"`
	DeathCause        DeathCause         `json:"death_cause"`
	OngoingAction     *OngoingActionInfo `json:"ongoing_action,omitempty"`
	Version           int64              `json:"version"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type OngoingActionInfo struct {
	Type    ActionType `json:"type"`
	Minutes int        `json:"minutes"`
	EndAt   time.Time  `json:"end_at"`
}

type ActionType string

const (
	ActionGather            ActionType = "gather"
	ActionRest              ActionType = "rest"
	ActionSleep             ActionType = "sleep"
	ActionMove              ActionType = "move"
	ActionCombat            ActionType = "combat"
	ActionBuild             ActionType = "build"
	ActionFarm              ActionType = "farm"
	ActionFarmPlant         ActionType = "farm_plant"
	ActionFarmHarvest       ActionType = "farm_harvest"
	ActionContainerDeposit  ActionType = "container_deposit"
	ActionContainerWithdraw ActionType = "container_withdraw"
	ActionRetreat           ActionType = "retreat"
	ActionCraft             ActionType = "craft"
	ActionEat               ActionType = "eat"
	ActionTerminate         ActionType = "terminate"
)

type ActionIntent struct {
	Type   ActionType     `json:"type"`
	Params map[string]int `json:"params,omitempty"`
}

type HeartbeatDelta struct {
	Minutes int `json:"minutes"`
}

type ResultCode string

const (
	ResultOK       ResultCode = "ok"
	ResultGameOver ResultCode = "game_over"
)

type DomainEvent struct {
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload"`
}

type WorldSnapshot struct {
	TimeOfDay         string         `json:"time_of_day"`
	ThreatLevel       int            `json:"threat_level"`
	VisibilityPenalty int            `json:"visibility_penalty"`
	NearbyResource    map[string]int `json:"nearby_resource"`
}

type SettlementResult struct {
	UpdatedState AgentStateAggregate `json:"updated_state"`
	Events       []DomainEvent       `json:"events"`
	ResultCode   ResultCode          `json:"result_code"`
}

type DeathCause string

const (
	DeathCauseUnknown    DeathCause = "unknown"
	DeathCauseStarvation DeathCause = "starvation"
	DeathCauseExhaustion DeathCause = "exhaustion"
	DeathCauseCombat     DeathCause = "combat"
)
