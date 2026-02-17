package gormrepo

import (
	"context"
	"fmt"
	"strconv"

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
		ObjectType:   obj.ObjectType,
		Quality:      obj.Quality,
		ObjectState:  obj.ObjectState,
	}
	if obj.CapacitySlots > 0 {
		m.CapacitySlots = int32(obj.CapacitySlots)
	}
	if obj.UsedSlots > 0 {
		m.UsedSlots = int32(obj.UsedSlots)
	}
	return getDBFromCtx(ctx, r.db).Create(&m).Error
}

func (r WorldObjectRepo) GetByObjectID(ctx context.Context, agentID, objectID string) (ports.WorldObjectRecord, error) {
	var m model.WorldObject
	err := getDBFromCtx(ctx, r.db).
		Where(&model.WorldObject{OwnerAgentID: agentID, ObjectID: objectID}).
		First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ports.WorldObjectRecord{}, ports.ErrNotFound
		}
		return ports.WorldObjectRecord{}, err
	}
	kind, _ := strconv.Atoi(m.Kind)
	return ports.WorldObjectRecord{
		ObjectID:      m.ObjectID,
		Kind:          kind,
		X:             int(m.X),
		Y:             int(m.Y),
		HP:            int(m.Hp),
		ObjectType:    m.ObjectType,
		Quality:       m.Quality,
		CapacitySlots: int(m.CapacitySlots),
		UsedSlots:     int(m.UsedSlots),
		ObjectState:   m.ObjectState,
	}, nil
}

func (r WorldObjectRepo) ListByAgentID(ctx context.Context, agentID string) ([]ports.WorldObjectRecord, error) {
	var rows []model.WorldObject
	if err := getDBFromCtx(ctx, r.db).Where(&model.WorldObject{OwnerAgentID: agentID}).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ports.WorldObjectRecord, 0, len(rows))
	for _, m := range rows {
		kind, _ := strconv.Atoi(m.Kind)
		out = append(out, ports.WorldObjectRecord{
			ObjectID:      m.ObjectID,
			Kind:          kind,
			X:             int(m.X),
			Y:             int(m.Y),
			HP:            int(m.Hp),
			ObjectType:    m.ObjectType,
			Quality:       m.Quality,
			CapacitySlots: int(m.CapacitySlots),
			UsedSlots:     int(m.UsedSlots),
			ObjectState:   m.ObjectState,
		})
	}
	return out, nil
}

func (r WorldObjectRepo) Update(ctx context.Context, agentID string, obj ports.WorldObjectRecord) error {
	updates := map[string]any{
		"hp":             obj.HP,
		"object_type":    obj.ObjectType,
		"quality":        obj.Quality,
		"capacity_slots": obj.CapacitySlots,
		"used_slots":     obj.UsedSlots,
		"object_state":   obj.ObjectState,
	}
	return getDBFromCtx(ctx, r.db).
		Model(&model.WorldObject{}).
		Where(&model.WorldObject{OwnerAgentID: agentID, ObjectID: obj.ObjectID}).
		Updates(updates).Error
}
