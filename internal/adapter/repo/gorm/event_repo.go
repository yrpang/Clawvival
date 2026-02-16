package gormrepo

import (
	"context"
	"encoding/json"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/domain/survival"

	"gorm.io/gorm"
)

type EventRepo struct {
	db *gorm.DB
}

func NewEventRepo(db *gorm.DB) EventRepo {
	return EventRepo{db: db}
}

func (r EventRepo) Append(ctx context.Context, events []survival.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]model.DomainEvent, 0, len(events))
	for _, e := range events {
		b, _ := json.Marshal(e.Payload)
		rows = append(rows, model.DomainEvent{
			Type:       e.Type,
			OccurredAt: e.OccurredAt,
			Payload:    b,
		})
	}
	return getDBFromCtx(ctx, r.db).Create(&rows).Error
}
