package world

type Snapshot struct {
	TimeOfDay      string
	ThreatLevel    int
	NearbyResource map[string]int
	Center         Point
	ViewRadius     int
	VisibleTiles   []Tile
}
