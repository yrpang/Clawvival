package status

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/app/shared/cooldown"
	"clawvival/internal/app/shared/stateview"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

var ErrInvalidRequest = errors.New("invalid status request")

type UseCase struct {
	StateRepo ports.AgentStateRepository
	EventRepo ports.EventRepository
	World     ports.WorldProvider
	Now       func() time.Time
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	nowFn := u.Now
	if nowFn == nil {
		nowFn = time.Now
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
	events := []survival.DomainEvent{}
	if u.EventRepo != nil {
		events, err = u.EventRepo.ListByAgentID(ctx, req.AgentID, 50)
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return Response{}, err
		}
	}
	state = stateview.Enrich(state, snapshot.TimeOfDay, isCurrentTileLit(snapshot.TimeOfDay))
	state.CurrentZone = stateview.CurrentZoneAtPosition(state.Position, snapshot.VisibleTiles)
	state.ActionCooldowns = cooldown.RemainingByAction(events, nowFn())
	return Response{
		State:              state,
		WorldTimeSeconds:   snapshot.WorldTimeSeconds,
		TimeOfDay:          snapshot.TimeOfDay,
		NextPhaseInSeconds: snapshot.NextPhaseInSeconds,
		HPDrainFeedback:    toHPDrainFeedback(stateview.EstimateHPDrain(state.Vitals, survival.StandardTickMinutes)),
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
		ProductionRecipes: toProductionRecipes(survival.ProductionRecipeRules()),
		BuildCosts:        cloneNestedIntMap(survival.BuildCostRules()),
	}
}

func toProductionRecipes(in []survival.ProductionRecipeRule) []ProductionRecipe {
	out := make([]ProductionRecipe, 0, len(in))
	for _, recipe := range in {
		out = append(out, ProductionRecipe{
			RecipeID: recipe.RecipeID,
			In:       cloneIntMap(recipe.In),
			Out:      cloneIntMap(recipe.Out),
		})
	}
	return out
}

func cloneIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneNestedIntMap(in map[string]map[string]int) map[string]map[string]int {
	if len(in) == 0 {
		return map[string]map[string]int{}
	}
	out := make(map[string]map[string]int, len(in))
	for k, v := range in {
		out[k] = cloneIntMap(v)
	}
	return out
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

func isCurrentTileLit(timeOfDay string) bool {
	return strings.EqualFold(strings.TrimSpace(timeOfDay), "day")
}
