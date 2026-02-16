package survival

import "time"

type Vitals struct {
	HP     int
	Hunger int
	Energy int
}

type Position struct {
	X int
	Y int
}

type AgentStateAggregate struct {
	AgentID    string
	Vitals     Vitals
	Position   Position
	Home       Position
	Inventory  map[string]int
	Dead       bool
	DeathCause DeathCause
	Version    int64
	UpdatedAt  time.Time
}

type ActionType string

const (
	ActionGather  ActionType = "gather"
	ActionRest    ActionType = "rest"
	ActionMove    ActionType = "move"
	ActionCombat  ActionType = "combat"
	ActionBuild   ActionType = "build"
	ActionFarm    ActionType = "farm"
	ActionRetreat ActionType = "retreat"
	ActionCraft   ActionType = "craft"
)

type ActionIntent struct {
	Type   ActionType
	Params map[string]int
}

type HeartbeatDelta struct {
	Minutes int
}

type ResultCode string

const (
	ResultOK       ResultCode = "ok"
	ResultGameOver ResultCode = "game_over"
)

type DomainEvent struct {
	Type       string
	OccurredAt time.Time
	Payload    map[string]any
}

type WorldSnapshot struct {
	TimeOfDay      string
	ThreatLevel    int
	NearbyResource map[string]int
}

type SettlementResult struct {
	UpdatedState AgentStateAggregate
	Events       []DomainEvent
	ResultCode   ResultCode
}

type DeathCause string

const (
	DeathCauseUnknown    DeathCause = "unknown"
	DeathCauseStarvation DeathCause = "starvation"
	DeathCauseExhaustion DeathCause = "exhaustion"
	DeathCauseCombat     DeathCause = "combat"
)
