package survival

import "testing"

func TestDefaultActionCostProfiles(t *testing.T) {
	profiles := DefaultActionCostProfiles()

	if len(profiles) == 0 {
		t.Fatal("expected non-empty action cost profiles")
	}

	gather, ok := profiles[ActionGather]
	if !ok {
		t.Fatal("expected gather profile")
	}
	if gather.DeltaHunger != -2 || gather.DeltaEnergy != -6 {
		t.Fatalf("unexpected gather profile: %+v", gather)
	}

	rest, ok := profiles[ActionRest]
	if !ok {
		t.Fatal("expected rest profile")
	}
	if rest.DeltaHunger != 3 || rest.DeltaEnergy != 20 {
		t.Fatalf("unexpected rest profile: %+v", rest)
	}

	sleep, ok := profiles[ActionSleep]
	if !ok {
		t.Fatal("expected sleep profile")
	}
	if sleep.DeltaHunger != 15 || sleep.DeltaEnergy != SleepBaseEnergyRecovery || sleep.DeltaHP != SleepBaseHPRecovery {
		t.Fatalf("unexpected sleep profile: %+v", sleep)
	}
	if got, ok := sleep.Variants["bed_quality_good"]; !ok {
		t.Fatalf("expected bed_quality_good variant: %+v", sleep.Variants)
	} else if got.DeltaHunger != 20 || got.DeltaEnergy != 45 || got.DeltaHP != 10 {
		t.Fatalf("unexpected bed_quality_good variant: %+v", got)
	}
}
