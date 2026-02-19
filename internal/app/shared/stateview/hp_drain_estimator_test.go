package stateview

import (
	"testing"

	"clawvival/internal/domain/survival"
)

func TestEstimateHPDrain_UsesTuningDefaults(t *testing.T) {
	got := EstimateHPDrain(survival.Vitals{Hunger: -100, Energy: -100}, 0)

	if got.Cap != survival.HPDrainCapPer30 {
		t.Fatalf("cap = %d, want %d", got.Cap, survival.HPDrainCapPer30)
	}
	if !got.IsLosingHP {
		t.Fatalf("expected IsLosingHP=true, got false")
	}
	if got.EstimatedLoss != survival.HPDrainCapPer30 {
		t.Fatalf("estimated loss = %d, want %d", got.EstimatedLoss, survival.HPDrainCapPer30)
	}
	if got.HungerComponent <= 0 || got.EnergyComponent <= 0 {
		t.Fatalf("expected both hunger and energy components > 0, got %+v", got)
	}
}
