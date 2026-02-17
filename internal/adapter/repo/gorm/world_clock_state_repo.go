package gormrepo

import (
	"context"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"

	"gorm.io/gorm"
)

type WorldClockStateRepo struct {
	db *gorm.DB
}

func NewWorldClockStateRepo(db *gorm.DB) WorldClockStateRepo {
	return WorldClockStateRepo{db: db}
}

func (r WorldClockStateRepo) Get(ctx context.Context) (string, time.Time, bool, error) {
	var row model.WorldClockState
	err := r.db.WithContext(ctx).
		Where(&model.WorldClockState{StateKey: "global"}).
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, err
	}
	return row.Phase, row.SwitchedAt, true, nil
}

func (r WorldClockStateRepo) Save(ctx context.Context, phase string, switchedAt time.Time) error {
	return r.db.WithContext(ctx).
		Where(&model.WorldClockState{StateKey: "global"}).
		Assign(model.WorldClockState{
			Phase:      phase,
			SwitchedAt: switchedAt,
			UpdatedAt:  time.Now(),
		}).
		FirstOrCreate(&model.WorldClockState{}).Error
}
