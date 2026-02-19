# Action UseCase Engine Skeleton Example

> This is a blueprint-style example for refactoring structure. It is not wired into production code directly.

```go
package action

import (
	"context"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type ActionMode int

const (
	// 普通动作：本次请求内完成结算
	ActionModeSettle ActionMode = iota
	// 启动进行中动作（如 rest）
	ActionModeStartOngoing
	// 只处理 ongoing 的收尾（如 terminate）
	ActionModeFinalizeOnly
)

type ExecuteMode int

const (
	// 继续主流程（通常进入 persist/respond）
	ExecuteModeContinue ExecuteMode = iota
	// 业务已完成；仍需先走统一 persist/respond 再返回
	ExecuteModeCompleted
)

type ActionSpec struct {
	Type         survival.ActionType
	Mode         ActionMode
	CanTerminate bool // 仅对 ongoing 类型动作有意义
	Handler      ActionHandler
}

type ActionHandler interface {
	BuildContext(ctx context.Context, uc UseCase, ac *ActionContext) error
	Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error
	ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error)
}

type BaseHandler struct{}

func (BaseHandler) BuildContext(context.Context, UseCase, *ActionContext) error { return nil }
func (BaseHandler) Precheck(context.Context, UseCase, *ActionContext) error     { return nil }
func (BaseHandler) ExecuteActionAndPlan(context.Context, UseCase, *ActionContext) (ExecuteMode, error) {
	return ExecuteModeContinue, nil
}

type ActionContext struct {
	In struct {
		Req     Request
		NowAt   time.Time
		AgentID string
	}
	View struct {
		Spec         ActionSpec
		StateBefore  survival.AgentStateAggregate
		Snapshot     world.Snapshot
		EventsBefore []survival.DomainEvent
		PreparedObj  *preparedObjectAction
	}
	Plan struct {
		StateToSave     *survival.AgentStateAggregate
		EventsToAppend  []survival.DomainEvent
		ExecutionToSave *ports.ActionExecutionRecord
		// ObjectOps/ResourceOps/CloseSession omitted for brevity in this example.
	}
	Tmp struct {
		ResolvedIntent survival.ActionIntent
		DeltaMinutes   int
		SettleResult   survival.SettlementResult
		Completed      bool
	}
}

type ActionEngine struct {
	Registry map[survival.ActionType]ActionSpec

	TxManager    ports.TxManager
	StateRepo    ports.AgentStateRepository
	ActionRepo   ports.ActionExecutionRepository
	EventRepo    ports.EventRepository
	ObjectRepo   ports.WorldObjectRepository
	ResourceRepo ports.AgentResourceNodeRepository
	SessionRepo  ports.AgentSessionRepository
	World        ports.WorldProvider
	Metrics      ports.ActionMetrics
	Settle       survival.SettlementService
	Now          func() time.Time
}

// 主流程入口（固定骨架）
func (e *ActionEngine) Execute(ctx context.Context, req Request) (Response, error) {
	// 1) validateRequest
	// 2) loadOrReplayIdempotent
	// 3) loadStateAndFinalizeOngoing
	// 4) resolveSpec
	// 5) spec.Handler.BuildContext
	// 6) spec.Handler.Precheck
	// 7) spec.Handler.ExecuteActionAndPlan (可能 Completed)
	// 8) PersistAndRespond (统一持久化后返回 Completed/Settled response)
	panic("skeleton")
}
```
