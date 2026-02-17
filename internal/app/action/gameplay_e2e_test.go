package action

import (
	"context"
	"os"
	"testing"
	"time"

	gormrepo "clawvival/internal/adapter/repo/gorm"
	"clawvival/internal/adapter/repo/gorm/model"
	worldruntime "clawvival/internal/adapter/world/runtime"
	"clawvival/internal/app/observe"
	"clawvival/internal/app/replay"
	"clawvival/internal/app/status"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestGameplayLoop_E2E_ObserveActionStatusReplay(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	agentID := "it-gameplay-loop"
	ctx := context.Background()
	_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	objectRepo := gormrepo.NewWorldObjectRepo(db)
	sessionRepo := gormrepo.NewAgentSessionRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:  agentID,
		Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position: survival.Position{X: 0, Y: 0},
		Home:     survival.Position{X: 0, Y: 0},
		Inventory: map[string]int{
			"seed":  2,
			"wood":  16,
			"stone": 2,
		},
		Version: 1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ChunkStore:      gormrepo.NewWorldChunkRepo(db),
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})

	observeUC := observe.UseCase{StateRepo: stateRepo, World: worldProvider}
	actionUC := UseCase{
		TxManager:   txManager,
		StateRepo:   stateRepo,
		ActionRepo:  actionRepo,
		EventRepo:   eventRepo,
		ObjectRepo:  objectRepo,
		SessionRepo: sessionRepo,
		World:       worldProvider,
		Settle:      survival.SettlementService{},
		Now:         func() time.Time { return now },
	}
	statusUC := status.UseCase{StateRepo: stateRepo, World: worldProvider}
	replayUC := replay.UseCase{Events: eventRepo}

	obs, err := observeUC.Execute(ctx, observe.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if len(obs.Snapshot.VisibleTiles) == 0 {
		t.Fatalf("expected visible tiles from observe")
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-loop",
	}); err != nil {
		t.Fatalf("gather: %v", err)
	}

	now = now.Add(2 * time.Minute)
	moveOut, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-move",
		Intent:         survival.ActionIntent{Type: survival.ActionMove, Direction: "E"}, StrategyHash: "sha-loop",
	})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moveOut.UpdatedState.Position.X != 1 || moveOut.UpdatedState.Position.Y != 0 {
		t.Fatalf("expected move to update position to (1,0), got (%d,%d)", moveOut.UpdatedState.Position.X, moveOut.UpdatedState.Position.Y)
	}
	if moveOut.UpdatedState.Vitals.Energy >= 42 {
		t.Fatalf("expected move to consume energy, got energy=%d", moveOut.UpdatedState.Vitals.Energy)
	}

	now = now.Add(6 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-build",
		Intent:         survival.ActionIntent{Type: survival.ActionBuild, ObjectType: "bed_rough", Pos: &survival.Position{X: 1, Y: 0}}, StrategyHash: "sha-loop",
	}); err != nil {
		t.Fatalf("build: %v", err)
	}

	now = now.Add(6 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-build-farm",
		Intent:         survival.ActionIntent{Type: survival.ActionBuild, ObjectType: "farm_plot", Pos: &survival.Position{X: 1, Y: 1}}, StrategyHash: "sha-loop",
	}); err != nil {
		t.Fatalf("build farm: %v", err)
	}

	now = now.Add(4 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-farm-plant",
		Intent:         survival.ActionIntent{Type: survival.ActionFarmPlant, FarmID: "obj-it-gameplay-loop-loop-build-farm"}, StrategyHash: "sha-loop",
	}); err != nil {
		t.Fatalf("farm plant: %v", err)
	}

	now = now.Add(11 * time.Minute)
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "loop-retreat",
		Intent:         survival.ActionIntent{Type: survival.ActionRetreat}, StrategyHash: "sha-loop",
	}); err != nil {
		t.Fatalf("retreat: %v", err)
	}

	st, err := statusUC.Execute(ctx, status.Request{AgentID: agentID})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.State.Version <= 1 {
		t.Fatalf("expected state version to advance, got=%d", st.State.Version)
	}
	if st.TimeOfDay == "" {
		t.Fatalf("expected status time of day")
	}

	rep, err := replayUC.Execute(ctx, replay.Request{AgentID: agentID, Limit: 100})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(rep.Events) == 0 {
		t.Fatalf("expected replay events")
	}
	if rep.LatestState.AgentID != agentID {
		t.Fatalf("latest state agent mismatch: got=%s", rep.LatestState.AgentID)
	}

	var objs []model.WorldObject
	if err := db.Where("owner_agent_id = ?", agentID).Find(&objs).Error; err != nil {
		t.Fatalf("query world_objects: %v", err)
	}
	if len(objs) == 0 {
		t.Fatalf("expected built world objects from gameplay loop")
	}
}

func TestGameplayLoop_E2E_AgentIsolationAndSharedWorldClock(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentA := "it-isolation-a"
	agentB := "it-isolation-b"
	for _, agentID := range []string{agentA, agentB} {
		_ = db.Exec("DELETE FROM world_objects WHERE owner_agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	}
	_ = db.Exec("DELETE FROM world_clock_state").Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seedA := survival.AgentStateAggregate{
		AgentID:    agentA,
		Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:   survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{"wood": 1},
		Dead:       false,
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	seedB := survival.AgentStateAggregate{
		AgentID:    agentB,
		Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Position:   survival.Position{X: 5, Y: 5},
		Inventory:  map[string]int{"wood": 1},
		Dead:       false,
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seedA, 0); err != nil {
		t.Fatalf("seed state A: %v", err)
	}
	if err := stateRepo.SaveWithVersion(ctx, seedB, 0); err != nil {
		t.Fatalf("seed state B: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ChunkStore:      gormrepo.NewWorldChunkRepo(db),
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})
	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}
	statusUC := status.UseCase{StateRepo: stateRepo, World: worldProvider}

	// world clock is global: first call persists day phase into shared row.
	stA1, err := statusUC.Execute(ctx, status.Request{AgentID: agentA})
	if err != nil {
		t.Fatalf("status A first: %v", err)
	}
	stB1, err := statusUC.Execute(ctx, status.Request{AgentID: agentB})
	if err != nil {
		t.Fatalf("status B first: %v", err)
	}
	if stA1.TimeOfDay != stB1.TimeOfDay {
		t.Fatalf("expected shared world phase, got A=%s B=%s", stA1.TimeOfDay, stB1.TimeOfDay)
	}

	// state mutation is isolated by agent_id.
	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentA,
		IdempotencyKey: "isolation-a-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-isolation",
	}); err != nil {
		t.Fatalf("action A gather: %v", err)
	}

	afterA, err := stateRepo.GetByAgentID(ctx, agentA)
	if err != nil {
		t.Fatalf("load state A: %v", err)
	}
	afterB, err := stateRepo.GetByAgentID(ctx, agentB)
	if err != nil {
		t.Fatalf("load state B: %v", err)
	}
	if afterA.Version <= seedA.Version {
		t.Fatalf("expected A version to advance, before=%d after=%d", seedA.Version, afterA.Version)
	}
	if afterB.Version != seedB.Version {
		t.Fatalf("expected B version unchanged, before=%d after=%d", seedB.Version, afterB.Version)
	}

	now = now.Add(11 * time.Minute)
	stA2, err := statusUC.Execute(ctx, status.Request{AgentID: agentA})
	if err != nil {
		t.Fatalf("status A second: %v", err)
	}
	stB2, err := statusUC.Execute(ctx, status.Request{AgentID: agentB})
	if err != nil {
		t.Fatalf("status B second: %v", err)
	}
	if stA2.TimeOfDay != "night" || stB2.TimeOfDay != "night" {
		t.Fatalf("expected both agents in night phase, got A=%s B=%s", stA2.TimeOfDay, stB2.TimeOfDay)
	}

	var clockRows []model.WorldClockState
	if err := db.Find(&clockRows).Error; err != nil {
		t.Fatalf("query world_clock_state: %v", err)
	}
	if len(clockRows) != 1 {
		t.Fatalf("expected single global clock row, got=%d", len(clockRows))
	}
	if clockRows[0].StateKey != "global" {
		t.Fatalf("expected global clock state key, got=%s", clockRows[0].StateKey)
	}
}

func TestGameplayLoop_E2E_ContinuousDeltaScaling(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agent30 := "it-dt-scale-30"
	agent60 := "it-dt-scale-60"
	for _, agentID := range []string{agent30, agent60} {
		_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		Vitals:     survival.Vitals{HP: 100, Hunger: 100, Energy: 100},
		Position:   survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{},
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	s30 := seed
	s30.AgentID = agent30
	s60 := seed
	s60.AgentID = agent60
	if err := stateRepo.SaveWithVersion(ctx, s30, 0); err != nil {
		t.Fatalf("seed 30 state: %v", err)
	}
	if err := stateRepo.SaveWithVersion(ctx, s60, 0); err != nil {
		t.Fatalf("seed 60 state: %v", err)
	}

	now := time.Unix(0, 0)
	if err := eventRepo.Append(ctx, agent30, []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: now.Add(-30 * time.Minute)},
	}); err != nil {
		t.Fatalf("seed event 30: %v", err)
	}
	if err := eventRepo.Append(ctx, agent60, []survival.DomainEvent{
		{Type: "action_settled", OccurredAt: now.Add(-60 * time.Minute)},
	}); err != nil {
		t.Fatalf("seed event 60: %v", err)
	}

	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:        func() time.Time { return now },
		ViewRadius: 2,
	})

	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	out30, err := actionUC.Execute(ctx, Request{
		AgentID:        agent30,
		IdempotencyKey: "dt-30-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, // ignored: server derives dt from event timeline
		StrategyHash:   "sha-dt",
	})
	if err != nil {
		t.Fatalf("action 30: %v", err)
	}
	out60, err := actionUC.Execute(ctx, Request{
		AgentID:        agent60,
		IdempotencyKey: "dt-60-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, // ignored: server derives dt from event timeline
		StrategyHash:   "sha-dt",
	})
	if err != nil {
		t.Fatalf("action 60: %v", err)
	}

	dropHunger30 := s30.Vitals.Hunger - out30.UpdatedState.Vitals.Hunger
	dropHunger60 := s60.Vitals.Hunger - out60.UpdatedState.Vitals.Hunger
	dropEnergy30 := s30.Vitals.Energy - out30.UpdatedState.Vitals.Energy
	dropEnergy60 := s60.Vitals.Energy - out60.UpdatedState.Vitals.Energy

	if dropHunger60 != dropHunger30*2 {
		t.Fatalf("expected hunger drop scale by dt, 30=%d 60=%d", dropHunger30, dropHunger60)
	}
	if dropEnergy60 != dropEnergy30*2 {
		t.Fatalf("expected energy drop scale by dt, 30=%d 60=%d", dropEnergy30, dropEnergy60)
	}
}

func TestGameplayLoop_E2E_DayNightNonCombatHPLossConsistent(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	dayAgent := "it-risk-day"
	nightAgent := "it-risk-night"
	for _, agentID := range []string{dayAgent, nightAgent} {
		_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
		_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		Vitals:     survival.Vitals{HP: 100, Hunger: 80, Energy: 80},
		Position:   survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{},
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	seedDay := seed
	seedDay.AgentID = dayAgent
	seedNight := seed
	seedNight.AgentID = nightAgent
	if err := stateRepo.SaveWithVersion(ctx, seedDay, 0); err != nil {
		t.Fatalf("seed day state: %v", err)
	}
	if err := stateRepo.SaveWithVersion(ctx, seedNight, 0); err != nil {
		t.Fatalf("seed night state: %v", err)
	}

	worldProviderDay := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:         func() time.Time { return time.Unix(0, 0) },
		ViewRadius:  2,
		ThreatDay:   1,
		ThreatNight: 3,
	})
	worldProviderNight := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:         func() time.Time { return time.Unix(0, 0).Add(11 * time.Minute) },
		ViewRadius:  2,
		ThreatDay:   1,
		ThreatNight: 3,
	})
	actionUCDay := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProviderDay,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(0, 0) },
	}
	actionUCNight := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProviderNight,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return time.Unix(0, 0).Add(11 * time.Minute) },
	}
	statusDay := status.UseCase{StateRepo: stateRepo, World: worldProviderDay}
	statusNight := status.UseCase{StateRepo: stateRepo, World: worldProviderNight}

	stDay, err := statusDay.Execute(ctx, status.Request{AgentID: dayAgent})
	if err != nil {
		t.Fatalf("status day: %v", err)
	}
	if stDay.TimeOfDay != "day" {
		t.Fatalf("expected day phase, got %s", stDay.TimeOfDay)
	}
	stNight, err := statusNight.Execute(ctx, status.Request{AgentID: nightAgent})
	if err != nil {
		t.Fatalf("status night: %v", err)
	}
	if stNight.TimeOfDay != "night" {
		t.Fatalf("expected night phase, got %s", stNight.TimeOfDay)
	}

	dayOut, err := actionUCDay.Execute(ctx, Request{
		AgentID:        dayAgent,
		IdempotencyKey: "gather-day",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-risk",
	})
	if err != nil {
		t.Fatalf("day gather: %v", err)
	}

	nightOut, err := actionUCNight.Execute(ctx, Request{
		AgentID:        nightAgent,
		IdempotencyKey: "gather-night",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-risk",
	})
	if err != nil {
		t.Fatalf("night gather: %v", err)
	}

	dayLoss := seedDay.Vitals.HP - dayOut.UpdatedState.Vitals.HP
	nightLoss := seedNight.Vitals.HP - nightOut.UpdatedState.Vitals.HP
	if nightLoss != dayLoss {
		t.Fatalf("expected day/night non-combat hp loss equal, day=%d night=%d", dayLoss, nightLoss)
	}
}

func TestGameplayLoop_E2E_StarvationTriggersGameOver(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentID := "it-starvation-gameover"
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:    agentID,
		Vitals:     survival.Vitals{HP: 5, Hunger: -300, Energy: 10},
		Position:   survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{},
		DeathCause: survival.DeathCauseUnknown,
		Version:    1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:        func() time.Time { return now },
		ViewRadius: 2,
	})
	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	out, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "starvation-rest",
		Intent:         survival.ActionIntent{Type: survival.ActionGather},
		StrategyHash:   "sha-starve",
	})
	if err != nil {
		t.Fatalf("gather action: %v", err)
	}
	if out.ResultCode != survival.ResultGameOver {
		t.Fatalf("expected game over, got %s", out.ResultCode)
	}
	if !out.UpdatedState.Dead {
		t.Fatalf("expected dead state")
	}
	if out.UpdatedState.DeathCause != survival.DeathCauseStarvation {
		t.Fatalf("expected starvation cause, got %s", out.UpdatedState.DeathCause)
	}

	foundGameOver := false
	for _, evt := range out.Events {
		if evt.Type == "game_over" {
			foundGameOver = true
			break
		}
	}
	if !foundGameOver {
		t.Fatalf("expected game_over event")
	}
}

func TestGameplayLoop_E2E_CriticalHPForcesRetreat(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentID := "it-critical-retreat"
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:  agentID,
		Vitals:   survival.Vitals{HP: 22, Hunger: -120, Energy: 10},
		Position: survival.Position{X: 5, Y: 5},
		Home:     survival.Position{X: 0, Y: 0},
		Version:  1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0).Add(11 * time.Minute)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ThreatDay:       1,
		ThreatNight:     3,
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})
	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	out, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "critical-gather",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-critical",
	})
	if err != nil {
		t.Fatalf("gather action: %v", err)
	}

	if out.ResultCode != survival.ResultOK {
		t.Fatalf("expected result ok, got %s", out.ResultCode)
	}
	if out.UpdatedState.Vitals.HP <= 0 || out.UpdatedState.Vitals.HP > 20 {
		t.Fatalf("expected HP in critical range (1-20), got %d", out.UpdatedState.Vitals.HP)
	}
	if out.UpdatedState.Position.X != 4 || out.UpdatedState.Position.Y != 4 {
		t.Fatalf("expected forced retreat to (4,4), got (%d,%d)", out.UpdatedState.Position.X, out.UpdatedState.Position.Y)
	}

	foundCritical := false
	foundForceRetreat := false
	for _, evt := range out.Events {
		if evt.Type == "critical_hp" {
			foundCritical = true
		}
		if evt.Type == "force_retreat" {
			foundForceRetreat = true
		}
	}
	if !foundCritical || !foundForceRetreat {
		t.Fatalf("expected critical_hp and force_retreat events, critical=%v force_retreat=%v", foundCritical, foundForceRetreat)
	}
}

func TestGameplayLoop_E2E_WorldPhaseChangedEvent(t *testing.T) {
	dsn := os.Getenv("CLAWVIVAL_DB_DSN")
	if dsn == "" {
		t.Skip("CLAWVIVAL_DB_DSN is required for integration test")
	}

	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	agentID := "it-phase-changed-event"
	_ = db.Exec("DELETE FROM action_executions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM domain_events WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_sessions WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM agent_states WHERE agent_id = ?", agentID).Error
	_ = db.Exec("DELETE FROM world_clock_state").Error

	stateRepo := gormrepo.NewAgentStateRepo(db)
	actionRepo := gormrepo.NewActionExecutionRepo(db)
	eventRepo := gormrepo.NewEventRepo(db)
	txManager := gormrepo.NewTxManager(db)

	seed := survival.AgentStateAggregate{
		AgentID:  agentID,
		Vitals:   survival.Vitals{HP: 100, Hunger: 80, Energy: 80},
		Position: survival.Position{X: 0, Y: 0},
		Version:  1,
	}
	if err := stateRepo.SaveWithVersion(ctx, seed, 0); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	now := time.Unix(0, 0)
	worldProvider := worldruntime.NewProvider(worldruntime.Config{
		Clock: world.NewClock(world.ClockConfig{
			StartAt:       time.Unix(0, 0),
			DayDuration:   10 * time.Minute,
			NightDuration: 5 * time.Minute,
		}),
		Now:             func() time.Time { return now },
		ViewRadius:      2,
		ClockStateStore: gormrepo.NewWorldClockStateRepo(db),
	})
	actionUC := UseCase{
		TxManager:  txManager,
		StateRepo:  stateRepo,
		ActionRepo: actionRepo,
		EventRepo:  eventRepo,
		World:      worldProvider,
		Settle:     survival.SettlementService{},
		Now:        func() time.Time { return now },
	}

	if _, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "phase-day-prime",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-phase",
	}); err != nil {
		t.Fatalf("prime action: %v", err)
	}

	now = now.Add(11 * time.Minute)
	out, err := actionUC.Execute(ctx, Request{
		AgentID:        agentID,
		IdempotencyKey: "phase-night-switch",
		Intent:         survival.ActionIntent{Type: survival.ActionGather}, StrategyHash: "sha-phase",
	})
	if err != nil {
		t.Fatalf("switch action: %v", err)
	}

	foundPhaseChanged := false
	for _, evt := range out.Events {
		if evt.Type != "world_phase_changed" {
			continue
		}
		foundPhaseChanged = true
		from, _ := evt.Payload["from"].(string)
		to, _ := evt.Payload["to"].(string)
		if from != "day" || to != "night" {
			t.Fatalf("expected day->night phase payload, got from=%s to=%s", from, to)
		}
	}
	if !foundPhaseChanged {
		t.Fatalf("expected world_phase_changed event")
	}
}
