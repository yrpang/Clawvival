package survival

import "testing"

func TestProductionLoop_GatherCraftBuildFarm(t *testing.T) {
	state := AgentStateAggregate{}

	ApplyGather(&state, WorldSnapshot{NearbyResource: map[string]int{"wood": 3, "stone": 2}})
	if state.Inventory["wood"] < 3 {
		t.Fatalf("expected gathered wood")
	}

	state.AddItem("wood", 5)
	if ok := Craft(&state, RecipePlank); !ok {
		t.Fatalf("expected craft plank success")
	}
	if state.Inventory["plank"] == 0 {
		t.Fatalf("expected plank output")
	}

	state.AddItem("plank", 4)
	if _, ok := Build(&state, BuildBed, 0, 0); !ok {
		t.Fatalf("expected build bed success")
	}

	state.AddItem("seed", 1)
	plot, ok := PlantSeed(&state)
	if !ok {
		t.Fatalf("expected plant seed success")
	}
	for i := 0; i < 5; i++ {
		TickFarm(&plot, 30)
	}
	if !plot.Ready {
		t.Fatalf("expected farm plot ready")
	}
	HarvestFarm(&state, &plot)
	if state.Inventory["wheat"] == 0 {
		t.Fatalf("expected harvest wheat")
	}
}

func TestCraftRejectsMissingInput(t *testing.T) {
	state := AgentStateAggregate{}
	if ok := Craft(&state, RecipePlank); ok {
		t.Fatalf("expected craft fail when missing input")
	}
}
