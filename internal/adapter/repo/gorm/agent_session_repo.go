package gormrepo

import (
	"context"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/domain/survival"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentSessionRepo struct {
	db *gorm.DB
}

func NewAgentSessionRepo(db *gorm.DB) AgentSessionRepo {
	return AgentSessionRepo{db: db}
}

func (r AgentSessionRepo) EnsureActive(ctx context.Context, sessionID, agentID string, startTick int64) error {
	m := model.AgentSession{
		SessionID: sessionID,
		AgentID:   agentID,
		StartTick: startTick,
		Status:    "alive",
	}
	return getDBFromCtx(ctx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error
}

func (r AgentSessionRepo) Close(ctx context.Context, sessionID string, cause survival.DeathCause, endedAt time.Time) error {
	updates := map[string]any{
		"status":      "dead",
		"death_cause": string(cause),
		"ended_at":    endedAt,
	}
	res := getDBFromCtx(ctx, r.db).
		Model(&model.AgentSession{}).
		Where(&model.AgentSession{SessionID: sessionID}).
		Updates(updates)
	return res.Error
}
