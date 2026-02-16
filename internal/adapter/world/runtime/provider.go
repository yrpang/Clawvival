package runtime

import (
	"context"
	"time"

	"clawverse/internal/domain/world"
)

type Config struct {
	DayStartHour   int
	NightStart     int
	ThreatDay      int
	ThreatNight    int
	ResourcesDay   map[string]int
	ResourcesNight map[string]int
	Now            func() time.Time
}

type Provider struct {
	cfg Config
}

func DefaultConfig() Config {
	return Config{
		DayStartHour:   6,
		NightStart:     18,
		ThreatDay:      1,
		ThreatNight:    3,
		ResourcesDay:   map[string]int{"wood": 10, "stone": 5},
		ResourcesNight: map[string]int{"wood": 6, "stone": 3},
		Now:            time.Now,
	}
}

func NewProvider(cfg Config) Provider {
	def := DefaultConfig()
	if cfg.DayStartHour < 0 || cfg.DayStartHour > 23 {
		cfg.DayStartHour = def.DayStartHour
	}
	if cfg.NightStart < 0 || cfg.NightStart > 23 {
		cfg.NightStart = def.NightStart
	}
	if cfg.DayStartHour >= cfg.NightStart {
		cfg.DayStartHour = def.DayStartHour
		cfg.NightStart = def.NightStart
	}
	if cfg.ResourcesDay == nil {
		cfg.ResourcesDay = copyMap(def.ResourcesDay)
	}
	if cfg.ResourcesNight == nil {
		cfg.ResourcesNight = copyMap(def.ResourcesNight)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return Provider{cfg: cfg}
}

func (p Provider) SnapshotForAgent(_ context.Context, _ string) (world.Snapshot, error) {
	hour := p.cfg.Now().Hour()
	if hour >= p.cfg.DayStartHour && hour < p.cfg.NightStart {
		return world.Snapshot{
			TimeOfDay:      "day",
			ThreatLevel:    p.cfg.ThreatDay,
			NearbyResource: copyMap(p.cfg.ResourcesDay),
		}, nil
	}
	return world.Snapshot{
		TimeOfDay:      "night",
		ThreatLevel:    p.cfg.ThreatNight,
		NearbyResource: copyMap(p.cfg.ResourcesNight),
	}, nil
}

func copyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
