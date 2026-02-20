package survival

import (
	"testing"
	"time"
)

func TestGameplayTuning_Defaults(t *testing.T) {
	if StandardTickMinutes != 30 {
		t.Fatalf("StandardTickMinutes = %d, want 30", StandardTickMinutes)
	}
	if BaseHungerDrainPer30 != 0 {
		t.Fatalf("BaseHungerDrainPer30 = %d, want 0", BaseHungerDrainPer30)
	}
	if HPDrainCapPer30 != 8 {
		t.Fatalf("HPDrainCapPer30 = %d, want 8", HPDrainCapPer30)
	}
	if HPDrainFromHungerCoeff != 0.04 || HPDrainFromEnergyCoeff != 0.03 {
		t.Fatalf("hp drain coeffs = (%v,%v), want (0.04,0.03)", HPDrainFromHungerCoeff, HPDrainFromEnergyCoeff)
	}
	if MinRestMinutes != 1 || MaxRestMinutes != 120 {
		t.Fatalf("rest bounds = (%d,%d), want (1,120)", MinRestMinutes, MaxRestMinutes)
	}
	if DefaultFarmGrowMinutes != 60 {
		t.Fatalf("DefaultFarmGrowMinutes = %d, want 60", DefaultFarmGrowMinutes)
	}
	if SeedPityMaxFails != 8 {
		t.Fatalf("SeedPityMaxFails = %d, want 8", SeedPityMaxFails)
	}
	if ActionNightVisionRadius != 3 {
		t.Fatalf("ActionNightVisionRadius = %d, want 3", ActionNightVisionRadius)
	}
	if DefaultInventoryCapacity != 30 {
		t.Fatalf("DefaultInventoryCapacity = %d, want 30", DefaultInventoryCapacity)
	}
	if SleepBaseEnergyRecovery != 35 || SleepBaseHPRecovery != 6 {
		t.Fatalf("sleep base recovery = (%d,%d), want (35,6)", SleepBaseEnergyRecovery, SleepBaseHPRecovery)
	}
	if CriticalHPThreshold != 15 || LowEnergyThreshold != 20 {
		t.Fatalf("status thresholds = (%d,%d), want (15,20)", CriticalHPThreshold, LowEnergyThreshold)
	}
	if DefaultRespawnMinutes != 60 {
		t.Fatalf("DefaultRespawnMinutes = %d, want 60", DefaultRespawnMinutes)
	}
	if WheatYieldMin != 1 || WheatYieldMax != 3 {
		t.Fatalf("wheat yield range = (%d,%d), want (1,3)", WheatYieldMin, WheatYieldMax)
	}
	if SeedDropChance != 0.2 || SeedReturnChance != 0.2 {
		t.Fatalf("seed chances = (%v,%v), want (0.2,0.2)", SeedDropChance, SeedReturnChance)
	}
	if VisionRadiusDay != 6 || VisionRadiusNight != 3 || TorchLightRadius != 3 {
		t.Fatalf("visibility = (%d,%d,%d), want (6,3,3)", VisionRadiusDay, VisionRadiusNight, TorchLightRadius)
	}
	if FoodBerryHungerRecovery != 20 || FoodBreadHungerRecovery != 30 || FoodWheatHungerRecovery != 15 || FoodJamHungerRecovery != 80 {
		t.Fatalf(
			"food recovery = (berry:%d,bread:%d,wheat:%d,jam:%d), want (20,30,15,80)",
			FoodBerryHungerRecovery,
			FoodBreadHungerRecovery,
			FoodWheatHungerRecovery,
			FoodJamHungerRecovery,
		)
	}
}

func TestGameplayTuning_CooldownsAndRespawn(t *testing.T) {
	if got := ActionCooldownDurations[ActionMove]; got != 1*time.Minute {
		t.Fatalf("move cooldown = %s, want 1m", got)
	}
	if got := ActionCooldownDurations[ActionBuild]; got != 5*time.Minute {
		t.Fatalf("build cooldown = %s, want 5m", got)
	}
	if got := ResourceRespawnDurations["wood"]; got != 60*time.Minute {
		t.Fatalf("wood respawn = %s, want 60m", got)
	}
	if got := ResourceRespawnDurations["berry"]; got != 30*time.Minute {
		t.Fatalf("berry respawn = %s, want 30m", got)
	}
}
