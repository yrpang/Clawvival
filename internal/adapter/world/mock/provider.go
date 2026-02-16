package mock

import (
	"context"

	"clawverse/internal/domain/world"
)

type Provider struct {
	Snapshot world.Snapshot
}

func (p Provider) SnapshotForAgent(_ context.Context, _ string) (world.Snapshot, error) {
	return p.Snapshot, nil
}
