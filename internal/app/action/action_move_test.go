package action

import (
	"context"
	"errors"
	"testing"
	"time"

	worldmock "clawvival/internal/adapter/world/mock"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_RejectsMoveWhenTargetTileBlocked(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: false},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-blocked",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Direction: "E"}})
	if !errors.Is(err, ErrActionInvalidPosition) {
		t.Fatalf("expected ErrActionInvalidPosition, got %v", err)
	}
	var posErr *ActionInvalidPositionError
	if !errors.As(err, &posErr) || posErr == nil {
		t.Fatalf("expected ActionInvalidPositionError details, got %T", err)
	}
	if posErr.TargetPos == nil || posErr.TargetPos.X != 1 || posErr.TargetPos.Y != 0 {
		t.Fatalf("unexpected target_pos details: %+v", posErr.TargetPos)
	}
	if posErr.BlockingTilePos == nil || posErr.BlockingTilePos.X != 1 || posErr.BlockingTilePos.Y != 0 {
		t.Fatalf("unexpected blocking_tile_pos details: %+v", posErr.BlockingTilePos)
	}
}

func TestUseCase_MoveToPosition_SucceedsWithMultiStepPath(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
				{X: 1, Y: 0, Passable: true},
				{X: 2, Y: 0, Passable: true},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-pos-ok",
		Intent: survival.ActionIntent{
			Type: survival.ActionMove,
			Pos:  &survival.Position{X: 2, Y: 0},
		},
	})
	if err != nil {
		t.Fatalf("expected move-to-position success, got %v", err)
	}
	if got, want := out.UpdatedState.Position.X, 2; got != want {
		t.Fatalf("expected moved to x=%d, got=%d", want, got)
	}
	if got, want := out.UpdatedState.Position.Y, 0; got != want {
		t.Fatalf("expected moved to y=%d, got=%d", want, got)
	}
}

func TestUseCase_MoveToPosition_RejectsWhenTargetNotPassable(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Version: 1},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}

	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:   "day",
			ThreatLevel: 1,
			VisibleTiles: []world.Tile{
				{X: 0, Y: 0, Passable: true},
				{X: 1, Y: 0, Passable: false},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	_, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-move-pos-blocked",
		Intent: survival.ActionIntent{
			Type: survival.ActionMove,
			Pos:  &survival.Position{X: 1, Y: 0},
		},
	})
	if !errors.Is(err, ErrActionInvalidPosition) {
		t.Fatalf("expected ErrActionInvalidPosition, got %v", err)
	}
	var posErr *ActionInvalidPositionError
	if !errors.As(err, &posErr) || posErr == nil {
		t.Fatalf("expected ActionInvalidPositionError details, got %T", err)
	}
	if posErr.TargetPos == nil || posErr.TargetPos.X != 1 || posErr.TargetPos.Y != 0 {
		t.Fatalf("unexpected target_pos details: %+v", posErr.TargetPos)
	}
	if posErr.BlockingTilePos == nil || posErr.BlockingTilePos.X != 1 || posErr.BlockingTilePos.Y != 0 {
		t.Fatalf("unexpected blocking_tile_pos details: %+v", posErr.BlockingTilePos)
	}
}
