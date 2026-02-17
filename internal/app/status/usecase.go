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
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID, world.Point{X: state.Position.X, Y: state.Position.Y})
	if err != nil {
		return Response{}, err
	}
	state = stateview.Enrich(state, snapshot.TimeOfDay)
	return Response{
		State:              state,
		WorldTimeSeconds:   snapshot.WorldTimeSeconds,
		TimeOfDay:          snapshot.TimeOfDay,
		NextPhaseInSeconds: snapshot.NextPhaseInSeconds,
		World: WorldMeta{
			Rules: defaultRules(),
		},
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
