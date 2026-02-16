package world

type TileKind string

const (
	TileGrass TileKind = "grass"
	TileTree  TileKind = "tree"
	TileRock  TileKind = "rock"
	TileDirt  TileKind = "dirt"
	TileWater TileKind = "water"
)

type Tile struct {
	X          int
	Y          int
	Kind       TileKind
	Zone       Zone
	Biome      Biome
	Passable   bool
	Resource   string
	BaseThreat int
}
