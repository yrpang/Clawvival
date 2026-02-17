package gormrepo

import (
	"context"
	"encoding/json"
	"time"

	"clawverse/internal/adapter/repo/gorm/model"
	"clawverse/internal/domain/world"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorldChunkRepo struct {
	db *gorm.DB
}

func NewWorldChunkRepo(db *gorm.DB) WorldChunkRepo {
	return WorldChunkRepo{db: db}
}

func (r WorldChunkRepo) GetChunk(ctx context.Context, coord world.ChunkCoord, phase string) (world.Chunk, bool, error) {
	var row model.WorldChunk
	err := r.db.WithContext(ctx).
		Where(map[string]any{
			"chunk_x": int32(coord.X),
			"chunk_y": int32(coord.Y),
			"phase":   phase,
		}).
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return world.Chunk{}, false, nil
		}
		return world.Chunk{}, false, err
	}
	tiles, err := decodeChunkTiles(row.Tiles)
	if err != nil {
		return world.Chunk{}, false, err
	}
	return world.Chunk{Coord: coord, Tiles: tiles}, true, nil
}

func (r WorldChunkRepo) SaveChunk(ctx context.Context, coord world.ChunkCoord, phase string, chunk world.Chunk) error {
	b, err := encodeChunkTiles(chunk.Tiles)
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
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chunk_x"}, {Name: "chunk_y"}, {Name: "phase"}},
		DoUpdates: clause.AssignmentColumns([]string{"tiles", "updated_at"}),
	}).Create(&row).Error
}

func encodeChunkTiles(tiles []world.Tile) ([]byte, error) {
	return json.Marshal(tiles)
}

func decodeChunkTiles(data []byte) ([]world.Tile, error) {
	out := []world.Tile{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
