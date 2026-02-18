package gormrepo

import (
	"context"
	"errors"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentResourceNodeRepo struct {
	db *gorm.DB
}

func NewAgentResourceNodeRepo(db *gorm.DB) AgentResourceNodeRepo {
	return AgentResourceNodeRepo{db: db}
}

func (r AgentResourceNodeRepo) Upsert(ctx context.Context, record ports.AgentResourceNodeRecord) error {
	row := model.AgentResourceNode{
		AgentID:       record.AgentID,
		TargetID:      record.TargetID,
		ResourceType:  record.ResourceType,
		X:             int32(record.X),
		Y:             int32(record.Y),
		DepletedUntil: record.DepletedUntil,
		UpdatedAt:     time.Now().UTC(),
	}
	return getDBFromCtx(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "agent_id"}, {Name: "target_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"resource_type", "x", "y", "depleted_until", "updated_at"}),
		}).
		Create(&row).Error
}

func (r AgentResourceNodeRepo) GetByTargetID(ctx context.Context, agentID, targetID string) (ports.AgentResourceNodeRecord, error) {
	var row model.AgentResourceNode
	err := getDBFromCtx(ctx, r.db).
		Where(&model.AgentResourceNode{AgentID: agentID, TargetID: targetID}).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentResourceNodeRecord{}, ports.ErrNotFound
		}
		return ports.AgentResourceNodeRecord{}, err
	}
	return ports.AgentResourceNodeRecord{
		AgentID:       row.AgentID,
		TargetID:      row.TargetID,
		ResourceType:  row.ResourceType,
		X:             int(row.X),
		Y:             int(row.Y),
		DepletedUntil: row.DepletedUntil,
	}, nil
}

func (r AgentResourceNodeRepo) ListByAgentID(ctx context.Context, agentID string) ([]ports.AgentResourceNodeRecord, error) {
	var rows []model.AgentResourceNode
	if err := getDBFromCtx(ctx, r.db).
		Where(&model.AgentResourceNode{AgentID: agentID}).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ports.ErrNotFound
	}
	out := make([]ports.AgentResourceNodeRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.AgentResourceNodeRecord{
			AgentID:       row.AgentID,
			TargetID:      row.TargetID,
			ResourceType:  row.ResourceType,
			X:             int(row.X),
			Y:             int(row.Y),
			DepletedUntil: row.DepletedUntil,
		})
	}
	return out, nil
}
