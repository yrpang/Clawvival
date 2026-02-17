package world

type Snapshot struct {
	TimeOfDay          string         `json:"time_of_day"`
	ThreatLevel        int            `json:"threat_level"`
	VisibilityPenalty  int            `json:"visibility_penalty"`
	NearbyResource     map[string]int `json:"nearby_resource"`
	Center             Point          `json:"center"`
	ViewRadius         int            `json:"view_radius"`
	VisibleTiles       []Tile         `json:"visible_tiles"`
	NextPhaseInSeconds int            `json:"next_phase_in_seconds"`
	PhaseChanged       bool           `json:"phase_changed"`
	PhaseFrom          string         `json:"phase_from"`
	PhaseTo            string         `json:"phase_to"`
}
