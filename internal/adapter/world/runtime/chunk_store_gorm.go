package runtime

import (
	"context"
	"time"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/domain/world"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormChunkStore struct {
	db *gorm.DB
}

func NewGormChunkStore(db *gorm.DB) GormChunkStore {
	return GormChunkStore{db: db}
}

func (s GormChunkStore) GetChunk(ctx context.Context, coord world.ChunkCoord, phase string) (world.Chunk, bool, error) {
	var row model.WorldChunk
	err := s.db.WithContext(ctx).
		Where(&model.WorldChunk{ChunkX: int32(coord.X), ChunkY: int32(coord.Y), Phase: phase}).
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return world.Chunk{}, false, nil
		}
		return world.Chunk{}, false, err
	}
	tiles, err := unmarshalChunkTiles(row.Tiles)
	if err != nil {
		return world.Chunk{}, false, err
	}
	return world.Chunk{Coord: coord, Tiles: tiles}, true, nil
}

func (s GormChunkStore) SaveChunk(ctx context.Context, coord world.ChunkCoord, phase string, chunk world.Chunk) error {
	b, err := marshalChunkTiles(chunk.Tiles)
	if err != nil {
		return err
	}
	row := model.WorldChunk{
		ChunkX:    int32(coord.X),
		ChunkY:    int32(coord.Y),
		Phase:     phase,
		Tiles:     b,
		UpdatedAt: time.Now(),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chunk_x"}, {Name: "chunk_y"}, {Name: "phase"}},
		DoUpdates: clause.AssignmentColumns([]string{"tiles", "updated_at"}),
	}).Create(&row).Error
}
