package survival

type RecipeID int

const (
	RecipePlank RecipeID = 1
	RecipeBread RecipeID = 2
)

type BuildKind int

const (
	BuildBed     BuildKind = 1
	BuildBox     BuildKind = 2
	BuildFarm    BuildKind = 3
	BuildTorch   BuildKind = 4
	BuildWall    BuildKind = 5
	BuildDoor    BuildKind = 6
	BuildFurnace BuildKind = 7
)

type FarmPlot struct {
	GrowthMinutes int
	Ready         bool
}

var recipeDefs = map[RecipeID]struct {
	In  map[string]int
	Out map[string]int
}{
	RecipePlank: {
		In:  map[string]int{"wood": 2},
		Out: map[string]int{"plank": 1},
	},
	RecipeBread: {
		In:  map[string]int{"wheat": 2},
		Out: map[string]int{"bread": 1},
	},
}

var buildCosts = map[BuildKind]map[string]int{
	BuildBed:     {"plank": 4},
	BuildBox:     {"plank": 2},
	BuildFarm:    {"wood": 2, "seed": 1},
	BuildTorch:   {"wood": 1},
	BuildWall:    {"stone": 3},
	BuildDoor:    {"plank": 2},
	BuildFurnace: {"stone": 6},
}

type BuiltObject struct {
	Kind BuildKind
	X    int
	Y    int
}

func ApplyGather(state *AgentStateAggregate, snapshot WorldSnapshot) {
	if state.Inventory == nil {
		state.Inventory = map[string]int{}
	}
	for item, qty := range snapshot.NearbyResource {
		if qty <= 0 {
			continue
		}
		state.Inventory[item] += qty
	}
}

func Craft(state *AgentStateAggregate, recipeID RecipeID) bool {
	recipe, ok := recipeDefs[recipeID]
	if !ok {
		return false
	}
	if !hasEnough(state, recipe.In) {
		return false
	}
	consume(state, recipe.In)
	produce(state, recipe.Out)
	return true
}

func Build(state *AgentStateAggregate, kind BuildKind, x, y int) (BuiltObject, bool) {
	cost, ok := buildCosts[kind]
	if !ok {
		return BuiltObject{}, false
	}
	if !hasEnough(state, cost) {
		return BuiltObject{}, false
	}
	consume(state, cost)
	return BuiltObject{Kind: kind, X: x, Y: y}, true
}

func PlantSeed(state *AgentStateAggregate) (FarmPlot, bool) {
	if !state.ConsumeItem("seed", 1) {
		return FarmPlot{}, false
	}
	return FarmPlot{}, true
}

func TickFarm(plot *FarmPlot, dtMinutes int) {
	if plot.Ready || dtMinutes <= 0 {
		return
	}
	plot.GrowthMinutes += dtMinutes
	if plot.GrowthMinutes >= 120 {
		plot.Ready = true
	}
}

func HarvestFarm(state *AgentStateAggregate, plot *FarmPlot) bool {
	if !plot.Ready {
		return false
	}
	state.AddItem("wheat", 2)
	plot.Ready = false
	plot.GrowthMinutes = 0
	return true
}

func hasEnough(state *AgentStateAggregate, required map[string]int) bool {
	for item, qty := range required {
		if state.Inventory[item] < qty {
			return false
		}
	}
	return true
}

func consume(state *AgentStateAggregate, required map[string]int) {
	for item, qty := range required {
		state.ConsumeItem(item, qty)
	}
}

func produce(state *AgentStateAggregate, out map[string]int) {
	for item, qty := range out {
		state.AddItem(item, qty)
	}
}
