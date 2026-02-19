package stateview

import (
	"testing"

	"clawvival/internal/domain/survival"
)

func TestEnrich_RecomputesInventoryUsedFromInventory(t *testing.T) {
	state := survival.AgentStateAggregate{
		Inventory:     map[string]int{"wood": 1, "stone": 2},
		InventoryUsed: 99, // stale persisted value
	}

	out := Enrich(state, "day", true)
	if out.InventoryUsed != 3 {
		t.Fatalf("expected inventory_used recomputed to 3, got=%d", out.InventoryUsed)
	}
}

func TestEnrich_IN_DARKRequiresNightAndUnlitTile(t *testing.T) {
	state := survival.AgentStateAggregate{
		Vitals: survival.Vitals{HP: 100, Hunger: 100, Energy: 100},
	}

	nightLit := Enrich(state, "night", true)
	for _, effect := range nightLit.StatusEffects {
		if effect == "IN_DARK" {
			t.Fatalf("did not expect IN_DARK when current tile is lit")
		}
	}

	nightUnlit := Enrich(state, "night", false)
	found := false
	for _, effect := range nightUnlit.StatusEffects {
		if effect == "IN_DARK" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected IN_DARK when night and current tile unlit")
	}
}
