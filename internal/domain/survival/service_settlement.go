package survival

import (
	"errors"
	"math"
	"strings"
	"time"
)

var ErrInvalidDelta = errors.New("invalid delta minutes")

type SettlementService struct{}

func (SettlementService) Settle(state AgentStateAggregate, intent ActionIntent, delta HeartbeatDelta, now time.Time, snapshot WorldSnapshot) (SettlementResult, error) {
	if delta.Minutes <= 0 {
		return SettlementResult{}, ErrInvalidDelta
	}

	next := cloneAgentState(state)
	next.UpdatedAt = now
	actionEvents := make([]DomainEvent, 0, 2)
	hpReasons := make([]map[string]any, 0, 4)
	hungerReasons := make([]map[string]any, 0, 4)
	energyReasons := make([]map[string]any, 0, 4)

	// Baseline drains per standard tick.
	applyReasonedDelta(&next.Vitals.Hunger, -scaledInt(BaseHungerDrainPer30, delta.Minutes), "BASE_HUNGER_DRAIN", &hungerReasons)

	switch intent.Type {
	case ActionGather:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionGatherDeltaEnergy, delta.Minutes), "ACTION_GATHER_COST", &energyReasons)
		applyReasonedDelta(&next.Vitals.Hunger, scaledInt(ActionGatherDeltaHunger, delta.Minutes), "ACTION_GATHER_COST", &hungerReasons)
		ApplyGather(&next, snapshot)
	case ActionRest:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionRestDeltaEnergy, delta.Minutes), "ACTION_REST_RECOVERY", &energyReasons)
	case ActionSleep:
		multiplier := sleepQualityMultiplier(intent.BedQuality)
		applyReasonedDelta(&next.Vitals.Energy, int(math.Round(scaledFloat(float64(SleepBaseEnergyRecovery)*multiplier, delta.Minutes))), "ACTION_SLEEP_RECOVERY", &energyReasons)
		applyReasonedHPDelta(&next.Vitals.HP, int(math.Round(scaledFloat(float64(SleepBaseHPRecovery)*multiplier, delta.Minutes))), "ACTION_SLEEP_RECOVERY", &hpReasons)
	case ActionMove:
		moveEnergyCost := scaledInt(-ActionMoveDeltaEnergy, delta.Minutes)
		if moveEnergyCost < 1 {
			moveEnergyCost = 1
		}
		applyReasonedDelta(&next.Vitals.Energy, -moveEnergyCost, "ACTION_MOVE_COST", &energyReasons)
		applyReasonedDelta(&next.Vitals.Hunger, scaledInt(ActionMoveDeltaHunger, delta.Minutes), "ACTION_MOVE_COST", &hungerReasons)
		next.Position.X += intent.DX
		next.Position.Y += intent.DY
	case ActionBuild:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionBuildDeltaEnergy, delta.Minutes), "ACTION_BUILD_COST", &energyReasons)
		if _, ok := buildKindFromIntent(intent.ObjectType); !ok {
			break
		}
		buildX, buildY := next.Position.X, next.Position.Y
		if intent.Pos != nil {
			buildX, buildY = intent.Pos.X, intent.Pos.Y
		}
		obj, ok := BuildObject(&next, intent.ObjectType, buildX, buildY)
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
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionFarmPlantDeltaEnergy, delta.Minutes), "ACTION_FARM_COST", &energyReasons)
		applyReasonedDelta(&next.Vitals.Hunger, scaledInt(ActionFarmPlantDeltaHunger, delta.Minutes), "ACTION_FARM_COST", &hungerReasons)
		_, _ = PlantSeed(&next)
	case ActionFarmHarvest:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionFarmHarvestDeltaEnergy, delta.Minutes), "ACTION_FARM_HARVEST_COST", &energyReasons)
		next.AddItem("wheat", 2)
		if shouldReturnHarvestSeed(now) {
			next.AddItem("seed", 1)
		}
	case ActionContainerDeposit, ActionContainerWithdraw:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionContainerDepositDeltaEnergy, delta.Minutes), "ACTION_CONTAINER_COST", &energyReasons)
		applyContainerTransfer(&next, intent)
	case ActionRetreat:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionRetreatDeltaEnergy, delta.Minutes), "ACTION_RETREAT_COST", &energyReasons)
		if intent.DX != 0 || intent.DY != 0 {
			next.Position.X += clampStep(intent.DX)
			next.Position.Y += clampStep(intent.DY)
		}
	case ActionCraft:
		applyReasonedDelta(&next.Vitals.Energy, scaledInt(ActionCraftDeltaEnergy, delta.Minutes), "ACTION_CRAFT_COST", &energyReasons)
		_ = Craft(&next, RecipeID(intent.RecipeID))
	case ActionEat:
		beforeHunger := next.Vitals.Hunger
		count := intent.Count
		if count <= 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			if !Eat(&next, foodIDFromIntent(intent.ItemType)) {
				break
			}
		}
		appendReason(&hungerReasons, "ACTION_EAT_RECOVERY", next.Vitals.Hunger-beforeHunger)
	}

	hungerLossPotential := int(math.Round(scaledFloat(HPDrainFromHungerCoeff*float64(absMinZero(next.Vitals.Hunger)), delta.Minutes)))
	energyLossPotential := int(math.Round(scaledFloat(HPDrainFromEnergyCoeff*float64(absMinZero(next.Vitals.Energy)), delta.Minutes)))
	hpCap := scaledInt(HPDrainCapPer30, delta.Minutes)
	hungerApplied, energyApplied := applyDualCap(hungerLossPotential, energyLossPotential, hpCap)
	hpLoss := hungerApplied + energyApplied
	if hungerApplied > 0 {
		appendReason(&hpReasons, "STARVING_HP_DRAIN", -hungerApplied)
	}
	if energyApplied > 0 {
		appendReason(&hpReasons, "EXHAUSTED_HP_DRAIN", -energyApplied)
	}
	applyReasonedHPDelta(&next.Vitals.HP, -hpLoss, "HP_LOSS_APPLIED", &hpReasons)
	next.Version++

	events := make([]DomainEvent, 0, 2)
	events = append(events, DomainEvent{
		Type:       "action_settled",
		OccurredAt: now,
		Payload: map[string]any{
			"world_time_before_seconds": snapshot.WorldTimeSeconds,
			"world_time_after_seconds":  snapshot.WorldTimeSeconds + int64(delta.Minutes*60),
			"settled_dt_minutes":        delta.Minutes,
			"state_before": map[string]any{
				"hp":             state.Vitals.HP,
				"hunger":         state.Vitals.Hunger,
				"energy":         state.Vitals.Energy,
				"x":              state.Position.X,
				"y":              state.Position.Y,
				"pos":            map[string]int{"x": state.Position.X, "y": state.Position.Y},
				"inventory_used": inventoryUsedCount(state.Inventory),
			},
			"decision": map[string]any{
				"intent":     string(intent.Type),
				"params":     intentDecisionParams(intent),
				"dt_minutes": delta.Minutes,
			},
			"state_after": map[string]any{
				"hp":             next.Vitals.HP,
				"hunger":         next.Vitals.Hunger,
				"energy":         next.Vitals.Energy,
				"x":              next.Position.X,
				"y":              next.Position.Y,
				"pos":            map[string]int{"x": next.Position.X, "y": next.Position.Y},
				"inventory_used": inventoryUsedCount(next.Inventory),
			},
			"result": map[string]any{
				"hp_loss":         hpLoss,
				"inventory_delta": inventoryDelta(state.Inventory, next.Inventory),
				"vitals_delta": map[string]int{
					"hp":     next.Vitals.HP - state.Vitals.HP,
					"hunger": next.Vitals.Hunger - state.Vitals.Hunger,
					"energy": next.Vitals.Energy - state.Vitals.Energy,
				},
				"vitals_change_reasons": map[string]any{
					"hp":     hpReasons,
					"hunger": hungerReasons,
					"energy": energyReasons,
				},
			},
		},
	})

	resultCode := ResultOK
	if next.Vitals.HP <= 0 {
		next.MarkDead(deriveDeathCause(next, intent))
		events = append(events, DomainEvent{
			Type:       "game_over",
			OccurredAt: now,
			Payload: map[string]any{
				"death_cause": mapDeathCauseForEvent(next.DeathCause),
				"state_before_last_action": map[string]any{
					"hp":                 state.Vitals.HP,
					"hunger":             state.Vitals.Hunger,
					"energy":             state.Vitals.Energy,
					"position":           map[string]int{"x": state.Position.X, "y": state.Position.Y},
					"inventory_used":     inventoryUsedCount(state.Inventory),
					"world_time_seconds": snapshot.WorldTimeSeconds,
					"inventory_summary":  inventorySummary(state.Inventory),
				},
				"state_after_last_action": map[string]any{
					"hp":                 next.Vitals.HP,
					"hunger":             next.Vitals.Hunger,
					"energy":             next.Vitals.Energy,
					"position":           map[string]int{"x": next.Position.X, "y": next.Position.Y},
					"inventory_used":     inventoryUsedCount(next.Inventory),
					"world_time_seconds": snapshot.WorldTimeSeconds + int64(delta.Minutes*60),
					"inventory_summary":  inventorySummary(next.Inventory),
				},
				"last_safe_home": map[string]int{
					"x": next.Home.X,
					"y": next.Home.Y,
				},
				"last_known_threat": nil,
			},
		})
		resultCode = ResultGameOver
	} else if next.Vitals.HP <= CriticalHPThreshold {
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
	case "wheat":
		return FoodWheat
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
	if intent.Count > 0 {
		out["count"] = intent.Count
	}
	if intent.RestMinutes > 0 {
		out["rest_minutes"] = intent.RestMinutes
	}
	if intent.BedID != "" {
		out["bed_id"] = intent.BedID
	}
	if strings.TrimSpace(intent.BedQuality) != "" {
		out["bed_quality"] = strings.ToUpper(strings.TrimSpace(intent.BedQuality))
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
	return int(math.Round(float64(per30) * float64(dt) / float64(StandardTickMinutes)))
}

func scaledFloat(per30 float64, dt int) float64 {
	return per30 * float64(dt) / float64(StandardTickMinutes)
}

func absMinZero(v int) int {
	if v < 0 {
		return -v
	}
	return 0
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
	default:
		return DeathCauseUnknown
	}
}

func sleepQualityMultiplier(quality string) float64 {
	switch strings.ToUpper(strings.TrimSpace(quality)) {
	case "GOOD":
		return 1.5
	default:
		return 1.0
	}
}

func inventoryUsedCount(inventory map[string]int) int {
	total := 0
	for _, count := range inventory {
		if count > 0 {
			total += count
		}
	}
	return total
}

func inventorySummary(inventory map[string]int) map[string]any {
	top := make([]map[string]any, 0, len(inventory))
	total := 0
	for itemType, count := range inventory {
		if count <= 0 {
			continue
		}
		total += count
		top = append(top, map[string]any{
			"item_type": itemType,
			"count":     count,
		})
	}
	return map[string]any{
		"total_items": total,
		"top":         top,
	}
}

func mapDeathCauseForEvent(cause DeathCause) string {
	switch cause {
	case DeathCauseStarvation:
		return "STARVATION"
	case DeathCauseThreat:
		return "THREAT"
	default:
		return "UNKNOWN"
	}
}

func clampStep(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

func shouldReturnHarvestSeed(now time.Time) bool {
	// Deterministic 20% return to keep simulation testable and explainable.
	return now.Unix()%5 == 0
}

func appendReason(reasons *[]map[string]any, code string, delta int) {
	if delta == 0 {
		return
	}
	*reasons = append(*reasons, map[string]any{
		"code":  code,
		"delta": delta,
	})
}

func applyReasonedDelta(target *int, delta int, code string, reasons *[]map[string]any) {
	if delta == 0 {
		return
	}
	*target += delta
	appendReason(reasons, code, delta)
}

func applyReasonedHPDelta(target *int, delta int, code string, reasons *[]map[string]any) {
	if delta == 0 {
		return
	}
	before := *target
	*target += delta
	if *target > 100 {
		*target = 100
	}
	actual := *target - before
	appendReason(reasons, code, actual)
}

func applyDualCap(primary, secondary, cap int) (int, int) {
	if cap <= 0 {
		return 0, 0
	}
	primaryApplied := min(primary, cap)
	remaining := cap - primaryApplied
	secondaryApplied := min(secondary, remaining)
	return primaryApplied, secondaryApplied
}

func inventoryDelta(before, after map[string]int) map[string]int {
	keys := map[string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	delta := map[string]int{}
	for k := range keys {
		diff := after[k] - before[k]
		if diff != 0 {
			delta[k] = diff
		}
	}
	return delta
}

func cloneAgentState(in AgentStateAggregate) AgentStateAggregate {
	out := in
	if in.Inventory != nil {
		out.Inventory = make(map[string]int, len(in.Inventory))
		for k, v := range in.Inventory {
			out.Inventory[k] = v
		}
	}
	if in.StatusEffects != nil {
		out.StatusEffects = append([]string(nil), in.StatusEffects...)
	}
	if in.OngoingAction != nil {
		copyAction := *in.OngoingAction
		out.OngoingAction = &copyAction
	}
	return out
}
