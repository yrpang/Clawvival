package stateview

import "clawvival/internal/domain/survival"

const (
	criticalHPThreshold = 15
	lowEnergyThreshold  = 20
	defaultCapacity     = 30
)

func Enrich(state survival.AgentStateAggregate, timeOfDay string) survival.AgentStateAggregate {
	next := state
	if next.InventoryCapacity <= 0 {
		next.InventoryCapacity = defaultCapacity
	}
	next.InventoryUsed = computeInventoryUsed(next)
	next.StatusEffects = deriveStatusEffects(next, timeOfDay)
	return next
}

func computeInventoryUsed(state survival.AgentStateAggregate) int {
	if state.InventoryUsed > 0 {
		return state.InventoryUsed
	}
	total := 0
	for _, count := range state.Inventory {
		if count > 0 {
			total += count
		}
	}
	return total
}

func deriveStatusEffects(state survival.AgentStateAggregate, timeOfDay string) []string {
	effects := make([]string, 0, 4)
	if state.Vitals.Hunger <= 0 {
		effects = append(effects, "STARVING")
	}
	if state.Vitals.Energy <= lowEnergyThreshold {
		effects = append(effects, "EXHAUSTED")
	}
	if state.Vitals.HP <= criticalHPThreshold {
		effects = append(effects, "CRITICAL")
	}
	if timeOfDay == "night" {
		effects = append(effects, "IN_DARK")
	}
	return effects
}
