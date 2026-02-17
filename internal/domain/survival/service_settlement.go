package survival

import (
	"errors"
	"math"
	"time"
)

var ErrInvalidDelta = errors.New("invalid delta minutes")

type SettlementService struct{}

func (SettlementService) Settle(state AgentStateAggregate, intent ActionIntent, delta HeartbeatDelta, now time.Time, snapshot WorldSnapshot) (SettlementResult, error) {
	if delta.Minutes <= 0 {
		return SettlementResult{}, ErrInvalidDelta
	}

	next := state
	next.UpdatedAt = now
	actionEvents := make([]DomainEvent, 0, 2)

	// Baseline drains per 30 mins.
	next.Vitals.Hunger -= scaledInt(4, delta.Minutes)

	switch intent.Type {
	case ActionGather:
		next.Vitals.Energy -= scaledInt(18, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(3, delta.Minutes)
		ApplyGather(&next, snapshot)
	case ActionRest:
		next.Vitals.Energy += scaledInt(10, delta.Minutes)
	case ActionMove:
		next.Vitals.Energy -= scaledInt(6, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(1, delta.Minutes)
	case ActionCombat:
		next.Vitals.Energy -= scaledInt(22, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(2, delta.Minutes)
	case ActionBuild:
		next.Vitals.Energy -= scaledInt(14, delta.Minutes)
		obj, ok := Build(&next, BuildKind(intent.Params["kind"]), next.Position.X, next.Position.Y)
		if ok {
			actionEvents = append(actionEvents, DomainEvent{
				Type:       "build_completed",
				OccurredAt: now,
				Payload: map[string]any{
					"kind": int(obj.Kind),
					"x":    obj.X,
					"y":    obj.Y,
					"hp":   100,
				},
			})
		}
	case ActionFarm:
		next.Vitals.Energy -= scaledInt(10, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(1, delta.Minutes)
		if intent.Params["seed"] > 0 {
			_, _ = PlantSeed(&next)
		}
	case ActionRetreat:
		next.Vitals.Energy -= scaledInt(8, delta.Minutes)
		next.Position = moveToward(next.Position, next.Home)
	case ActionCraft:
		next.Vitals.Energy -= scaledInt(12, delta.Minutes)
		_ = Craft(&next, RecipeID(intent.Params["recipe"]))
	}

	hpLossFromHunger := scaledFloat(0.08*float64(absMinZero(next.Vitals.Hunger)), delta.Minutes)
	hpLossFromEnergy := scaledFloat(0.05*float64(absMinZero(next.Vitals.Energy)), delta.Minutes)
	hpCap := float64(scaledInt(12, delta.Minutes))
	hpLoss := int(math.Round(minFloat(hpLossFromHunger+hpLossFromEnergy, hpCap)))
	if intent.Type == ActionCombat {
		if snapshot.TimeOfDay == "night" {
			hpLoss += scaledInt(snapshot.ThreatLevel, delta.Minutes)
		} else {
			hpLoss += scaledInt(maxInt(1, snapshot.ThreatLevel/2), delta.Minutes)
		}
		if snapshot.VisibilityPenalty > 0 {
			hpLoss += scaledInt(snapshot.VisibilityPenalty, delta.Minutes)
		}
	}

	next.Vitals.HP -= hpLoss
	next.Version++

	events := make([]DomainEvent, 0, 2)
	events = append(events, DomainEvent{
		Type:       "action_settled",
		OccurredAt: now,
		Payload: map[string]any{
			"state_before": map[string]any{
				"hp":     state.Vitals.HP,
				"hunger": state.Vitals.Hunger,
				"energy": state.Vitals.Energy,
				"x":      state.Position.X,
				"y":      state.Position.Y,
			},
			"decision": map[string]any{
				"intent":     string(intent.Type),
				"params":     intent.Params,
				"dt_minutes": delta.Minutes,
			},
			"state_after": map[string]any{
				"hp":     next.Vitals.HP,
				"hunger": next.Vitals.Hunger,
				"energy": next.Vitals.Energy,
				"x":      next.Position.X,
				"y":      next.Position.Y,
			},
			"result": map[string]any{
				"hp_loss": hpLoss,
			},
		},
	})

	resultCode := ResultOK
	if next.Vitals.HP <= 0 {
		next.MarkDead(deriveDeathCause(next, intent))
		events = append(events, DomainEvent{Type: "game_over", OccurredAt: now})
		resultCode = ResultGameOver
	} else if next.Vitals.HP <= 20 {
		next.Position = moveToward(next.Position, next.Home)
		events = append(events, DomainEvent{Type: "critical_hp", OccurredAt: now})
		events = append(events, DomainEvent{Type: "force_retreat", OccurredAt: now})
	}
	events = append(events, actionEvents...)

	return SettlementResult{
		UpdatedState: next,
		Events:       events,
		ResultCode:   resultCode,
	}, nil
}

func scaledInt(per30 int, dt int) int {
	return int(math.Round(float64(per30) * float64(dt) / 30.0))
}

func scaledFloat(per30 float64, dt int) float64 {
	return per30 * float64(dt) / 30.0
}

func absMinZero(v int) int {
	if v < 0 {
		return -v
	}
	return 0
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func moveToward(from, to Position) Position {
	next := from
	if from.X < to.X {
		next.X++
	} else if from.X > to.X {
		next.X--
	}
	if from.Y < to.Y {
		next.Y++
	} else if from.Y > to.Y {
		next.Y--
	}
	return next
}

func deriveDeathCause(state AgentStateAggregate, intent ActionIntent) DeathCause {
	switch {
	case state.Vitals.Hunger < 0:
		return DeathCauseStarvation
	case state.Vitals.Energy < 0:
		return DeathCauseExhaustion
	case intent.Type == ActionCombat:
		return DeathCauseCombat
	default:
		return DeathCauseUnknown
	}
}
