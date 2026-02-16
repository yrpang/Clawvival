package main

import (
	"log"
	"time"

	httpadapter "clawverse/internal/adapter/http"
	"clawverse/internal/adapter/repo/memory"
	staticskills "clawverse/internal/adapter/skills/static"
	worldmock "clawverse/internal/adapter/world/mock"
	"clawverse/internal/app/action"
	"clawverse/internal/app/observe"
	"clawverse/internal/app/skills"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	store := memory.NewStore()
	store.SeedState(survival.AgentStateAggregate{
		AgentID: "demo-agent",
		Vitals:  survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
		Version: 1,
	})

	stateRepo := memory.NewAgentStateRepo(store)
	actionRepo := memory.NewActionExecutionRepo(store)
	eventRepo := memory.NewEventRepo(store)
	txManager := memory.NewTxManager(store)
	worldProvider := worldmock.Provider{Snapshot: world.Snapshot{
		TimeOfDay:      "day",
		ThreatLevel:    1,
		NearbyResource: map[string]int{"wood": 10, "stone": 5},
	}}
	skillsProvider := staticskills.Provider{Root: "./skills"}

	h := httpadapter.Handler{
		ObserveUC: observe.UseCase{StateRepo: stateRepo, World: worldProvider},
		ActionUC: action.UseCase{
			TxManager:  txManager,
			StateRepo:  stateRepo,
			ActionRepo: actionRepo,
			EventRepo:  eventRepo,
			World:      worldProvider,
			Settle:     survival.SettlementService{},
			Now:        time.Now,
		},
		StatusUC: status.UseCase{StateRepo: stateRepo},
		SkillsUC: skills.UseCase{Provider: skillsProvider},
	}

	s := server.Default(server.WithHostPorts(":8080"))
	h.RegisterRoutes(s)

	log.Println("clawverse server listening on :8080 (demo agent: demo-agent)")
	s.Spin()
}
