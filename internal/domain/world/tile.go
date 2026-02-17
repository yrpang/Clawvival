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
	X          int      `json:"x"`
	Y          int      `json:"y"`
	Kind       TileKind `json:"kind"`
	Zone       Zone     `json:"zone"`
	Biome      Biome    `json:"biome"`
	Passable   bool     `json:"passable"`
	Resource   string   `json:"resource,omitempty"`
	BaseThreat int      `json:"base_threat"`
}
