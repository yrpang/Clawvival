package survival

import "time"

const (
	StandardTickMinutes = 30

	BaseHungerDrainPer30 = 4
	HPDrainCapPer30      = 12

	HPDrainFromHungerCoeff = 0.08
	HPDrainFromEnergyCoeff = 0.05

	MinRestMinutes = 1
	MaxRestMinutes = 120

	DefaultFarmGrowMinutes = 60
	SeedPityMaxFails       = 8

	ActionNightVisionRadius = 3

	DefaultInventoryCapacity = 30

	SleepBaseEnergyRecovery = 30
	SleepBaseHPRecovery     = 8

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
	ActionMoveDeltaEnergy = -6

	ActionGatherDeltaHunger = -3
	ActionGatherDeltaEnergy = -18

	ActionCraftDeltaHunger = 0
	ActionCraftDeltaEnergy = -12

	ActionBuildDeltaHunger = 0
	ActionBuildDeltaEnergy = -14

	ActionEatDeltaHunger = 12
	ActionEatDeltaEnergy = 0

	ActionRestDeltaHunger = 14
	ActionRestDeltaEnergy = 18

	ActionSleepDeltaHunger = 24
	ActionSleepDeltaEnergy = SleepBaseEnergyRecovery

	ActionFarmPlantDeltaHunger = -1
	ActionFarmPlantDeltaEnergy = -10

	ActionFarmHarvestDeltaHunger = 0
	ActionFarmHarvestDeltaEnergy = -8

	ActionContainerDepositDeltaHunger = 0
	ActionContainerDepositDeltaEnergy = -4

	ActionContainerWithdrawDeltaHunger = 0
	ActionContainerWithdrawDeltaEnergy = -4

	ActionRetreatDeltaHunger = 0
	ActionRetreatDeltaEnergy = -8

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
