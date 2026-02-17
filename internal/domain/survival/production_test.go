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

func TestApplyGather_UsesToolEfficiency(t *testing.T) {
	state := AgentStateAggregate{
		Inventory: map[string]int{
			"tool_axe":     1,
			"tool_pickaxe": 1,
		},
	}
	ApplyGather(&state, WorldSnapshot{NearbyResource: map[string]int{"wood": 2, "stone": 3, "berry": 1}})

	if got, want := state.Inventory["wood"], 4; got != want {
		t.Fatalf("wood gather mismatch: got=%d want=%d", got, want)
	}
	if got, want := state.Inventory["stone"], 6; got != want {
		t.Fatalf("stone gather mismatch: got=%d want=%d", got, want)
	}
	if got, want := state.Inventory["berry"], 1; got != want {
		t.Fatalf("berry gather mismatch: got=%d want=%d", got, want)
	}
}

func TestEatAndCanEat(t *testing.T) {
	state := AgentStateAggregate{
		Vitals: Vitals{Hunger: 70},
		Inventory: map[string]int{
			"berry": 1,
			"bread": 1,
		},
	}
	if !CanEat(state, FoodBerry) {
		t.Fatalf("expected CanEat berry true")
	}
	if ok := Eat(&state, FoodBerry); !ok {
		t.Fatalf("expected Eat berry success")
	}
	if got, want := state.Inventory["berry"], 0; got != want {
		t.Fatalf("berry consume mismatch: got=%d want=%d", got, want)
	}
	if got, want := state.Vitals.Hunger, 82; got != want {
		t.Fatalf("hunger recover mismatch: got=%d want=%d", got, want)
	}

	if ok := Eat(&state, FoodBread); !ok {
		t.Fatalf("expected Eat bread success")
	}
	if got, want := state.Inventory["bread"], 0; got != want {
		t.Fatalf("bread consume mismatch: got=%d want=%d", got, want)
	}
	if got, want := state.Vitals.Hunger, 100; got != want {
		t.Fatalf("hunger should cap at 100: got=%d want=%d", got, want)
	}
}
