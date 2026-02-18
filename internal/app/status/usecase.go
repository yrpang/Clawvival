package status

import (
	"context"
	"errors"
	"strings"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/stateview"
	"clawvival/internal/domain/world"
)

var ErrInvalidRequest = errors.New("invalid status request")

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
	state.SessionID = "session-" + req.AgentID
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return Response{}, err
	}
	state = stateview.Enrich(state, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))
	return Response{
		State:              state,
		WorldTimeSeconds:   snapshot.WorldTimeSeconds,
		TimeOfDay:          snapshot.TimeOfDay,
		NextPhaseInSeconds: snapshot.NextPhaseInSeconds,
		HPDrainFeedback:    toHPDrainFeedback(stateview.EstimateHPDrain(state.Vitals, 30)),
		World: WorldMeta{
			Rules: defaultRules(),
		},
		ActionCosts: defaultActionCosts(),
	}, nil
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
		StandardTickMinutes: 30,
		DrainsPer30m: DrainsPer30m{
			HungerDrain:            4,
			EnergyDrain:            0,
			HPDrainModel:           "dynamic_capped",
			HPDrainFromHungerCoeff: 0.08,
			HPDrainFromEnergyCoeff: 0.05,
			HPDrainCap:             12,
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
			WheatYieldRange:  []int{1, 3},
			SeedReturnChance: 0.2,
		},
		Seed: Seed{
			SeedDropChance:   0.2,
			SeedPityMaxFails: 8,
		},
	}
}

func defaultActionCosts() map[string]ActionCost {
	return map[string]ActionCost{
		"move":               {BaseMinutes: 1, DeltaHunger: -1, DeltaEnergy: -6, Requirements: []string{"PASSABLE_TILE"}},
		"gather":             {BaseMinutes: 5, DeltaHunger: -3, DeltaEnergy: -18, Requirements: []string{"VISIBLE_TARGET"}},
		"craft":              {BaseMinutes: 2, DeltaHunger: 0, DeltaEnergy: -12, Requirements: []string{"RECIPE_INPUTS"}},
		"build":              {BaseMinutes: 3, DeltaHunger: 0, DeltaEnergy: -14, Requirements: []string{"BUILD_MATERIALS", "VALID_POS"}},
		"eat":                {BaseMinutes: 1, DeltaHunger: 12, DeltaEnergy: 0, Requirements: []string{"HAS_ITEM"}},
		"rest":               {BaseMinutes: 30, DeltaHunger: 0, DeltaEnergy: 10, Requirements: []string{}},
		"sleep":              {BaseMinutes: 60, DeltaHunger: 0, DeltaEnergy: 18, Requirements: []string{"BED_ID"}},
		"farm_plant":         {BaseMinutes: 2, DeltaHunger: -1, DeltaEnergy: -10, Requirements: []string{"FARM_ID", "HAS_SEED"}},
		"farm_harvest":       {BaseMinutes: 2, DeltaHunger: 0, DeltaEnergy: -8, Requirements: []string{"FARM_ID", "FARM_READY"}},
		"container_deposit":  {BaseMinutes: 1, DeltaHunger: 0, DeltaEnergy: -4, Requirements: []string{"CONTAINER_ID", "HAS_ITEMS"}},
		"container_withdraw": {BaseMinutes: 1, DeltaHunger: 0, DeltaEnergy: -4, Requirements: []string{"CONTAINER_ID", "CAPACITY_AVAILABLE"}},
		"retreat":            {BaseMinutes: 1, DeltaHunger: 0, DeltaEnergy: -8, Requirements: []string{}},
		"terminate":          {BaseMinutes: 1, DeltaHunger: 0, DeltaEnergy: 0, Requirements: []string{"INTERRUPTIBLE_ONGOING_ACTION"}},
	}
}

func isCurrentTileLit(timeOfDay string) bool {
	return strings.EqualFold(strings.TrimSpace(timeOfDay), "day")
}
