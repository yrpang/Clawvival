package gormrepo

import (
	"context"
	"errors"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"

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
	if err := getDBFromCtx(ctx, r.db).Where("agent_id = ?", agentID).First(&m).Error; err != nil {
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
		Position: survival.Position{X: int(m.X), Y: int(m.Y)},
		Version:  m.Version,
	}, nil
}

func (r AgentStateRepo) SaveWithVersion(ctx context.Context, state survival.AgentStateAggregate, expectedVersion int64) error {
	db := getDBFromCtx(ctx, r.db)
	if expectedVersion == 0 {
		m := model.AgentState{
			AgentID: state.AgentID,
			Hp:      int32(state.Vitals.HP),
			Hunger:  int32(state.Vitals.Hunger),
			Energy:  int32(state.Vitals.Energy),
			X:       int32(state.Position.X),
			Y:       int32(state.Position.Y),
			Version: state.Version,
		}
		if err := db.Create(&m).Error; err != nil {
			return err
		}
		return nil
	}

	updates := map[string]any{
		"hp":      int32(state.Vitals.HP),
		"hunger":  int32(state.Vitals.Hunger),
		"energy":  int32(state.Vitals.Energy),
		"x":       int32(state.Position.X),
		"y":       int32(state.Position.Y),
		"version": state.Version,
	}

	res := db.Model(&model.AgentState{}).
		Where("agent_id = ? AND version = ?", state.AgentID, expectedVersion).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrConflict
	}
	return nil
}
