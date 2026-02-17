package gormrepo

import (
	"context"
	"encoding/json"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventRepo struct {
	db *gorm.DB
}

func NewEventRepo(db *gorm.DB) EventRepo {
	return EventRepo{db: db}
}

func (r EventRepo) Append(ctx context.Context, agentID string, events []survival.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]model.DomainEvent, 0, len(events))
	for _, e := range events {
		b, _ := json.Marshal(e.Payload)
		rows = append(rows, model.DomainEvent{
			AgentID:    agentID,
			Type:       e.Type,
			OccurredAt: e.OccurredAt,
			Payload:    b,
		})
	}
	return getDBFromCtx(ctx, r.db).Create(&rows).Error
}

func (r EventRepo) ListByAgentID(ctx context.Context, agentID string, limit int) ([]survival.DomainEvent, error) {
	rows := []model.DomainEvent{}
	query := getDBFromCtx(ctx, r.db).
		Where(&model.DomainEvent{AgentID: agentID}).
		Clauses(clause.OrderBy{
			Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "occurred_at"}, Desc: true}},
		})
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ports.ErrNotFound
	}

	out := make([]survival.DomainEvent, 0, len(rows))
	for _, row := range rows {
		var payload map[string]any
		if len(row.Payload) > 0 {
			_ = json.Unmarshal(row.Payload, &payload)
		}
		out = append(out, survival.DomainEvent{
			Type:       row.Type,
			OccurredAt: row.OccurredAt,
			Payload:    payload,
		})
	}
	return out, nil
}
