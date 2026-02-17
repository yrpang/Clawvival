package action

import "clawvival/internal/domain/survival"

type Request struct {
	AgentID        string
	IdempotencyKey string
	Intent         survival.ActionIntent
	StrategyHash   string
}

type Response struct {
	SettledDTMinutes       int                          `json:"settled_dt_minutes"`
	WorldTimeBeforeSeconds int64                        `json:"world_time_before_seconds"`
	WorldTimeAfterSeconds  int64                        `json:"world_time_after_seconds"`
	UpdatedState           survival.AgentStateAggregate `json:"updated_state"`
	Events                 []survival.DomainEvent       `json:"events"`
	ResultCode             survival.ResultCode          `json:"result_code"`
}
