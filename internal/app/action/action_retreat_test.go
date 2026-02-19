package action

import (
	"context"
	"testing"
	"time"

	worldmock "clawvival/internal/adapter/world/mock"
	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestUseCase_RetreatAvoidsBlockedTile(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 10,
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 0, Passable: true, BaseThreat: 4},
				{X: -1, Y: 0, Passable: false, BaseThreat: 1},
				{X: 0, Y: 1, Passable: true, BaseThreat: 1},
				{X: 0, Y: -1, Passable: true, BaseThreat: 1},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700006000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-retreat-blocked",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X == -1 && out.UpdatedState.Position.Y == 0 {
		t.Fatalf("retreat selected blocked tile")
	}
}

func TestUseCase_GameOverEventIncludesLastKnownThreatWhenVisible(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{
		"agent-1": {
			AgentID:   "agent-1",
			Vitals:    survival.Vitals{HP: 1, Hunger: -200, Energy: -50},
			Position:  survival.Position{X: 0, Y: 0},
			Home:      survival.Position{X: 0, Y: 0},
			Inventory: map[string]int{},
			Version:   1,
		},
	}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{
		TxManager:  stubTxManager{},
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World: worldmock.Provider{Snapshot: world.Snapshot{
			WorldTimeSeconds: 100,
			TimeOfDay:        "night",
			ThreatLevel:      3,
			VisibleTiles: []world.Tile{
				{X: 1, Y: 1, BaseThreat: 4, Resource: "wood"},
				{X: -1, Y: 0, BaseThreat: 2},
			},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700007000, 0) },
	}
	out, err := uc.Execute(context.Background(), Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-gameover-threat",
		Intent:         survival.ActionIntent{Type: survival.ActionGather, TargetID: "res_1_1_wood"},
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	found := false
	for _, evt := range out.Events {
		if evt.Type != "game_over" {
			continue
		}
		found = true
		last, _ := evt.Payload["last_known_threat"].(map[string]any)
		if last == nil || last["id"] == nil {
			t.Fatalf("expected last_known_threat in game_over payload, got=%v", evt.Payload["last_known_threat"])
		}
	}
	if !found {
		t.Fatalf("expected game_over event")
	}
}
func TestUseCase_RetreatMovesAwayFromHighestThreatTile(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Home: survival.Position{X: 0, Y: 0}, Inventory: map[string]int{}, Version: 1}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{TxManager: stubTxManager{}, StateRepo: stateRepo, ActionRepo: actionRepo, EventRepo: eventRepo, World: worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 10, TimeOfDay: "night", ThreatLevel: 3, VisibleTiles: []world.Tile{{X: 1, Y: 0, Passable: true, BaseThreat: 4}, {X: -1, Y: 0, Passable: true, BaseThreat: 1}, {X: 0, Y: 1, Passable: true, BaseThreat: 2}, {X: 0, Y: -1, Passable: true, BaseThreat: 2}}}}, Settle: survival.SettlementService{}, Now: func() time.Time {
		return time.Unix(1700004000, 0)
	}}
	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", IdempotencyKey: "k-retreat-away-threat", Intent: survival.ActionIntent{Type: survival.ActionRetreat}})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X != -1 || out.UpdatedState.Position.Y != 0 {
		t.Fatalf("expected retreat move west to (-1,0), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}
}
func TestUseCase_RetreatIgnoresCenterThreatAndStillMovesAway(t *testing.T) {
	stateRepo := &stubStateRepo{byAgent: map[string]survival.AgentStateAggregate{"agent-1": {AgentID: "agent-1", Vitals: survival.Vitals{HP: 100, Hunger: 80, Energy: 60}, Position: survival.Position{X: 0, Y: 0}, Home: survival.Position{X: 5, Y: 5}, Inventory: map[string]int{}, Version: 1}}}
	actionRepo := &stubActionRepo{byKey: map[string]ports.ActionExecutionRecord{}}
	eventRepo := &stubEventRepo{}
	uc := UseCase{TxManager: stubTxManager{}, StateRepo: stateRepo, ActionRepo: actionRepo, EventRepo: eventRepo, World: worldmock.Provider{Snapshot: world.Snapshot{WorldTimeSeconds: 10, TimeOfDay: "night", ThreatLevel: 3, VisibleTiles: []world.Tile{{X: 0, Y: 0, Passable: true, BaseThreat: 4}, {X: 1, Y: 0, Passable: true, BaseThreat: 4}, {X: -1, Y: 0, Passable: true, BaseThreat: 1}, {X: 0, Y: 1, Passable: true, BaseThreat: 2}, {X: 0, Y: -1, Passable: true, BaseThreat: 2}}}}, Settle: survival.SettlementService{}, Now: func() time.Time {
		return time.Unix(1700005000, 0)
	}}
	out, err := uc.Execute(context.Background(), Request{AgentID: "agent-1", IdempotencyKey: "k-retreat-center-threat", Intent: survival.ActionIntent{Type: survival.ActionRetreat}})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if out.UpdatedState.Position.X == 0 && out.UpdatedState.Position.Y == 0 {
		t.Fatalf("expected retreat to move away from threat, got no movement")
	}
}
