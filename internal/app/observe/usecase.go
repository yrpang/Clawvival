package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/stateview"
	"clawvival/internal/domain/world"
)

var ErrInvalidRequest = errors.New("invalid observe request")

const (
	fixedViewRadius = 5
	fixedViewSize   = fixedViewRadius*2 + 1
	nightVisionRadius = 3
)

type UseCase struct {
	StateRepo  ports.AgentStateRepository
	ObjectRepo ports.WorldObjectRepository
	World      ports.WorldProvider
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
	state = stateview.Enrich(state, snapshot.TimeOfDay)
	tiles := buildWindowTiles(world.Point{X: state.Position.X, Y: state.Position.Y}, snapshot.TimeOfDay, snapshot.VisibleTiles)
	objects := []ObservedObject{}
	if u.ObjectRepo != nil {
		rows, err := u.ObjectRepo.ListByAgentID(ctx, req.AgentID)
		if err != nil {
			return Response{}, err
		}
		objects = projectObjects(tiles, rows)
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
			Rules: defaultRules(),
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
		Tiles:            tiles,
		Objects:          objects,
		Resources:        projectResources(tiles),
		Threats:          projectThreats(tiles),
		LocalThreatLevel: snapshot.ThreatLevel,
	}, nil
}

func defaultRules() Rules {
	return Rules{
		StandardTickMinutes: 30,
		DrainsPer30m: DrainsPer30m{
			HungerDrain:     5,
			EnergyDrain:     4,
			HPDrainStarving: 8,
		},
		Thresholds: Thresholds{
			CriticalHP: 15,
			LowEnergy:  20,
		},
		Visibility: Visibility{
			VisionRadiusDay:   6,
			VisionRadiusNight: 3,
			TorchLightRadius:  3,
		},
		Farming: Farming{
			FarmGrowMinutes:  60,
			WheatYieldMin:    1,
			WheatYieldMax:    3,
			SeedReturnChance: 0.2,
		},
		Seed: Seed{
			SeedDropChance:   0.2,
			SeedPityMaxFails: 8,
		},
	}
}

func projectTiles(tiles []world.Tile, timeOfDay string) []ObservedTile {
	isLit := timeOfDay == "day"
	out := make([]ObservedTile, 0, len(tiles))
	for _, t := range tiles {
		out = append(out, ObservedTile{
			Pos:          world.Point{X: t.X, Y: t.Y},
			TerrainType:  string(t.Kind),
			IsWalkable:   t.Passable,
			IsLit:        isLit,
			IsVisible:    true,
			ResourceType: t.Resource,
			BaseThreat:   t.BaseThreat,
		})
	}
	return out
}

func projectResources(tiles []ObservedTile) []ObservedResource {
	out := make([]ObservedResource, 0, len(tiles))
	for _, t := range tiles {
		if !t.IsVisible || t.ResourceType == "" {
			continue
		}
		out = append(out, ObservedResource{
			ID:         fmt.Sprintf("res_%d_%d_%s", t.Pos.X, t.Pos.Y, t.ResourceType),
			Type:       t.ResourceType,
			Pos:        t.Pos,
			IsDepleted: false,
		})
	}
	return out
}

func projectThreats(tiles []ObservedTile) []ObservedThreat {
	out := make([]ObservedThreat, 0, len(tiles))
	for _, t := range tiles {
		if !t.IsVisible || t.BaseThreat <= 0 {
			continue
		}
		out = append(out, ObservedThreat{
			ID:          fmt.Sprintf("thr_%d_%d", t.Pos.X, t.Pos.Y),
			Type:        "wild",
			Pos:         t.Pos,
			DangerScore: min(100, t.BaseThreat*25),
		})
	}
	return out
}

func projectObjects(tiles []ObservedTile, objects []ports.WorldObjectRecord) []ObservedObject {
	visible := map[string]bool{}
	for _, t := range tiles {
		if t.IsVisible {
			visible[posKey(t.Pos.X, t.Pos.Y)] = true
		}
	}
	out := make([]ObservedObject, 0, len(objects))
	for _, obj := range objects {
		if !visible[posKey(obj.X, obj.Y)] {
			continue
		}
		entry := ObservedObject{
			ID:            obj.ObjectID,
			Type:          normalizeObjectType(obj),
			Quality:       strings.ToUpper(strings.TrimSpace(obj.Quality)),
			Pos:           world.Point{X: obj.X, Y: obj.Y},
			CapacitySlots: obj.CapacitySlots,
			UsedSlots:     obj.UsedSlots,
		}
		if state := extractObjectState(obj); state != "" {
			entry.State = state
		}
		out = append(out, entry)
	}
	return out
}

func normalizeObjectType(obj ports.WorldObjectRecord) string {
	if t := strings.TrimSpace(obj.ObjectType); t != "" {
		return t
	}
	switch obj.Kind {
	case 1:
		return "bed"
	case 2:
		return "box"
	case 3:
		return "farm_plot"
	default:
		return "unknown"
	}
}

func extractObjectState(obj ports.WorldObjectRecord) string {
	if strings.TrimSpace(obj.ObjectState) == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(obj.ObjectState), &raw); err != nil {
		return ""
	}
	state, _ := raw["state"].(string)
	return strings.ToUpper(strings.TrimSpace(state))
}

func buildWindowTiles(center world.Point, timeOfDay string, visible []world.Tile) []ObservedTile {
	isLit := timeOfDay == "day"
	visionRadius := fixedViewRadius
	if !isLit {
		visionRadius = nightVisionRadius
	}
	visibleByPos := make(map[string]world.Tile, len(visible))
	for _, tile := range visible {
		visibleByPos[posKey(tile.X, tile.Y)] = tile
	}

	out := make([]ObservedTile, 0, fixedViewSize*fixedViewSize)
	for y := center.Y - fixedViewRadius; y <= center.Y+fixedViewRadius; y++ {
		for x := center.X - fixedViewRadius; x <= center.X+fixedViewRadius; x++ {
			tile, ok := visibleByPos[posKey(x, y)]
			if !ok {
				out = append(out, ObservedTile{
					Pos:         world.Point{X: x, Y: y},
					TerrainType: "unknown",
					IsWalkable:  false,
					IsLit:       false,
					IsVisible:   false,
				})
				continue
			}
			dist := abs(x-center.X) + abs(y-center.Y)
			isVisible := dist <= visionRadius
			out = append(out, ObservedTile{
				Pos:          world.Point{X: x, Y: y},
				TerrainType:  string(tile.Kind),
				IsWalkable:   tile.Passable,
				IsLit:        isLit,
				IsVisible:    isVisible,
				ResourceType: tile.Resource,
				BaseThreat:   tile.BaseThreat,
			})
		}
	}
	return out
}

func posKey(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
