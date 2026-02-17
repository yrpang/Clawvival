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
	case ActionSleep:
		next.Vitals.Energy += scaledInt(18, delta.Minutes)
		next.Vitals.HP += scaledInt(4, delta.Minutes)
		if next.Vitals.HP > 100 {
			next.Vitals.HP = 100
		}
	case ActionMove:
		moveEnergyCost := scaledInt(6, delta.Minutes)
		if moveEnergyCost < 1 {
			moveEnergyCost = 1
		}
		next.Vitals.Energy -= moveEnergyCost
		next.Vitals.Hunger -= scaledInt(1, delta.Minutes)
		next.Position.X += intent.DX
		next.Position.Y += intent.DY
	case ActionCombat:
		next.Vitals.Energy -= scaledInt(22, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(2, delta.Minutes)
	case ActionBuild:
		next.Vitals.Energy -= scaledInt(14, delta.Minutes)
		kind, ok := buildKindFromIntent(intent.ObjectType)
		if !ok {
			break
		}
		buildX, buildY := next.Position.X, next.Position.Y
		if intent.Pos != nil {
			buildX, buildY = intent.Pos.X, intent.Pos.Y
		}
		obj, ok := Build(&next, kind, buildX, buildY)
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
	case ActionFarm, ActionFarmPlant:
		next.Vitals.Energy -= scaledInt(10, delta.Minutes)
		next.Vitals.Hunger -= scaledInt(1, delta.Minutes)
		_, _ = PlantSeed(&next)
	case ActionFarmHarvest:
		next.Vitals.Energy -= scaledInt(8, delta.Minutes)
		next.AddItem("wheat", 2)
	case ActionContainerDeposit, ActionContainerWithdraw:
		next.Vitals.Energy -= scaledInt(4, delta.Minutes)
		applyContainerTransfer(&next, intent)
	case ActionRetreat:
		next.Vitals.Energy -= scaledInt(8, delta.Minutes)
		next.Position = moveToward(next.Position, next.Home)
	case ActionCraft:
		next.Vitals.Energy -= scaledInt(12, delta.Minutes)
		_ = Craft(&next, RecipeID(intent.RecipeID))
	case ActionEat:
		_ = Eat(&next, foodIDFromIntent(intent.ItemType))
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
				"params":     intentDecisionParams(intent),
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

func buildKindFromIntent(objectType string) (BuildKind, bool) {
	switch objectType {
	case "bed", "bed_rough", "bed_good":
		return BuildBed, true
	case "box":
		return BuildBox, true
	case "farm_plot":
		return BuildFarm, true
	case "torch":
		return BuildTorch, true
	default:
		return 0, false
	}
}

func foodIDFromIntent(itemType string) FoodID {
	switch itemType {
	case "berry":
		return FoodBerry
	case "bread":
		return FoodBread
	default:
		return FoodBerry
	}
}

func intentDecisionParams(intent ActionIntent) map[string]any {
	out := map[string]any{}
	if intent.Direction != "" {
		out["direction"] = intent.Direction
	}
	if intent.TargetID != "" {
		out["target_id"] = intent.TargetID
	}
	if intent.RecipeID > 0 {
		out["recipe_id"] = intent.RecipeID
	}
	if intent.ObjectType != "" {
		out["object_type"] = intent.ObjectType
	}
	if intent.Pos != nil {
		out["pos"] = map[string]int{"x": intent.Pos.X, "y": intent.Pos.Y}
	}
	if intent.ItemType != "" {
		out["item_type"] = intent.ItemType
	}
	if intent.RestMinutes > 0 {
		out["rest_minutes"] = intent.RestMinutes
	}
	if intent.BedID != "" {
		out["bed_id"] = intent.BedID
	}
	if intent.FarmID != "" {
		out["farm_id"] = intent.FarmID
	}
	if intent.ContainerID != "" {
		out["container_id"] = intent.ContainerID
	}
	if len(intent.Items) > 0 {
		items := make([]map[string]any, 0, len(intent.Items))
		for _, item := range intent.Items {
			items = append(items, map[string]any{"item_type": item.ItemType, "count": item.Count})
		}
		out["items"] = items
	}
	if intent.DX != 0 || intent.DY != 0 {
		out["dx"] = intent.DX
		out["dy"] = intent.DY
	}
	return out
}

func applyContainerTransfer(state *AgentStateAggregate, intent ActionIntent) {
	for _, item := range intent.Items {
		if item.Count <= 0 || item.ItemType == "" {
			continue
		}
		switch intent.Type {
		case ActionContainerDeposit:
			_ = state.ConsumeItem(item.ItemType, item.Count)
		case ActionContainerWithdraw:
			state.AddItem(item.ItemType, item.Count)
		}
	}
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
