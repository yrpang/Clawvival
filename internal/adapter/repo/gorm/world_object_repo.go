package gormrepo

import (
	"context"
	"fmt"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"

	"gorm.io/gorm"
)

type WorldObjectRepo struct {
	db *gorm.DB
}

func NewWorldObjectRepo(db *gorm.DB) WorldObjectRepo {
	return WorldObjectRepo{db: db}
}

func (r WorldObjectRepo) Save(ctx context.Context, agentID string, obj ports.WorldObjectRecord) error {
	m := model.WorldObject{
		ObjectID:     obj.ObjectID,
		Kind:         fmt.Sprintf("%d", obj.Kind),
		X:            int32(obj.X),
		Y:            int32(obj.Y),
		Hp:           int32(obj.HP),
		OwnerAgentID: agentID,
	}
	return getDBFromCtx(ctx, r.db).Create(&m).Error
}
