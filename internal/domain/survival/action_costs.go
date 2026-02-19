package survival

type ActionCostProfile struct {
	DeltaHunger  int
	DeltaEnergy  int
	DeltaHP      int
	Requirements []string
	Variants     map[string]ActionCostVariant
}

type ActionCostVariant struct {
	DeltaHunger int
	DeltaEnergy int
	DeltaHP     int
}

func DefaultActionCostProfiles() map[ActionType]ActionCostProfile {
	netHunger := func(actionDelta int) int {
		return actionDelta - BaseHungerDrainPer30
	}
	return map[ActionType]ActionCostProfile{
		ActionMove: {
			DeltaHunger:  netHunger(ActionMoveDeltaHunger),
			DeltaEnergy:  ActionMoveDeltaEnergy,
			Requirements: []string{"PASSABLE_TILE"},
		},
		ActionGather: {
			DeltaHunger:  netHunger(ActionGatherDeltaHunger),
			DeltaEnergy:  ActionGatherDeltaEnergy,
			Requirements: []string{"VISIBLE_TARGET"},
		},
		ActionCraft: {
			DeltaHunger:  netHunger(ActionCraftDeltaHunger),
			DeltaEnergy:  ActionCraftDeltaEnergy,
			Requirements: []string{"RECIPE_INPUTS"},
		},
		ActionBuild: {
			DeltaHunger:  netHunger(ActionBuildDeltaHunger),
			DeltaEnergy:  ActionBuildDeltaEnergy,
			Requirements: []string{"BUILD_MATERIALS", "VALID_POS"},
		},
		ActionEat: {
			DeltaHunger:  netHunger(ActionEatDeltaHunger),
			DeltaEnergy:  ActionEatDeltaEnergy,
			Requirements: []string{"HAS_ITEM"},
		},
		ActionRest: {
			DeltaHunger:  netHunger(ActionRestDeltaHunger),
			DeltaEnergy:  ActionRestDeltaEnergy,
			Requirements: []string{},
		},
		ActionSleep: {
			DeltaHunger:  netHunger(ActionSleepDeltaHunger),
			DeltaEnergy:  ActionSleepDeltaEnergy,
			DeltaHP:      SleepBaseHPRecovery,
			Requirements: []string{"BED_ID"},
			Variants: map[string]ActionCostVariant{
				"bed_quality_rough": {
					DeltaHunger: netHunger(ActionSleepDeltaHunger),
					DeltaEnergy: ActionSleepDeltaEnergy,
					DeltaHP:     SleepBaseHPRecovery,
				},
				"bed_quality_good": {
					DeltaHunger: netHunger(ActionSleepDeltaHunger),
					DeltaEnergy: int(1.5 * float64(ActionSleepDeltaEnergy)),
					DeltaHP:     int(1.5 * float64(SleepBaseHPRecovery)),
				},
			},
		},
		ActionFarmPlant: {
			DeltaHunger:  netHunger(ActionFarmPlantDeltaHunger),
			DeltaEnergy:  ActionFarmPlantDeltaEnergy,
			Requirements: []string{"FARM_ID", "HAS_SEED"},
		},
		ActionFarmHarvest: {
			DeltaHunger:  netHunger(ActionFarmHarvestDeltaHunger),
			DeltaEnergy:  ActionFarmHarvestDeltaEnergy,
			Requirements: []string{"FARM_ID", "FARM_READY"},
		},
		ActionContainerDeposit: {
			DeltaHunger:  netHunger(ActionContainerDepositDeltaHunger),
			DeltaEnergy:  ActionContainerDepositDeltaEnergy,
			Requirements: []string{"CONTAINER_ID", "HAS_ITEMS"},
		},
		ActionContainerWithdraw: {
			DeltaHunger:  netHunger(ActionContainerWithdrawDeltaHunger),
			DeltaEnergy:  ActionContainerWithdrawDeltaEnergy,
			Requirements: []string{"CONTAINER_ID", "CAPACITY_AVAILABLE"},
		},
		ActionRetreat: {
			DeltaHunger:  netHunger(ActionRetreatDeltaHunger),
			DeltaEnergy:  ActionRetreatDeltaEnergy,
			Requirements: []string{},
		},
		ActionTerminate: {
			DeltaHunger:  ActionTerminateDeltaHunger,
			DeltaEnergy:  ActionTerminateDeltaEnergy,
			Requirements: []string{"INTERRUPTIBLE_ONGOING_ACTION"},
		},
	}
}
