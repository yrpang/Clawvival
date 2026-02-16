package mock

import (
	"context"

	"clawverse/internal/domain/world"
)

type Provider struct {
	Snapshot world.Snapshot
}

func (p Provider) SnapshotForAgent(_ context.Context, _ string, center world.Point) (world.Snapshot, error) {
	s := p.Snapshot
	s.Center = center
	return s, nil
}
