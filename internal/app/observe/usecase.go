package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/shared/cooldown"
	"clawvival/internal/app/shared/resourcestate"
	"clawvival/internal/app/shared/stateview"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

var ErrInvalidRequest = errors.New("invalid observe request")

const (
	fixedViewRadius   = 5
	fixedViewSize     = fixedViewRadius*2 + 1
	nightVisionRadius = survival.VisionRadiusNight
)

type UseCase struct {
	StateRepo    ports.AgentStateRepository
	ObjectRepo   ports.WorldObjectRepository
	EventRepo    ports.EventRepository
	ResourceRepo ports.AgentResourceNodeRepository
	World        ports.WorldProvider
	Settle       survival.SettlementService
	Now          func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	state, err := u.StateRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return Response{}, err
	}
	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowAt := nowFn()
	state, err = u.settleBeforeObserve(ctx, req.AgentID, state, nowAt)
	if err != nil {
		return Response{}, err
	}
	state.SessionID = "session-" + req.AgentID
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return Response{}, err
	}
	depleted, err := loadDepletedTargets(ctx, u.ResourceRepo, req.AgentID, nowAt)
	if err != nil {
		return Response{}, err
	}
	events := []survival.DomainEvent{}
	if u.EventRepo != nil {
		events, err = u.EventRepo.ListByAgentID(ctx, req.AgentID, 50)
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return Response{}, err
		}
	}
	applyDepletedResourcesToSnapshot(&snapshot, depleted)
	state = stateview.Enrich(state, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))
	state.CurrentZone = stateview.CurrentZoneAtPosition(state.Position, snapshot.VisibleTiles)
	state.ActionCooldowns = cooldown.RemainingByAction(events, nowFn())
	tiles := buildWindowTiles(world.Point{X: state.Position.X, Y: state.Position.Y}, snapshot.TimeOfDay, snapshot.VisibleTiles)
	objects := []ObservedObject{}
	if u.ObjectRepo != nil {
		rows, err := u.ObjectRepo.ListByAgentID(ctx, req.AgentID)
		if err != nil {
			return Response{}, err
		}
		objects = projectObjects(tiles, rows)
	}
	resources := projectResources(tiles, depleted)
	snapshot.NearbyResource = summarizeNearby(resources)
	return Response{
		State:              state,
		Snapshot:           snapshot,
		WorldTimeSeconds:   snapshot.WorldTimeSeconds,
		TimeOfDay:          snapshot.TimeOfDay,
		NextPhaseInSeconds: snapshot.NextPhaseInSeconds,
		HPDrainFeedback:    toHPDrainFeedback(stateview.EstimateHPDrain(state.Vitals, survival.StandardTickMinutes)),
		View: View{
			Width:  fixedViewSize,
			Height: fixedViewSize,
			Center: world.Point{X: state.Position.X, Y: state.Position.Y},
			Radius: fixedViewRadius,
		},
		World: WorldMeta{
			Rules: defaultRules(),
		},
		ActionCosts:      defaultActionCosts(),
		Tiles:            tiles,
		Objects:          objects,
		Resources:        resources,
		Threats:          projectThreats(tiles),
		LocalThreatLevel: snapshot.ThreatLevel,
	}, nil
}

func (u UseCase) settleBeforeObserve(ctx context.Context, agentID string, state survival.AgentStateAggregate, nowAt time.Time) (survival.AgentStateAggregate, error) {
	if state.Dead {
		return state, nil
	}

	settledOngoing := false
	ongoing := state.OngoingAction
	if ongoing != nil {
		if nowAt.Before(ongoing.EndAt) {
			return state, nil
		}

		startAt := ongoing.EndAt.Add(-time.Duration(ongoing.Minutes) * time.Minute)
		deltaMinutes := int(nowAt.Sub(startAt).Minutes())
		if deltaMinutes < 0 {
			deltaMinutes = 0
		}
		if deltaMinutes > ongoing.Minutes {
			deltaMinutes = ongoing.Minutes
		}

		snapshot, err := u.World.SnapshotForAgent(ctx, agentID, world.Point{X: state.Position.X, Y: state.Position.Y})
		if err != nil {
			return survival.AgentStateAggregate{}, err
		}

		result := survival.SettlementResult{
			UpdatedState: state,
			Events:       []survival.DomainEvent{},
			ResultCode:   survival.ResultOK,
		}
		if deltaMinutes > 0 {
			intent := survival.ActionIntent{Type: ongoing.Type}
			if ongoing.Type == survival.ActionSleep {
				intent.BedID = ongoing.BedID
				intent.BedQuality = ongoing.Quality
			}
			result, err = u.Settle.Settle(
				state,
				intent,
				survival.HeartbeatDelta{Minutes: deltaMinutes},
				nowAt,
				survival.WorldSnapshot{
					TimeOfDay:         snapshot.TimeOfDay,
					ThreatLevel:       snapshot.ThreatLevel,
					VisibilityPenalty: snapshot.VisibilityPenalty,
					NearbyResource:    snapshot.NearbyResource,
					WorldTimeSeconds:  snapshot.WorldTimeSeconds,
				},
			)
			if err != nil {
				return survival.AgentStateAggregate{}, err
			}
		}
		result.UpdatedState.OngoingAction = nil
		result.UpdatedState.UpdatedAt = nowAt

		sessionID := "session-" + agentID
		for i := range result.Events {
			if result.Events[i].Payload == nil {
				result.Events[i].Payload = map[string]any{}
			}
			result.Events[i].Payload["agent_id"] = agentID
			result.Events[i].Payload["session_id"] = sessionID
		}
		result.Events = append(result.Events, survival.DomainEvent{
			Type:       "ongoing_action_ended",
			OccurredAt: nowAt,
			Payload: map[string]any{
				"agent_id":        agentID,
				"session_id":      sessionID,
				"action_type":     string(ongoing.Type),
				"planned_minutes": ongoing.Minutes,
				"forced":          false,
			},
		})

		if err := u.StateRepo.SaveWithVersion(ctx, result.UpdatedState, state.Version); err != nil {
			return survival.AgentStateAggregate{}, err
		}
		if u.EventRepo != nil {
			if err := u.EventRepo.Append(ctx, agentID, result.Events); err != nil {
				return survival.AgentStateAggregate{}, err
			}
		}
		state = result.UpdatedState
		settledOngoing = true
	}

	if settledOngoing {
		return state, nil
	}
	if state.UpdatedAt.IsZero() || !nowAt.After(state.UpdatedAt) {
		return state, nil
	}
	elapsedMinutes := int(nowAt.Sub(state.UpdatedAt).Minutes())
	ticks := elapsedMinutes / survival.StandardTickMinutes
	if ticks <= 0 {
		return state, nil
	}
	deltaMinutes := ticks * survival.StandardTickMinutes
	settleAt := state.UpdatedAt.Add(time.Duration(deltaMinutes) * time.Minute)

	snapshot, err := u.World.SnapshotForAgent(ctx, agentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return survival.AgentStateAggregate{}, err
	}
	result, err := u.Settle.Settle(
		state,
		survival.ActionIntent{Type: survival.ActionTerminate},
		survival.HeartbeatDelta{Minutes: deltaMinutes},
		settleAt,
		survival.WorldSnapshot{
			TimeOfDay:         snapshot.TimeOfDay,
			ThreatLevel:       snapshot.ThreatLevel,
			VisibilityPenalty: snapshot.VisibilityPenalty,
			NearbyResource:    snapshot.NearbyResource,
			WorldTimeSeconds:  snapshot.WorldTimeSeconds,
		},
	)
	if err != nil {
		return survival.AgentStateAggregate{}, err
	}
	result.UpdatedState.UpdatedAt = settleAt
	if err := u.StateRepo.SaveWithVersion(ctx, result.UpdatedState, state.Version); err != nil {
		return survival.AgentStateAggregate{}, err
	}
	if u.EventRepo != nil {
		sessionID := "session-" + agentID
		for i := range result.Events {
			if result.Events[i].Payload == nil {
				result.Events[i].Payload = map[string]any{}
			}
			result.Events[i].Payload["agent_id"] = agentID
			result.Events[i].Payload["session_id"] = sessionID
		}
		if err := u.EventRepo.Append(ctx, agentID, result.Events); err != nil {
			return survival.AgentStateAggregate{}, err
		}
	}
	return result.UpdatedState, nil
}

func defaultActionCosts() map[string]ActionCost {
	profiles := survival.DefaultActionCostProfiles()
	out := make(map[string]ActionCost, len(profiles))
	for action, profile := range profiles {
		variants := map[string]ActionCostVariant{}
		if len(profile.Variants) > 0 {
			variants = make(map[string]ActionCostVariant, len(profile.Variants))
			for key, variant := range profile.Variants {
				variants[key] = ActionCostVariant{
					DeltaHunger: variant.DeltaHunger,
					DeltaEnergy: variant.DeltaEnergy,
					DeltaHP:     variant.DeltaHP,
				}
			}
		}
		out[string(action)] = ActionCost{
			DeltaHunger:  profile.DeltaHunger,
			DeltaEnergy:  profile.DeltaEnergy,
			DeltaHP:      profile.DeltaHP,
			Requirements: append([]string(nil), profile.Requirements...),
			Variants:     variants,
		}
	}
	return out
}

func toHPDrainFeedback(in stateview.HPDrainEstimate) HPDrainFeedback {
	return HPDrainFeedback{
		IsLosingHP:         in.IsLosingHP,
		EstimatedLossPer30: in.EstimatedLoss,
		HungerComponent:    in.HungerComponent,
		EnergyComponent:    in.EnergyComponent,
		CapPer30:           in.Cap,
		Causes:             in.Causes,
	}
}

func defaultRules() Rules {
	return Rules{
		StandardTickMinutes: survival.StandardTickMinutes,
		DrainsPer30m: DrainsPer30m{
			HungerDrain:            survival.BaseHungerDrainPer30,
			EnergyDrain:            0,
			HPDrainModel:           "dynamic_capped",
			HPDrainFromHungerCoeff: survival.HPDrainFromHungerCoeff,
			HPDrainFromEnergyCoeff: survival.HPDrainFromEnergyCoeff,
			HPDrainCap:             survival.HPDrainCapPer30,
		},
		Thresholds: Thresholds{
			CriticalHP: survival.CriticalHPThreshold,
			LowEnergy:  survival.LowEnergyThreshold,
		},
		Visibility: Visibility{
			VisionRadiusDay:   survival.VisionRadiusDay,
			VisionRadiusNight: survival.VisionRadiusNight,
			TorchLightRadius:  survival.TorchLightRadius,
		},
		Farming: Farming{
			FarmGrowMinutes:  survival.DefaultFarmGrowMinutes,
			WheatYieldRange:  []int{survival.WheatYieldMin, survival.WheatYieldMax},
			SeedReturnChance: survival.SeedReturnChance,
		},
		Seed: Seed{
			SeedDropChance:   survival.SeedDropChance,
			SeedPityMaxFails: survival.SeedPityMaxFails,
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

func projectResources(tiles []ObservedTile, depleted map[string]int) []ObservedResource {
	out := make([]ObservedResource, 0, len(tiles))
	for _, t := range tiles {
		if !t.IsVisible || t.ResourceType == "" {
			continue
		}
		id := resourcestate.BuildResourceTargetID(t.Pos.X, t.Pos.Y, t.ResourceType)
		if depleted[strings.TrimSpace(id)] > 0 {
			continue
		}
		out = append(out, ObservedResource{
			ID:         id,
			Type:       t.ResourceType,
			Pos:        t.Pos,
			IsDepleted: false,
		})
	}
	return out
}

func summarizeNearby(resources []ObservedResource) map[string]int {
	out := map[string]int{}
	for _, res := range resources {
		out[res.Type]++
	}
	return out
}

func applyDepletedResourcesToSnapshot(snapshot *world.Snapshot, depleted map[string]int) {
	if snapshot == nil || len(snapshot.VisibleTiles) == 0 || len(depleted) == 0 {
		return
	}
	for i := range snapshot.VisibleTiles {
		tile := &snapshot.VisibleTiles[i]
		if strings.TrimSpace(tile.Resource) == "" {
			continue
		}
		targetID := resourcestate.BuildResourceTargetID(tile.X, tile.Y, tile.Resource)
		if depleted[strings.TrimSpace(targetID)] <= 0 {
			continue
		}
		tile.Resource = ""
	}
}

func loadDepletedTargets(ctx context.Context, repo ports.AgentResourceNodeRepository, agentID string, now time.Time) (map[string]int, error) {
	if repo == nil {
		return map[string]int{}, nil
	}
	rows, err := repo.ListByAgentID(ctx, agentID)
	if errors.Is(err, ports.ErrNotFound) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, row := range rows {
		if !row.DepletedUntil.After(now) {
			continue
		}
		remaining := int(row.DepletedUntil.Sub(now).Seconds())
		if remaining < 1 {
			remaining = 1
		}
		out[row.TargetID] = remaining
	}
	return out, nil
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

func isCurrentTileLit(timeOfDay string) bool {
	return strings.EqualFold(strings.TrimSpace(timeOfDay), "day")
}
