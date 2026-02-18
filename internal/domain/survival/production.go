package survival

import "strings"

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

type FoodID int

const (
	FoodBerry FoodID = 1
	FoodBread FoodID = 2
	FoodWheat FoodID = 3
)

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
	BuildBed:     {"wood": 8},
	BuildBox:     {"wood": 4},
	BuildFarm:    {"wood": 2, "stone": 2},
	BuildTorch:   {"wood": 1},
	BuildWall:    {"stone": 3},
	BuildDoor:    {"wood": 2},
	BuildFurnace: {"stone": 6},
}

var buildDefsByObjectType = map[string]struct {
	Kind BuildKind
	Cost map[string]int
}{
	"bed":       {Kind: BuildBed, Cost: map[string]int{"wood": 8}},
	"bed_rough": {Kind: BuildBed, Cost: map[string]int{"wood": 8}},
	"bed_good":  {Kind: BuildBed, Cost: map[string]int{"wood": 6, "berry": 2}},
	"box":       {Kind: BuildBox, Cost: map[string]int{"wood": 4}},
	"farm_plot": {Kind: BuildFarm, Cost: map[string]int{"wood": 2, "stone": 2}},
	"torch":     {Kind: BuildTorch, Cost: map[string]int{"wood": 1}},
	"wall":      {Kind: BuildWall, Cost: map[string]int{"stone": 3}},
	"door":      {Kind: BuildDoor, Cost: map[string]int{"wood": 2}},
	"furnace":   {Kind: BuildFurnace, Cost: map[string]int{"stone": 6}},
}

var foodDefs = map[FoodID]struct {
	ItemName      string
	HungerRecover int
}{
	FoodBerry: {ItemName: "berry", HungerRecover: 12},
	FoodBread: {ItemName: "bread", HungerRecover: 28},
	FoodWheat: {ItemName: "wheat", HungerRecover: 16},
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
		state.Inventory[item] += qty * gatherMultiplier(state, item)
	}
}

func gatherMultiplier(state *AgentStateAggregate, item string) int {
	switch item {
	case "wood":
		if state.Inventory["tool_axe"] > 0 {
			return 2
		}
	case "stone":
		if state.Inventory["tool_pickaxe"] > 0 {
			return 2
		}
	}
	return 1
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
	return BuildObject(state, kindToObjectType(kind), x, y)
}

func BuildObject(state *AgentStateAggregate, objectType string, x, y int) (BuiltObject, bool) {
	def, ok := buildDefsByObjectType[strings.ToLower(strings.TrimSpace(objectType))]
	if !ok {
		return BuiltObject{}, false
	}
	if !hasEnough(state, def.Cost) {
		return BuiltObject{}, false
	}
	consume(state, def.Cost)
	return BuiltObject{Kind: def.Kind, X: x, Y: y}, true
}

func CanCraft(state AgentStateAggregate, recipeID RecipeID) bool {
	recipe, ok := recipeDefs[recipeID]
	if !ok {
		return false
	}
	return hasEnough(&state, recipe.In)
}

func CanBuild(state AgentStateAggregate, kind BuildKind) bool {
	cost, ok := buildCosts[kind]
	if !ok {
		return false
	}
	return hasEnough(&state, cost)
}

func CanBuildObjectType(state AgentStateAggregate, objectType string) bool {
	def, ok := buildDefsByObjectType[strings.ToLower(strings.TrimSpace(objectType))]
	if !ok {
		return false
	}
	return hasEnough(&state, def.Cost)
}

func CanPlantSeed(state AgentStateAggregate) bool {
	return state.Inventory["seed"] > 0
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
	if plot.GrowthMinutes >= 60 {
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

func CanEat(state AgentStateAggregate, foodID FoodID) bool {
	food, ok := foodDefs[foodID]
	if !ok {
		return false
	}
	return state.Inventory[food.ItemName] > 0
}

func Eat(state *AgentStateAggregate, foodID FoodID) bool {
	food, ok := foodDefs[foodID]
	if !ok {
		return false
	}
	if !state.ConsumeItem(food.ItemName, 1) {
		return false
	}
	state.Vitals.Hunger += food.HungerRecover
	if state.Vitals.Hunger > 100 {
		state.Vitals.Hunger = 100
	}
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

func kindToObjectType(kind BuildKind) string {
	switch kind {
	case BuildBed:
		return "bed_rough"
	case BuildBox:
		return "box"
	case BuildFarm:
		return "farm_plot"
	case BuildTorch:
		return "torch"
	case BuildWall:
		return "wall"
	case BuildDoor:
		return "door"
	case BuildFurnace:
		return "furnace"
	default:
		return ""
	}
}
