package gormrepo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"

	"gorm.io/gorm"
)

type AgentStateRepo struct {
	db *gorm.DB
}

func NewAgentStateRepo(db *gorm.DB) AgentStateRepo {
	return AgentStateRepo{db: db}
}

func (r AgentStateRepo) GetByAgentID(ctx context.Context, agentID string) (survival.AgentStateAggregate, error) {
	var m model.AgentState
	if err := getDBFromCtx(ctx, r.db).Where(&model.AgentState{AgentID: agentID}).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return survival.AgentStateAggregate{}, ports.ErrNotFound
		}
		return survival.AgentStateAggregate{}, err
	}
	return survival.AgentStateAggregate{
		AgentID: agentID,
		Vitals: survival.Vitals{
			HP:     int(m.Hp),
			Hunger: int(m.Hunger),
			Energy: int(m.Energy),
		},
		Position:          survival.Position{X: int(m.X), Y: int(m.Y)},
		Inventory:         decodeInventory(m.Inventory),
		InventoryCapacity: int(m.InventoryCapacity),
		InventoryUsed:     int(m.InventoryUsed),
		Dead:              m.Dead,
		DeathCause:        survival.DeathCause(m.DeathCause),
		OngoingAction: decodeOngoingAction(
			m.OngoingActionType,
			m.OngoingActionMinutes,
			m.OngoingActionEndAt,
		),
		Version: m.Version,
	}, nil
}

func (r AgentStateRepo) SaveWithVersion(ctx context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	db := getDBFromCtx(ctx, r.db)
	if expectedVersion == 0 {
		m := model.AgentState{
			AgentID:           state.AgentID,
			Hp:                int32(state.Vitals.HP),
			Hunger:            int32(state.Vitals.Hunger),
			Energy:            int32(state.Vitals.Energy),
			X:                 int32(state.Position.X),
			Y:                 int32(state.Position.Y),
			Version:           state.Version,
			Inventory:         encodeInventory(state.Inventory),
			InventoryCapacity: int32(resolveInventoryCapacity(state)),
			InventoryUsed:     int32(resolveInventoryUsed(state)),
			Dead:              state.Dead,
			DeathCause:        string(state.DeathCause),
		}
		applyOngoingActionModel(&m, state.OngoingAction)
		if err := db.Create(&m).Error; err != nil {
			return err
		}
		return nil
	}

	updates := map[string]any{
		"hp":                 int32(state.Vitals.HP),
		"hunger":             int32(state.Vitals.Hunger),
		"energy":             int32(state.Vitals.Energy),
		"x":                  int32(state.Position.X),
		"y":                  int32(state.Position.Y),
		"version":            state.Version,
		"inventory":          encodeInventory(state.Inventory),
		"inventory_capacity": int32(resolveInventoryCapacity(state)),
		"inventory_used":     int32(resolveInventoryUsed(state)),
		"dead":               state.Dead,
		"death_cause":        string(state.DeathCause),
	}
	if state.OngoingAction == nil {
		updates["ongoing_action_type"] = ""
		updates["ongoing_action_end_at"] = time.Time{}
		updates["ongoing_action_minutes"] = 0
	} else {
		updates["ongoing_action_type"] = string(state.OngoingAction.Type)
		updates["ongoing_action_end_at"] = state.OngoingAction.EndAt
		updates["ongoing_action_minutes"] = state.OngoingAction.Minutes
	}

	res := db.Model(&model.AgentState{}).
		Where(&model.AgentState{AgentID: state.AgentID, Version: expectedVersion}).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrConflict
	}
	return nil
}

func encodeInventory(inv map[string]int) string {
	if len(inv) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(inv)
	return string(b)
}

func decodeInventory(raw string) map[string]int {
	if raw == "" {
		return map[string]int{}
	}
	out := map[string]int{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func decodeOngoingAction(actionType string, minutes int32, endAt time.Time) *survival.OngoingActionInfo {
	if actionType == "" || endAt.IsZero() {
		return nil
	}
	if minutes <= 0 {
		return nil
	}
	return &survival.OngoingActionInfo{
		Type:    survival.ActionType(actionType),
		Minutes: int(minutes),
		EndAt:   endAt,
	}
}

func applyOngoingActionModel(m *model.AgentState, ongoing *survival.OngoingActionInfo) {
	if ongoing == nil {
		m.OngoingActionType = ""
		m.OngoingActionEndAt = time.Time{}
		m.OngoingActionMinutes = 0
		return
	}
	endAt := ongoing.EndAt
	m.OngoingActionType = string(ongoing.Type)
	m.OngoingActionEndAt = endAt
	m.OngoingActionMinutes = int32(ongoing.Minutes)
}

func resolveInventoryCapacity(state survival.AgentStateAggregate) int {
	if state.InventoryCapacity > 0 {
		return state.InventoryCapacity
	}
	return 30
}

func resolveInventoryUsed(state survival.AgentStateAggregate) int {
	if state.InventoryUsed > 0 {
		return state.InventoryUsed
	}
	total := 0
	for _, count := range state.Inventory {
		if count > 0 {
			total += count
		}
	}
	return total
}
