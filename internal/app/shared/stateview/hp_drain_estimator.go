package stateview

import (
	"math"

	"clawvival/internal/domain/survival"
)

type HPDrainEstimate struct {
	IsLosingHP      bool
	EstimatedLoss   int
	HungerComponent int
	EnergyComponent int
	Cap             int
	Causes          []string
}

func EstimateHPDrain(vitals survival.Vitals, dtMinutes int) HPDrainEstimate {
	if dtMinutes <= 0 {
		dtMinutes = survival.StandardTickMinutes
	}
	cap := scaledInt(survival.HPDrainCapPer30, dtMinutes)
	if cap < 0 {
		cap = 0
	}

	hungerPotential := int(math.Round(
		survival.HPDrainFromHungerCoeff * float64(absMinZero(vitals.Hunger)) * float64(dtMinutes) / float64(survival.StandardTickMinutes),
	))
	energyPotential := int(math.Round(
		survival.HPDrainFromEnergyCoeff * float64(absMinZero(vitals.Energy)) * float64(dtMinutes) / float64(survival.StandardTickMinutes),
	))
	hungerApplied, energyApplied := applyDualCap(hungerPotential, energyPotential, cap)
	loss := hungerApplied + energyApplied

	causes := make([]string, 0, 2)
	if hungerApplied > 0 {
		causes = append(causes, "STARVING_HP_DRAIN")
	}
	if energyApplied > 0 {
		causes = append(causes, "EXHAUSTED_HP_DRAIN")
	}

	return HPDrainEstimate{
		IsLosingHP:      loss > 0,
		EstimatedLoss:   loss,
		HungerComponent: hungerApplied,
		EnergyComponent: energyApplied,
		Cap:             cap,
		Causes:          causes,
	}
}

func scaledInt(per30 int, dt int) int {
	return int(math.Round(float64(per30) * float64(dt) / float64(survival.StandardTickMinutes)))
}

func absMinZero(v int) int {
	if v < 0 {
		return -v
	}
	return 0
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
