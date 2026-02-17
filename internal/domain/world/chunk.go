package world

type ChunkCoord struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Chunk struct {
	Coord ChunkCoord `json:"coord"`
	Tiles []Tile     `json:"tiles"`
}
