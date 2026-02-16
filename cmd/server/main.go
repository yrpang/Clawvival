package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	httpadapter "clawverse/internal/adapter/http"
	gormrepo "clawverse/internal/adapter/repo/gorm"
	staticskills "clawverse/internal/adapter/skills/static"
	worldmock "clawverse/internal/adapter/world/mock"
	"clawverse/internal/app/action"
	"clawverse/internal/app/observe"
	"clawverse/internal/app/ports"
	"clawverse/internal/app/skills"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"
	"clawverse/internal/domain/world"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	stateRepo, actionRepo, eventRepo, txManager := mustBuildRepos()
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

func mustBuildRepos() (ports.AgentStateRepository, ports.ActionExecutionRepository, ports.EventRepository, ports.TxManager) {
	dsn := os.Getenv("CLAWVERSE_DB_DSN")
	if dsn == "" {
		log.Fatal("CLAWVERSE_DB_DSN is required")
	}
	db, err := gormrepo.OpenPostgres(dsn)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}

	stateRepo := gormrepo.NewAgentStateRepo(db)
	_, err = stateRepo.GetByAgentID(context.Background(), "demo-agent")
	if err != nil && errors.Is(err, ports.ErrNotFound) {
		seed := survival.AgentStateAggregate{
			AgentID: "demo-agent",
			Vitals:  survival.Vitals{HP: 100, Hunger: 80, Energy: 60},
			Version: 1,
		}
		if saveErr := stateRepo.SaveWithVersion(context.Background(), seed, 0); saveErr != nil {
			log.Fatalf("seed demo agent: %v (did you run SQL migrations manually?)", saveErr)
		}
	} else if err != nil {
		log.Fatalf("load demo agent: %v (did you run SQL migrations manually?)", err)
	}

	return stateRepo, gormrepo.NewActionExecutionRepo(db), gormrepo.NewEventRepo(db), gormrepo.NewTxManager(db)
}
