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
	AgentID   string
	Vitals    Vitals
	Position  Position
	Version   int64
	UpdatedAt time.Time
}

type ActionType string

const (
	ActionGather ActionType = "gather"
	ActionRest   ActionType = "rest"
	ActionMove   ActionType = "move"
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
