package observe

import (
	"context"
	"errors"
	"strings"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/world"
)

var ErrInvalidRequest = errors.New("invalid observe request")

const (
	fixedViewRadius = 5
	fixedViewSize   = fixedViewRadius*2 + 1
)

type UseCase struct {
	StateRepo ports.AgentStateRepository
	World     ports.WorldProvider
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	state, err := u.StateRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return Response{}, err
	}
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return Response{}, err
	}
	return Response{
		State:    state,
		Snapshot: snapshot,
		View: View{
			Width:  fixedViewSize,
			Height: fixedViewSize,
			Center: world.Point{X: state.Position.X, Y: state.Position.Y},
			Radius: fixedViewRadius,
		},
		World: WorldMeta{
			Rules: Rules{StandardTickMinutes: 30},
		},
		ActionCosts: map[string]ActionCost{
			"move":               {BaseMinutes: 1},
			"gather":             {BaseMinutes: 5},
			"craft":              {BaseMinutes: 2},
			"build":              {BaseMinutes: 3},
			"eat":                {BaseMinutes: 1},
			"rest":               {BaseMinutes: 30},
			"sleep":              {BaseMinutes: 60},
			"farm_plant":         {BaseMinutes: 2},
			"farm_harvest":       {BaseMinutes: 2},
			"container_deposit":  {BaseMinutes: 1},
			"container_withdraw": {BaseMinutes: 1},
			"retreat":            {BaseMinutes: 1},
		},
	}, nil
}
