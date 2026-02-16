package action

import (
	"context"
	"testing"
	"time"

	"clawverse/internal/adapter/repo/memory"
	worldmock "clawverse/internal/adapter/world/mock"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"
)

func TestUseCase_Idempotency(t *testing.T) {
	store := memory.NewStore()
	store.SeedState(survival.AgentStateAggregate{
		AgentID: "agent-1",
		Vitals:  survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Version: 1,
	})

	uc := UseCase{
		TxManager:  memory.NewTxManager(store),
		StateRepo:  memory.NewAgentStateRepo(store),
		ActionRepo: memory.NewActionExecutionRepo(store),
		EventRepo:  memory.NewEventRepo(store),
		World: worldmock.Provider{Snapshot: world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    1,
			NearbyResource: map[string]int{"wood": 10},
		}},
		Settle: survival.SettlementService{},
		Now:    func() time.Time { return time.Unix(1700000000, 0) },
	}

	req := Request{
		AgentID:        "agent-1",
		IdempotencyKey: "k-1",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		DeltaMinutes:   30,
	}

	first, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	second, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}

	if first.UpdatedState.Version != second.UpdatedState.Version {
		t.Fatalf("idempotency broken: version mismatch first=%d second=%d", first.UpdatedState.Version, second.UpdatedState.Version)
	}
}
