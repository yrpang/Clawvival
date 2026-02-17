package runtime

import (
	"context"
	"time"

	"clawverse/internal/adapter/repo/gorm/model"

	"gorm.io/gorm"
)

type GormClockStateStore struct {
	db *gorm.DB
}

func NewGormClockStateStore(db *gorm.DB) GormClockStateStore {
	return GormClockStateStore{db: db}
}

func (s GormClockStateStore) Get(ctx context.Context) (string, time.Time, bool, error) {
	var row model.WorldClockState
	err := s.db.WithContext(ctx).
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

func (s GormClockStateStore) Save(ctx context.Context, phase string, switchedAt time.Time) error {
	return s.db.WithContext(ctx).
		Where(&model.WorldClockState{StateKey: "global"}).
		Assign(model.WorldClockState{
			Phase:      phase,
			SwitchedAt: switchedAt,
			UpdatedAt:  time.Now(),
		}).
		FirstOrCreate(&model.WorldClockState{}).Error
}
