package world

type Zone string

const (
	ZoneSafe   Zone = "safe"
	ZoneForest Zone = "forest"
	ZoneQuarry Zone = "quarry"
	ZoneWild   Zone = "wild"
)

type Biome string

const (
	BiomePlain     Biome = "plain"
	BiomeForest    Biome = "forest"
	BiomeMountain  Biome = "mountain"
	BiomeWasteland Biome = "wasteland"
)

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
