package ports

import (
	"context"

	"clawverse/internal/domain/world"
)

type WorldProvider interface {
	SnapshotForAgent(ctx context.Context, agentID string, center world.Point) (world.Snapshot, error)
}
