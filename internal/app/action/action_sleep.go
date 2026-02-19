package action

import (
	"context"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type sleepActionHandler struct{ BaseHandler }

func validateSleepActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.BedID) != ""
}

func (h sleepActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h sleepActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func settleInstantSleepAction(state survival.AgentStateAggregate, intent survival.ActionIntent, bed ports.WorldObjectRecord, nowAt time.Time, snapshot world.Snapshot) survival.SettlementResult {
	next := state
	if next.Inventory != nil {
		inv := make(map[string]int, len(next.Inventory))
		for k, v := range next.Inventory {
			inv[k] = v
		}
		next.Inventory = inv
	}
	if next.StatusEffects != nil {
		next.StatusEffects = append([]string(nil), next.StatusEffects...)
	}
	energyMultiplierNum, energyMultiplierDen := sleepMultiplierByQuality(bed.Quality)
	energyRecovery := (sleepBaseEnergyRecovery*energyMultiplierNum + energyMultiplierDen - 1) / energyMultiplierDen
	hpRecovery := (sleepBaseHPRecovery*energyMultiplierNum + energyMultiplierDen - 1) / energyMultiplierDen

	before := state.Vitals
	next.Vitals.Energy += energyRecovery
	next.Vitals.HP += hpRecovery
	if next.Vitals.HP > 100 {
		next.Vitals.HP = 100
	}
	hpRecovery = next.Vitals.HP - before.HP
	next.Version++
	next.UpdatedAt = nowAt

	event := survival.DomainEvent{
		Type:       "action_settled",
		OccurredAt: nowAt,
		Payload: map[string]any{
			"world_time_before_seconds": snapshot.WorldTimeSeconds,
			"world_time_after_seconds":  snapshot.WorldTimeSeconds,
			"settled_dt_minutes":        0,
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
				"intent": string(intent.Type),
				"params": map[string]any{
					"bed_id":      intent.BedID,
					"bed_quality": strings.ToUpper(strings.TrimSpace(bed.Quality)),
				},
				"dt_minutes": 0,
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
				"hp_loss":         0,
				"inventory_delta": map[string]int{},
				"vitals_delta": map[string]int{
					"hp":     next.Vitals.HP - state.Vitals.HP,
					"hunger": next.Vitals.Hunger - state.Vitals.Hunger,
					"energy": next.Vitals.Energy - state.Vitals.Energy,
				},
				"vitals_change_reasons": map[string]any{
					"hp": []map[string]any{
						{"code": "ACTION_SLEEP_RECOVERY", "delta": hpRecovery},
					},
					"hunger": []map[string]any{},
					"energy": []map[string]any{
						{"code": "ACTION_SLEEP_RECOVERY", "delta": energyRecovery},
					},
				},
			},
		},
	}
	return survival.SettlementResult{
		UpdatedState: next,
		Events:       []survival.DomainEvent{event},
		ResultCode:   survival.ResultOK,
	}
}

func sleepMultiplierByQuality(quality string) (int, int) {
	switch strings.ToUpper(strings.TrimSpace(quality)) {
	case "GOOD":
		return 3, 2
	default:
		return 1, 1
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
