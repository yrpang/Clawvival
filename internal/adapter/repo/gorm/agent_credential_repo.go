package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"

	"gorm.io/gorm"
)

type AgentCredentialRepo struct {
	db *gorm.DB
}

func NewAgentCredentialRepo(db *gorm.DB) AgentCredentialRepo {
	return AgentCredentialRepo{db: db}
}

func (r AgentCredentialRepo) Create(ctx context.Context, credential ports.AgentCredentialRecord) error {
	row := model.AgentCredential{
		AgentID:   credential.AgentID,
		KeySalt:   credential.KeySalt,
		KeyHash:   credential.KeyHash,
		Status:    credential.Status,
		CreatedAt: credential.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	if err := getDBFromCtx(ctx, r.db).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return err
	}
	return nil
}

func (r AgentCredentialRepo) GetByAgentID(ctx context.Context, agentID string) (ports.AgentCredentialRecord, error) {
	var row model.AgentCredential
	if err := getDBFromCtx(ctx, r.db).Where(&model.AgentCredential{AgentID: agentID}).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentCredentialRecord{}, ports.ErrNotFound
		}
		return ports.AgentCredentialRecord{}, err
	}
	return ports.AgentCredentialRecord{
		AgentID:   row.AgentID,
		KeySalt:   row.KeySalt,
		KeyHash:   row.KeyHash,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
	}, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
