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

	state.AddItem("wood", 8)
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
	if got, want := state.Vitals.Hunger, 80; got != want {
		t.Fatalf("hunger recover mismatch: got=%d want=%d", got, want)
	}

	if ok := Eat(&state, FoodBread); !ok {
		t.Fatalf("expected Eat bread success")
	}
	if got, want := state.Inventory["bread"], 0; got != want {
		t.Fatalf("bread consume mismatch: got=%d want=%d", got, want)
	}
	if got, want := state.Vitals.Hunger, 90; got != want {
		t.Fatalf("hunger recover mismatch after bread: got=%d want=%d", got, want)
	}
}

func TestEatAndCanEat_Wheat(t *testing.T) {
	state := AgentStateAggregate{
		Vitals: Vitals{Hunger: 30},
		Inventory: map[string]int{
			"wheat": 1,
		},
	}
	if !CanEat(state, FoodWheat) {
		t.Fatalf("expected CanEat wheat true")
	}
	if ok := Eat(&state, FoodWheat); !ok {
		t.Fatalf("expected Eat wheat success")
	}
	if got, want := state.Inventory["wheat"], 0; got != want {
		t.Fatalf("wheat consume mismatch: got=%d want=%d", got, want)
	}
	if got := state.Vitals.Hunger; got <= 30 {
		t.Fatalf("expected hunger increase after wheat, got=%d", got)
	}
}

func TestBuildCosts_MVPv1MinimumSet(t *testing.T) {
	state := AgentStateAggregate{Inventory: map[string]int{
		"wood":  14,
		"stone": 2,
		"seed":  1,
		"berry": 2,
	}}
	if _, ok := BuildObject(&state, "bed_rough", 0, 0); !ok {
		t.Fatalf("expected bed_rough build success with wood cost")
	}
	if _, ok := BuildObject(&state, "box", 1, 0); !ok {
		t.Fatalf("expected box build success with wood cost")
	}
	if _, ok := BuildObject(&state, "farm_plot", 1, 1); !ok {
		t.Fatalf("expected farm_plot build success with wood+stone cost")
	}
	if got := state.Inventory["wood"]; got != 0 {
		t.Fatalf("expected wood fully consumed, got=%d", got)
	}
	if got := state.Inventory["stone"]; got != 0 {
		t.Fatalf("expected stone fully consumed, got=%d", got)
	}
	if got := state.Inventory["seed"]; got != 1 {
		t.Fatalf("expected seed not consumed by build, got=%d", got)
	}
}

func TestProductionRecipeRules_ExposeStableCatalog(t *testing.T) {
	rules := ProductionRecipeRules()
	if len(rules) < 2 {
		t.Fatalf("expected at least 2 production recipes, got=%d", len(rules))
	}
	if got := rules[0]; got.RecipeID != int(RecipePlank) || got.In["wood"] != 2 || got.Out["plank"] != 1 {
		t.Fatalf("unexpected first production recipe: %+v", got)
	}
	if got := rules[1]; got.RecipeID != int(RecipeBread) || got.In["wheat"] != 2 || got.Out["bread"] != 1 {
		t.Fatalf("unexpected second production recipe: %+v", got)
	}
}

func TestProductionRecipeRules_CoversAllRuntimeRecipes(t *testing.T) {
	rules := ProductionRecipeRules()
	if got, want := len(rules), len(recipeDefs); got != want {
		t.Fatalf("production recipe count mismatch: got=%d want=%d", got, want)
	}
	exported := map[int]ProductionRecipeRule{}
	for _, r := range rules {
		exported[r.RecipeID] = r
	}
	for id, def := range recipeDefs {
		r, ok := exported[int(id)]
		if !ok {
			t.Fatalf("missing exported recipe for runtime recipe_id=%d", id)
		}
		if !sameRecipeMap(r.In, def.In) {
			t.Fatalf("recipe_id=%d input mismatch: got=%v want=%v", id, r.In, def.In)
		}
		if !sameRecipeMap(r.Out, def.Out) {
			t.Fatalf("recipe_id=%d output mismatch: got=%v want=%v", id, r.Out, def.Out)
		}
	}
}

func TestBuildCostRules_CoversAllRuntimeBuildDefs(t *testing.T) {
	rules := BuildCostRules()
	if got, want := len(rules), len(buildDefsByObjectType); got != want {
		t.Fatalf("build cost count mismatch: got=%d want=%d", got, want)
	}
	for objectType, def := range buildDefsByObjectType {
		cost, ok := rules[objectType]
		if !ok {
			t.Fatalf("missing exported build cost for object_type=%s", objectType)
		}
		if !sameRecipeMap(cost, def.Cost) {
			t.Fatalf("object_type=%s build cost mismatch: got=%v want=%v", objectType, cost, def.Cost)
		}
	}
}

func sameRecipeMap(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
