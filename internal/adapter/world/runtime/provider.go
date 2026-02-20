package runtime

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"clawvival/internal/domain/world"
)

type Config struct {
	Clock           world.Clock
	ThreatDay       int
	ThreatNight     int
	ResourcesDay    map[string]int
	ResourcesNight  map[string]int
	ViewRadius      int
	Now             func() time.Time
	ChunkStore      ChunkStore
	ClockStateStore ClockStateStore
	RefreshInterval time.Duration
}

type Provider struct {
	cfg       Config
	chunkSize int
}

type ChunkStore interface {
	GetChunk(ctx context.Context, coord world.ChunkCoord, phase string) (world.Chunk, bool, error)
	SaveChunk(ctx context.Context, coord world.ChunkCoord, phase string, chunk world.Chunk) error
}

type ClockStateStore interface {
	Get(ctx context.Context) (phase string, switchedAt time.Time, ok bool, err error)
	Save(ctx context.Context, phase string, switchedAt time.Time) error
}

func DefaultConfig() Config {
	return Config{
		Clock:          world.DefaultClock(),
		ThreatDay:      1,
		ThreatNight:    3,
		ResourcesDay:   map[string]int{"wood": 10, "stone": 5},
		ResourcesNight: map[string]int{"wood": 6, "stone": 3},
		ViewRadius:     5,
		Now:            time.Now,
	}
}

func NewProvider(cfg Config) Provider {
	def := DefaultConfig()
	if cfg.Clock == (world.Clock{}) {
		cfg.Clock = def.Clock
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
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 5 * time.Minute
	}
	return Provider{cfg: cfg, chunkSize: 8}
}

func (p Provider) SnapshotForAgent(ctx context.Context, _ string, center world.Point) (world.Snapshot, error) {
	nowAt := p.cfg.Now()
	phase, next := p.cfg.Clock.PhaseAt(nowAt)
	isDay := phase == world.PhaseDay
	timeOfDay := "night"
	threat := p.cfg.ThreatNight
	nearby := copyMap(p.cfg.ResourcesNight)
	if isDay {
		timeOfDay = "day"
		threat = p.cfg.ThreatDay
		nearby = copyMap(p.cfg.ResourcesDay)
	}
	phaseChange, err := p.persistPhase(ctx, timeOfDay, nowAt)
	if err != nil {
		return world.Snapshot{}, err
	}

	tiles := make([]world.Tile, 0, (p.cfg.ViewRadius*2+1)*(p.cfg.ViewRadius*2+1))
	counts := map[string]int{}
	refreshBucket := resourceRefreshBucket(nowAt, p.cfg.RefreshInterval)
	chunks, err := p.loadChunksForWindow(ctx, center, timeOfDay)
	if err != nil {
		return world.Snapshot{}, err
	}
	for _, chunk := range chunks {
		for _, t := range chunk.Tiles {
			if t.X < center.X-p.cfg.ViewRadius || t.X > center.X+p.cfg.ViewRadius || t.Y < center.Y-p.cfg.ViewRadius || t.Y > center.Y+p.cfg.ViewRadius {
				continue
			}
			if !isDay {
				t.BaseThreat++
			}
			tiles = append(tiles, t)
			if res := refreshedResource(t.Resource, refreshBucket); res != "" {
				counts[res]++
			}
		}
	}
	if len(counts) > 0 {
		nearby = counts
	}

	return world.Snapshot{
		WorldTimeSeconds:   p.cfg.Clock.WorldTimeSecondsAt(nowAt),
		TimeOfDay:          timeOfDay,
		ThreatLevel:        threat,
		VisibilityPenalty:  visibilityPenalty(isDay),
		NearbyResource:     nearby,
		Center:             center,
		ViewRadius:         p.cfg.ViewRadius,
		VisibleTiles:       tiles,
		NextPhaseInSeconds: int(next.Seconds()),
		PhaseChanged:       phaseChange.changed,
		PhaseFrom:          phaseChange.from,
		PhaseTo:            phaseChange.to,
	}, nil
}

type phaseChange struct {
	changed bool
	from    string
	to      string
}

func (p Provider) persistPhase(ctx context.Context, phase string, now time.Time) (phaseChange, error) {
	if p.cfg.ClockStateStore == nil {
		return phaseChange{}, nil
	}
	current, _, ok, err := p.cfg.ClockStateStore.Get(ctx)
	if err != nil {
		return phaseChange{}, err
	}
	if ok && current == phase {
		return phaseChange{}, nil
	}
	if err := p.cfg.ClockStateStore.Save(ctx, phase, now); err != nil {
		return phaseChange{}, err
	}
	if !ok {
		return phaseChange{}, nil
	}
	return phaseChange{changed: true, from: current, to: phase}, nil
}

func (p Provider) loadChunksForWindow(ctx context.Context, center world.Point, phase string) ([]world.Chunk, error) {
	minX := floorDiv(center.X-p.cfg.ViewRadius, p.chunkSize)
	maxX := floorDiv(center.X+p.cfg.ViewRadius, p.chunkSize)
	minY := floorDiv(center.Y-p.cfg.ViewRadius, p.chunkSize)
	maxY := floorDiv(center.Y+p.cfg.ViewRadius, p.chunkSize)

	out := make([]world.Chunk, 0, (maxX-minX+1)*(maxY-minY+1))
	for cy := minY; cy <= maxY; cy++ {
		for cx := minX; cx <= maxX; cx++ {
			coord := world.ChunkCoord{X: cx, Y: cy}
			if p.cfg.ChunkStore != nil {
				if cached, ok, err := p.cfg.ChunkStore.GetChunk(ctx, coord, phase); err != nil {
					return nil, err
				} else if ok {
					out = append(out, cached)
					continue
				}
			}
			chunk := p.generateChunk(coord)
			if p.cfg.ChunkStore != nil {
				if err := p.cfg.ChunkStore.SaveChunk(ctx, coord, phase, chunk); err != nil {
					return nil, err
				}
			}
			out = append(out, chunk)
		}
	}
	return out, nil
}

func (p Provider) generateChunk(coord world.ChunkCoord) world.Chunk {
	tiles := make([]world.Tile, 0, p.chunkSize*p.chunkSize)
	baseX := coord.X * p.chunkSize
	baseY := coord.Y * p.chunkSize
	for y := 0; y < p.chunkSize; y++ {
		for x := 0; x < p.chunkSize; x++ {
			tiles = append(tiles, genTile(baseX+x, baseY+y))
		}
	}
	return world.Chunk{Coord: coord, Tiles: tiles}
}

func floorDiv(a, b int) int {
	if a >= 0 {
		return a / b
	}
	return -(((-a) + b - 1) / b)
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
		} else if seed%11 == 0 {
			kind = world.TileGrass
			resource = "berry"
			passable = true
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

func visibilityPenalty(isDay bool) int {
	if isDay {
		return 0
	}
	return 1
}

func resourceRefreshBucket(now time.Time, interval time.Duration) int64 {
	if interval <= 0 {
		return 0
	}
	return now.Unix() / int64(interval.Seconds())
}

func refreshedResource(resource string, bucket int64) string {
	switch resource {
	case "wood":
		if bucket%2 == 1 {
			return ""
		}
	case "stone":
		if bucket%2 == 0 {
			return ""
		}
	}
	return resource
}

func marshalChunkTiles(tiles []world.Tile) ([]byte, error) {
	return json.Marshal(tiles)
}

func unmarshalChunkTiles(data []byte) ([]world.Tile, error) {
	out := []world.Tile{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
