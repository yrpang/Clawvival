package world

type ChunkCoord struct {
	X int
	Y int
}

type Chunk struct {
	Coord ChunkCoord
	Tiles []Tile
}
