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
}
