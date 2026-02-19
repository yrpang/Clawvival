package survival

type ActionCostProfile struct {
	BaseMinutes  int
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
			BaseMinutes:  ActionMoveBaseMinutes,
			DeltaHunger:  netHunger(ActionMoveDeltaHunger),
			DeltaEnergy:  ActionMoveDeltaEnergy,
			Requirements: []string{"PASSABLE_TILE"},
		},
		ActionGather: {
			BaseMinutes:  ActionGatherBaseMinutes,
			DeltaHunger:  netHunger(ActionGatherDeltaHunger),
			DeltaEnergy:  ActionGatherDeltaEnergy,
			Requirements: []string{"VISIBLE_TARGET"},
		},
		ActionCraft: {
			BaseMinutes:  ActionCraftBaseMinutes,
			DeltaHunger:  netHunger(ActionCraftDeltaHunger),
			DeltaEnergy:  ActionCraftDeltaEnergy,
			Requirements: []string{"RECIPE_INPUTS"},
		},
		ActionBuild: {
			BaseMinutes:  ActionBuildBaseMinutes,
			DeltaHunger:  netHunger(ActionBuildDeltaHunger),
			DeltaEnergy:  ActionBuildDeltaEnergy,
			Requirements: []string{"BUILD_MATERIALS", "VALID_POS"},
		},
		ActionEat: {
			BaseMinutes:  ActionEatBaseMinutes,
			DeltaHunger:  netHunger(ActionEatDeltaHunger),
			DeltaEnergy:  ActionEatDeltaEnergy,
			Requirements: []string{"HAS_ITEM"},
		},
		ActionRest: {
			BaseMinutes:  ActionRestBaseMinutes,
			DeltaHunger:  netHunger(ActionRestDeltaHunger),
			DeltaEnergy:  ActionRestDeltaEnergy,
			Requirements: []string{},
		},
		ActionSleep: {
			BaseMinutes:  ActionSleepBaseMinutes,
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
			BaseMinutes:  ActionFarmPlantBaseMinutes,
			DeltaHunger:  netHunger(ActionFarmPlantDeltaHunger),
			DeltaEnergy:  ActionFarmPlantDeltaEnergy,
			Requirements: []string{"FARM_ID", "HAS_SEED"},
		},
		ActionFarmHarvest: {
			BaseMinutes:  ActionFarmHarvestBaseMinutes,
			DeltaHunger:  netHunger(ActionFarmHarvestDeltaHunger),
			DeltaEnergy:  ActionFarmHarvestDeltaEnergy,
			Requirements: []string{"FARM_ID", "FARM_READY"},
		},
		ActionContainerDeposit: {
			BaseMinutes:  ActionContainerDepositBaseMinutes,
			DeltaHunger:  netHunger(ActionContainerDepositDeltaHunger),
			DeltaEnergy:  ActionContainerDepositDeltaEnergy,
			Requirements: []string{"CONTAINER_ID", "HAS_ITEMS"},
		},
		ActionContainerWithdraw: {
			BaseMinutes:  ActionContainerWithdrawBaseMinutes,
			DeltaHunger:  netHunger(ActionContainerWithdrawDeltaHunger),
			DeltaEnergy:  ActionContainerWithdrawDeltaEnergy,
			Requirements: []string{"CONTAINER_ID", "CAPACITY_AVAILABLE"},
		},
		ActionRetreat: {
			BaseMinutes:  ActionRetreatBaseMinutes,
			DeltaHunger:  netHunger(ActionRetreatDeltaHunger),
			DeltaEnergy:  ActionRetreatDeltaEnergy,
			Requirements: []string{},
		},
		ActionTerminate: {
			BaseMinutes:  ActionTerminateBaseMinutes,
			DeltaHunger:  ActionTerminateDeltaHunger,
			DeltaEnergy:  ActionTerminateDeltaEnergy,
			Requirements: []string{"INTERRUPTIBLE_ONGOING_ACTION"},
		},
	}
}
