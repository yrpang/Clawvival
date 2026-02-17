package gormrepo

import (
	"context"
	"encoding/json"
	"errors"

	"clawvival/internal/adapter/repo/gorm/model"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"

	"gorm.io/gorm"
)

type ActionExecutionRepo struct {
	db *gorm.DB
}

func NewActionExecutionRepo(db *gorm.DB) ActionExecutionRepo {
	return ActionExecutionRepo{db: db}
}

func (r ActionExecutionRepo) GetByIdempotencyKey(ctx context.Context, agentID, key string) (*ports.ActionExecutionRecord, error) {
	var m model.ActionExecution
	err := getDBFromCtx(ctx, r.db).
		Where(&model.ActionExecution{AgentID: agentID, IdempotencyKey: key}).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &ports.ActionExecutionRecord{
		AgentID:        m.AgentID,
		IdempotencyKey: m.IdempotencyKey,
		Result:         decodeResult(m),
		AppliedAt:      m.AppliedAt,
	}, nil
}

func (r ActionExecutionRepo) SaveExecution(ctx context.Context, execution ports.ActionExecutionRecord) error {
	stateJSON, _ := json.Marshal(execution.Result.UpdatedState)
	eventsJSON, _ := json.Marshal(execution.Result.Events)
	m := model.ActionExecution{
		AgentID:        execution.AgentID,
		IdempotencyKey: execution.IdempotencyKey,
		IntentType:     execution.IntentType,
		Dt:             int32(execution.DT),
		ResultCode:     string(execution.Result.ResultCode),
		UpdatedState:   stateJSON,
		Events:         eventsJSON,
		AppliedAt:      execution.AppliedAt,
	}
	if err := getDBFromCtx(ctx, r.db).Create(&m).Error; err != nil {
		return err
	}
	return nil
}

func decodeResult(m model.ActionExecution) ports.ActionResult {
	var state survival.AgentStateAggregate
	var events []survival.DomainEvent
	_ = json.Unmarshal(m.UpdatedState, &state)
	_ = json.Unmarshal(m.Events, &events)
	return ports.ActionResult{
		UpdatedState: state,
		Events:       events,
		ResultCode:   survival.ResultCode(m.ResultCode),
	}
}
