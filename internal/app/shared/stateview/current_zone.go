package stateview

import (
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func CurrentZoneAtPosition(pos survival.Position, tiles []world.Tile) string {
	for _, tile := range tiles {
		if tile.X == pos.X && tile.Y == pos.Y {
			return string(tile.Zone)
		}
	}
	return ""
}
