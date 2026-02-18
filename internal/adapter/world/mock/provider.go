package mock

import (
	"context"

	"clawvival/internal/domain/world"
)

type Provider struct {
	Snapshot world.Snapshot
}

func (p Provider) SnapshotForAgent(_ context.Context, _ string, center world.Point) (world.Snapshot, error) {
	s := p.Snapshot
	s.Center = center
	if len(s.VisibleTiles) == 0 {
		s.VisibleTiles = []world.Tile{{
			X:        center.X,
			Y:        center.Y,
			Kind:     world.TileGrass,
			Passable: true,
			Resource: "wood",
		}}
	}
	return s, nil
}
