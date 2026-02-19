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
	SessionID         string             `json:"session_id,omitempty"`
	Vitals            Vitals             `json:"vitals"`
	Position          Position           `json:"position"`
	CurrentZone       string             `json:"current_zone,omitempty"`
	Home              Position           `json:"home"`
	Inventory         map[string]int     `json:"inventory"`
	InventoryCapacity int                `json:"inventory_capacity"`
	InventoryUsed     int                `json:"inventory_used"`
	ActionCooldowns   map[string]int     `json:"action_cooldowns,omitempty"`
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
	Type        ActionType   `json:"type"`
	Direction   string       `json:"direction,omitempty"`
	TargetID    string       `json:"target_id,omitempty"`
	RecipeID    int          `json:"recipe_id,omitempty"`
	Count       int          `json:"count,omitempty"`
	ObjectType  string       `json:"object_type,omitempty"`
	Pos         *Position    `json:"pos,omitempty"`
	ItemType    string       `json:"item_type,omitempty"`
	RestMinutes int          `json:"rest_minutes,omitempty"`
	BedID       string       `json:"bed_id,omitempty"`
	BedQuality  string       `json:"-"`
	FarmID      string       `json:"farm_id,omitempty"`
	ContainerID string       `json:"container_id,omitempty"`
	Items       []ItemAmount `json:"items,omitempty"`
	DX          int          `json:"-"`
	DY          int          `json:"-"`
}

type ItemAmount struct {
	ItemType string `json:"item_type"`
	Count    int    `json:"count"`
}

type HeartbeatDelta struct {
	Minutes int `json:"minutes"`
}

type ResultCode string

const (
	ResultOK       ResultCode = "OK"
	ResultFailed   ResultCode = "FAILED"
	ResultGameOver ResultCode = "FAILED"
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
	WorldTimeSeconds  int64          `json:"world_time_seconds"`
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
	DeathCauseThreat     DeathCause = "threat"
)
