package observe

import (
	"context"
	"errors"
	"fmt"
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
		Tiles:            projectTiles(snapshot.VisibleTiles, snapshot.TimeOfDay),
		Objects:          []ObservedObject{},
		Resources:        projectResources(snapshot.VisibleTiles),
		Threats:          projectThreats(snapshot.VisibleTiles),
		LocalThreatLevel: snapshot.ThreatLevel,
	}, nil
}

func projectTiles(tiles []world.Tile, timeOfDay string) []ObservedTile {
	isLit := timeOfDay == "day"
	out := make([]ObservedTile, 0, len(tiles))
	for _, t := range tiles {
		out = append(out, ObservedTile{
			Pos:         world.Point{X: t.X, Y: t.Y},
			TerrainType: string(t.Kind),
			IsWalkable:  t.Passable,
			IsLit:       isLit,
			IsVisible:   true,
		})
	}
	return out
}

func projectResources(tiles []world.Tile) []ObservedResource {
	out := make([]ObservedResource, 0, len(tiles))
	for _, t := range tiles {
		if t.Resource == "" {
			continue
		}
		out = append(out, ObservedResource{
			ID:         fmt.Sprintf("res_%d_%d_%s", t.X, t.Y, t.Resource),
			Type:       t.Resource,
			Pos:        world.Point{X: t.X, Y: t.Y},
			IsDepleted: false,
		})
	}
	return out
}

func projectThreats(tiles []world.Tile) []ObservedThreat {
	out := make([]ObservedThreat, 0, len(tiles))
	for _, t := range tiles {
		if t.BaseThreat <= 0 {
			continue
		}
		out = append(out, ObservedThreat{
			ID:          fmt.Sprintf("thr_%d_%d", t.X, t.Y),
			Type:        "wild",
			Pos:         world.Point{X: t.X, Y: t.Y},
			DangerScore: min(100, t.BaseThreat*25),
		})
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
