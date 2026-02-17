package world

type Snapshot struct {
	TimeOfDay          string
	ThreatLevel        int
	VisibilityPenalty  int
	NearbyResource     map[string]int
	Center             Point
	ViewRadius         int
	VisibleTiles       []Tile
	NextPhaseInSeconds int
	PhaseChanged       bool
	PhaseFrom          string
	PhaseTo            string
}
