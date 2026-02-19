package action

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

type farmPlantActionHandler struct{ BaseHandler }
type farmHarvestActionHandler struct{ BaseHandler }
type containerDepositActionHandler struct{ BaseHandler }
type containerWithdrawActionHandler struct{ BaseHandler }
type buildActionHandler struct{ BaseHandler }

func validateBuildActionParams(intent survival.ActionIntent) bool {
	_, ok := buildKindFromObjectType(intent.ObjectType)
	return ok && intent.Pos != nil
}

func validateFarmPlantActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.FarmID) != ""
}

func validateFarmHarvestActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.FarmID) != ""
}

func validateContainerActionParams(intent survival.ActionIntent) bool {
	return strings.TrimSpace(intent.ContainerID) != "" && hasValidItems(intent.Items)
}

func (h buildActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if err := runStandardActionPrecheck(ctx, uc, ac); err != nil {
		return err
	}
	if !buildPreconditionsSatisfied(ac.View.StateWorking, ac.Tmp.ResolvedIntent) {
		return ErrActionPreconditionFailed
	}
	return nil
}

func (h buildActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func (h farmPlantActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if err := runStandardActionPrecheck(ctx, uc, ac); err != nil {
		return err
	}
	if !survival.CanPlantSeed(ac.View.StateWorking) {
		return ErrActionPreconditionFailed
	}
	return nil
}

func (h farmPlantActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func (h farmHarvestActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h farmHarvestActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func (h containerDepositActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h containerDepositActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func (h containerWithdrawActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h containerWithdrawActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

type preparedObjectAction struct {
	record ports.WorldObjectRecord
	box    boxObjectState
	farm   farmObjectState
}

type boxObjectState struct {
	Inventory map[string]int `json:"inventory"`
}

type farmObjectState struct {
	State         string `json:"state"`
	PlantedAtUnix int64  `json:"planted_at_unix,omitempty"`
	ReadyAtUnix   int64  `json:"ready_at_unix,omitempty"`
}

func prepareObjectAction(ctx context.Context, nowAt time.Time, state survival.AgentStateAggregate, intent survival.ActionIntent, repo ports.WorldObjectRepository, agentID string) (*preparedObjectAction, error) {
	if repo == nil {
		switch intent.Type {
		case survival.ActionSleep, survival.ActionFarmPlant, survival.ActionFarmHarvest, survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
			return nil, ErrActionPreconditionFailed
		}
		return nil, nil
	}
	switch intent.Type {
	case survival.ActionSleep:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.BedID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isBedObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		if obj.X != state.Position.X || obj.Y != state.Position.Y {
			return nil, ErrActionPreconditionFailed
		}
		return &preparedObjectAction{record: obj}, nil
	case survival.ActionContainerDeposit, survival.ActionContainerWithdraw:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.ContainerID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isBoxObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		box, err := parseBoxObjectState(obj.ObjectState)
		if err != nil {
			return nil, ErrActionPreconditionFailed
		}
		total := 0
		requested := aggregateItemCounts(intent.Items)
		for _, item := range intent.Items {
			total += item.Count
		}
		for itemType, need := range requested {
			switch intent.Type {
			case survival.ActionContainerDeposit:
				if state.Inventory[itemType] < need {
					return nil, ErrActionPreconditionFailed
				}
			case survival.ActionContainerWithdraw:
				if box.Inventory[itemType] < need {
					return nil, ErrActionPreconditionFailed
				}
			}
		}
		if intent.Type == survival.ActionContainerDeposit && obj.CapacitySlots > 0 && obj.UsedSlots+total > obj.CapacitySlots {
			return nil, ErrContainerFull
		}
		if intent.Type == survival.ActionContainerWithdraw {
			capacity := state.InventoryCapacity
			if capacity <= 0 {
				capacity = defaultInventoryCapacity
			}
			if inventoryUsed(state.Inventory)+total > capacity {
				return nil, ErrInventoryFull
			}
		}
		return &preparedObjectAction{record: obj, box: box}, nil
	case survival.ActionFarmPlant, survival.ActionFarmHarvest:
		obj, err := repo.GetByObjectID(ctx, agentID, intent.FarmID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				return nil, ErrActionPreconditionFailed
			}
			return nil, err
		}
		if !isFarmObject(obj) {
			return nil, ErrActionPreconditionFailed
		}
		farm, err := parseFarmObjectState(obj.ObjectState)
		if err != nil {
			return nil, ErrActionPreconditionFailed
		}
		switch intent.Type {
		case survival.ActionFarmPlant:
			if strings.ToUpper(strings.TrimSpace(farm.State)) != "IDLE" {
				return nil, ErrActionPreconditionFailed
			}
		case survival.ActionFarmHarvest:
			ready := strings.ToUpper(strings.TrimSpace(farm.State)) == "READY"
			if strings.ToUpper(strings.TrimSpace(farm.State)) == "GROWING" && farm.ReadyAtUnix > 0 && nowAt.Unix() >= farm.ReadyAtUnix {
				ready = true
			}
			if !ready {
				return nil, ErrActionPreconditionFailed
			}
		}
		return &preparedObjectAction{record: obj, farm: farm}, nil
	default:
		return nil, nil
	}
}

func persistObjectAction(ctx context.Context, nowAt time.Time, intent survival.ActionIntent, prepared *preparedObjectAction, repo ports.WorldObjectRepository, agentID string) error {
	if repo == nil || prepared == nil {
		return nil
	}
	obj := prepared.record
	switch intent.Type {
	case survival.ActionContainerDeposit:
		if prepared.box.Inventory == nil {
			prepared.box.Inventory = map[string]int{}
		}
		for _, item := range intent.Items {
			prepared.box.Inventory[item.ItemType] += item.Count
			obj.UsedSlots += item.Count
		}
		encoded, err := json.Marshal(prepared.box)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionContainerWithdraw:
		if prepared.box.Inventory == nil {
			prepared.box.Inventory = map[string]int{}
		}
		for _, item := range intent.Items {
			prepared.box.Inventory[item.ItemType] -= item.Count
			if prepared.box.Inventory[item.ItemType] <= 0 {
				delete(prepared.box.Inventory, item.ItemType)
			}
			obj.UsedSlots -= item.Count
		}
		if obj.UsedSlots < 0 {
			obj.UsedSlots = 0
		}
		encoded, err := json.Marshal(prepared.box)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionFarmPlant:
		prepared.farm.State = "GROWING"
		prepared.farm.PlantedAtUnix = nowAt.Unix()
		prepared.farm.ReadyAtUnix = nowAt.Add(defaultFarmGrowMinutes * time.Minute).Unix()
		encoded, err := json.Marshal(prepared.farm)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	case survival.ActionFarmHarvest:
		prepared.farm.State = "IDLE"
		prepared.farm.PlantedAtUnix = 0
		prepared.farm.ReadyAtUnix = 0
		encoded, err := json.Marshal(prepared.farm)
		if err != nil {
			return err
		}
		obj.ObjectState = string(encoded)
		return repo.Update(ctx, agentID, obj)
	default:
		return nil
	}
}

func parseBoxObjectState(raw string) (boxObjectState, error) {
	out := boxObjectState{Inventory: map[string]int{}}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return boxObjectState{}, err
	}
	if out.Inventory == nil {
		out.Inventory = map[string]int{}
	}
	return out, nil
}

func parseFarmObjectState(raw string) (farmObjectState, error) {
	out := farmObjectState{State: "IDLE"}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return farmObjectState{}, err
	}
	if strings.TrimSpace(out.State) == "" {
		out.State = "IDLE"
	}
	return out, nil
}

func isBoxObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "box" || obj.Kind == int(survival.BuildBox)
}

func isFarmObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "farm_plot" || obj.Kind == int(survival.BuildFarm)
}

func isBedObject(obj ports.WorldObjectRecord) bool {
	typ := strings.ToLower(strings.TrimSpace(obj.ObjectType))
	return typ == "bed" || obj.Kind == int(survival.BuildBed)
}

func buildObjectDefaults(intentObjectType string) (objectType, quality string, capacitySlots int, objectState string) {
	switch strings.ToLower(strings.TrimSpace(intentObjectType)) {
	case "box":
		return "box", "", 60, `{"inventory":{}}`
	case "farm_plot":
		return "farm_plot", "", 0, `{"state":"IDLE"}`
	case "bed_good":
		return "bed", "GOOD", 0, ""
	case "bed_rough":
		return "bed", "ROUGH", 0, ""
	case "bed":
		return "bed", "ROUGH", 0, ""
	default:
		return strings.ToLower(strings.TrimSpace(intentObjectType)), "", 0, ""
	}
}

func inventoryUsed(items map[string]int) int {
	total := 0
	for _, c := range items {
		if c > 0 {
			total += c
		}
	}
	return total
}

func aggregateItemCounts(items []survival.ItemAmount) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ItemType))
		if key == "" || item.Count <= 0 {
			continue
		}
		out[key] += item.Count
	}
	return out
}

func hasValidItems(items []survival.ItemAmount) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item.ItemType) == "" || item.Count <= 0 {
			return false
		}
	}
	return true
}

func buildPreconditionsSatisfied(state survival.AgentStateAggregate, intent survival.ActionIntent) bool {
	if intent.Type != survival.ActionBuild {
		return true
	}
	_, ok := buildKindFromObjectType(intent.ObjectType)
	return ok && survival.CanBuildObjectType(state, intent.ObjectType)
}

func buildKindFromObjectType(objectType string) (survival.BuildKind, bool) {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "bed", "bed_rough", "bed_good":
		return survival.BuildBed, true
	case "box":
		return survival.BuildBox, true
	case "farm_plot":
		return survival.BuildFarm, true
	case "torch":
		return survival.BuildTorch, true
	default:
		return 0, false
	}
}

func attachBuiltObjectIDs(events []survival.DomainEvent, ids []string) {
	if len(ids) == 0 {
		return
	}
	for i := range events {
		if events[i].Type != "action_settled" || events[i].Payload == nil {
			continue
		}
		result, _ := events[i].Payload["result"].(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		result["built_object_ids"] = ids
		events[i].Payload["result"] = result
	}
}
