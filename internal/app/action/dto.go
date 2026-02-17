package action

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID        string
	IdempotencyKey string
	Intent         survival.ActionIntent
	DeltaMinutes   int
	StrategyHash   string
}

type Response struct {
	UpdatedState survival.AgentStateAggregate `json:"updated_state"`
	Events       []survival.DomainEvent       `json:"events"`
	ResultCode   survival.ResultCode          `json:"result_code"`
}
