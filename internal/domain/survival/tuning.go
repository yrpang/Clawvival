package survival

import "time"

const (
	StandardTickMinutes = 30

	BaseHungerDrainPer30 = 0
	HPDrainCapPer30      = 8

	HPDrainFromHungerCoeff = 0.04
	HPDrainFromEnergyCoeff = 0.03

	MinRestMinutes = 1
	MaxRestMinutes = 120

	DefaultFarmGrowMinutes = 60
	SeedPityMaxFails       = 8

	ActionNightVisionRadius = 3

	DefaultInventoryCapacity = 30

	SleepBaseEnergyRecovery = 35
	SleepBaseHPRecovery     = 6
	SleepGoodHungerRecovery = 20
	SleepGoodEnergyRecovery = 45
	SleepGoodHPRecovery     = 10

	CriticalHPThreshold = 15
	LowEnergyThreshold  = 20

	DefaultRespawnMinutes = 60

	WheatYieldMin     = 1
	WheatYieldMax     = 3
	SeedDropChance    = 0.2
	SeedReturnChance  = 0.2
	VisionRadiusDay   = 6
	VisionRadiusNight = 3
	TorchLightRadius  = 3

	ActionMoveDeltaHunger = -1
	ActionMoveDeltaEnergy = -2

	ActionGatherDeltaHunger = -2
	ActionGatherDeltaEnergy = -6

	ActionCraftDeltaHunger = -1
	ActionCraftDeltaEnergy = -4

	ActionBuildDeltaHunger = -1
	ActionBuildDeltaEnergy = -6

	ActionEatDeltaHunger = 10
	ActionEatDeltaEnergy = 0

	FoodBerryHungerRecovery = 20
	FoodBreadHungerRecovery = 30
	FoodWheatHungerRecovery = 15
	FoodJamHungerRecovery   = 80

	ActionRestDeltaHunger = 3
	ActionRestDeltaEnergy = 20

	ActionSleepDeltaHunger = 15
	ActionSleepDeltaEnergy = SleepBaseEnergyRecovery

	ActionFarmPlantDeltaHunger = -1
	ActionFarmPlantDeltaEnergy = -4

	ActionFarmHarvestDeltaHunger = -1
	ActionFarmHarvestDeltaEnergy = -4

	ActionContainerDepositDeltaHunger = 0
	ActionContainerDepositDeltaEnergy = 0

	ActionContainerWithdrawDeltaHunger = 0
	ActionContainerWithdrawDeltaEnergy = 0

	ActionRetreatDeltaHunger = 0
	ActionRetreatDeltaEnergy = -2

	ActionTerminateDeltaHunger = 0
	ActionTerminateDeltaEnergy = 0
)

var ActionCooldownDurations = map[ActionType]time.Duration{
	ActionBuild:     5 * time.Minute,
	ActionCraft:     5 * time.Minute,
	ActionFarmPlant: 3 * time.Minute,
	ActionMove:      1 * time.Minute,
	ActionSleep:     5 * time.Minute,
}

var ResourceRespawnDurations = map[string]time.Duration{
	"wood":  60 * time.Minute,
	"stone": 60 * time.Minute,
	"berry": 30 * time.Minute,
	"seed":  30 * time.Minute,
}
