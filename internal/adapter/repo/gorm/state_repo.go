package gormrepo

import (
	"context"
	"encoding/json"
	"errors"

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
		Position:   survival.Position{X: int(m.X), Y: int(m.Y)},
		Inventory:  decodeInventory(m.Inventory),
		Dead:       m.Dead,
		DeathCause: survival.DeathCause(m.DeathCause),
		Version:    m.Version,
	}, nil
}

func (r AgentStateRepo) SaveWithVersion(ctx context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	db := getDBFromCtx(ctx, r.db)
	if expectedVersion == 0 {
		m := model.AgentState{
			AgentID:    state.AgentID,
			Hp:         int32(state.Vitals.HP),
			Hunger:     int32(state.Vitals.Hunger),
			Energy:     int32(state.Vitals.Energy),
			X:          int32(state.Position.X),
			Y:          int32(state.Position.Y),
			Version:    state.Version,
			Inventory:  encodeInventory(state.Inventory),
			Dead:       state.Dead,
			DeathCause: string(state.DeathCause),
		}
		if err := db.Create(&m).Error; err != nil {
			return err
		}
		return nil
	}

	updates := map[string]any{
		"hp":          int32(state.Vitals.HP),
		"hunger":      int32(state.Vitals.Hunger),
		"energy":      int32(state.Vitals.Energy),
		"x":           int32(state.Position.X),
		"y":           int32(state.Position.Y),
		"version":     state.Version,
		"inventory":   encodeInventory(state.Inventory),
		"dead":        state.Dead,
		"death_cause": string(state.DeathCause),
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
