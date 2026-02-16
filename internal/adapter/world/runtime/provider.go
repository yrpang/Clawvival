package runtime

import (
	"context"
	"math"
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
	ViewRadius     int
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
		ViewRadius:     3,
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
	if cfg.ViewRadius <= 0 {
		cfg.ViewRadius = def.ViewRadius
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return Provider{cfg: cfg}
}

func (p Provider) SnapshotForAgent(_ context.Context, _ string, center world.Point) (world.Snapshot, error) {
	hour := p.cfg.Now().Hour()
	isDay := hour >= p.cfg.DayStartHour && hour < p.cfg.NightStart
	timeOfDay := "night"
	threat := p.cfg.ThreatNight
	nearby := copyMap(p.cfg.ResourcesNight)
	if isDay {
		timeOfDay = "day"
		threat = p.cfg.ThreatDay
		nearby = copyMap(p.cfg.ResourcesDay)
	}

	tiles := make([]world.Tile, 0, (p.cfg.ViewRadius*2+1)*(p.cfg.ViewRadius*2+1))
	counts := map[string]int{}
	for y := center.Y - p.cfg.ViewRadius; y <= center.Y+p.cfg.ViewRadius; y++ {
		for x := center.X - p.cfg.ViewRadius; x <= center.X+p.cfg.ViewRadius; x++ {
			t := genTile(x, y)
			if !isDay {
				t.BaseThreat++
			}
			tiles = append(tiles, t)
			if t.Resource != "" {
				counts[t.Resource]++
			}
		}
	}
	if len(counts) > 0 {
		nearby = counts
	}

	return world.Snapshot{
		TimeOfDay:      timeOfDay,
		ThreatLevel:    threat,
		NearbyResource: nearby,
		Center:         center,
		ViewRadius:     p.cfg.ViewRadius,
		VisibleTiles:   tiles,
	}, nil
}

func genTile(x, y int) world.Tile {
	z := zoneByDistance(x, y)
	b := biomeByZone(z)
	seed := tileSeed(x, y)
	kind := world.TileGrass
	passable := true
	resource := ""
	baseThreat := zoneBaseThreat(z)

	switch z {
	case world.ZoneSafe:
		kind = world.TileGrass
	case world.ZoneForest:
		if seed%5 == 0 {
			kind = world.TileTree
			resource = "wood"
			passable = false
		} else {
			kind = world.TileGrass
		}
	case world.ZoneQuarry:
		if seed%4 == 0 {
			kind = world.TileRock
			resource = "stone"
			passable = false
		} else {
			kind = world.TileDirt
		}
	case world.ZoneWild:
		if seed%7 == 0 {
			kind = world.TileWater
			passable = false
		} else if seed%3 == 0 {
			kind = world.TileTree
			resource = "wood"
			passable = false
		} else {
			kind = world.TileDirt
		}
	}

	return world.Tile{
		X:          x,
		Y:          y,
		Kind:       kind,
		Zone:       z,
		Biome:      b,
		Passable:   passable,
		Resource:   resource,
		BaseThreat: baseThreat,
	}
}

func zoneByDistance(x, y int) world.Zone {
	d := int(math.Abs(float64(x)) + math.Abs(float64(y)))
	switch {
	case d <= 6:
		return world.ZoneSafe
	case d <= 20:
		return world.ZoneForest
	case d <= 35:
		return world.ZoneQuarry
	default:
		return world.ZoneWild
	}
}

func biomeByZone(z world.Zone) world.Biome {
	switch z {
	case world.ZoneSafe:
		return world.BiomePlain
	case world.ZoneForest:
		return world.BiomeForest
	case world.ZoneQuarry:
		return world.BiomeMountain
	default:
		return world.BiomeWasteland
	}
}

func zoneBaseThreat(z world.Zone) int {
	switch z {
	case world.ZoneSafe:
		return 1
	case world.ZoneForest:
		return 2
	case world.ZoneQuarry:
		return 3
	default:
		return 4
	}
}

func tileSeed(x, y int) int {
	v := x*73856093 ^ y*19349663
	if v < 0 {
		v = -v
	}
	return v
}

func copyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
