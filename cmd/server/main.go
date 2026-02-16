package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	httpadapter "clawverse/internal/adapter/http"
	metricsinmem "clawverse/internal/adapter/metrics/inmemory"
	gormrepo "clawverse/internal/adapter/repo/gorm"
	staticskills "clawverse/internal/adapter/skills/static"
	worldruntime "clawverse/internal/adapter/world/runtime"
	"clawverse/internal/app/action"
	"clawverse/internal/app/observe"
	"clawverse/internal/app/ports"
	"clawverse/internal/app/skills"
	"clawverse/internal/app/status"
	"clawverse/internal/domain/survival"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	stateRepo, actionRepo, eventRepo, txManager := mustBuildRepos()
	worldProvider := buildWorldProviderFromEnv()
	skillsProvider := staticskills.Provider{Root: "./skills"}
	kpiRecorder := metricsinmem.NewRecorder()

	h := httpadapter.Handler{
		ObserveUC: observe.UseCase{StateRepo: stateRepo, World: worldProvider},
		ActionUC: action.UseCase{
			TxManager:  txManager,
			StateRepo:  stateRepo,
			ActionRepo: actionRepo,
			EventRepo:  eventRepo,
			World:      worldProvider,
			Metrics:    kpiRecorder,
			Settle:     survival.SettlementService{},
			Now:        time.Now,
		},
		StatusUC: status.UseCase{StateRepo: stateRepo},
		SkillsUC: skills.UseCase{Provider: skillsProvider},
		KPI:      kpiRecorder,
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

func buildWorldProviderFromEnv() ports.WorldProvider {
	cfg := worldruntime.DefaultConfig()
	cfg.DayStartHour = intEnv("WORLD_DAY_START_HOUR", cfg.DayStartHour)
	cfg.NightStart = intEnv("WORLD_NIGHT_START_HOUR", cfg.NightStart)
	cfg.ThreatDay = intEnv("WORLD_THREAT_DAY", cfg.ThreatDay)
	cfg.ThreatNight = intEnv("WORLD_THREAT_NIGHT", cfg.ThreatNight)

	if resources := resourcesEnv("WORLD_RESOURCES_DAY"); len(resources) > 0 {
		cfg.ResourcesDay = resources
	}
	if resources := resourcesEnv("WORLD_RESOURCES_NIGHT"); len(resources) > 0 {
		cfg.ResourcesNight = resources
	}

	return worldruntime.NewProvider(cfg)
}

func intEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func resourcesEnv(key string) map[string]int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	out := map[string]int{}
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			continue
		}
		out[name] = n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
