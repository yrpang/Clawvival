package runtime

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type worldClockStateRow struct {
	ID         int64
	StateKey   string
	Phase      string
	SwitchedAt time.Time
	UpdatedAt  time.Time
}

func (worldClockStateRow) TableName() string { return "world_clock_state" }

type GormClockStateStore struct {
	db *gorm.DB
}

func NewGormClockStateStore(db *gorm.DB) GormClockStateStore {
	return GormClockStateStore{db: db}
}

func (s GormClockStateStore) Get(ctx context.Context) (string, time.Time, bool, error) {
	var row worldClockStateRow
	err := s.db.WithContext(ctx).Where("state_key = ?", "global").First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, err
	}
	return row.Phase, row.SwitchedAt, true, nil
}

func (s GormClockStateStore) Save(ctx context.Context, phase string, switchedAt time.Time) error {
	row := worldClockStateRow{
		StateKey:   "global",
		Phase:      phase,
		SwitchedAt: switchedAt,
		UpdatedAt:  time.Now(),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "state_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"phase", "switched_at", "updated_at"}),
	}).Create(&row).Error
}
