package status

import "clawverse/internal/domain/survival"

type Request struct {
	AgentID string
}

type Response struct {
	State              survival.AgentStateAggregate `json:"state"`
	TimeOfDay          string                       `json:"time_of_day"`
	NextPhaseInSeconds int                          `json:"next_phase_in_seconds"`
}
