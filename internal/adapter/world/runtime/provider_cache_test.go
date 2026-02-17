package runtime

import (
	"context"
	"testing"
	"time"

	"clawvival/internal/domain/world"
)

type fakeChunkStore struct {
	chunks map[string]world.Chunk
	saves  int
}

func (s *fakeChunkStore) GetChunk(_ context.Context, coord world.ChunkCoord, phase string) (world.Chunk, bool, error) {
	if s.chunks == nil {
		s.chunks = map[string]world.Chunk{}
	}
	k := phase + ":" + key(coord)
	c, ok := s.chunks[k]
	return c, ok, nil
}

func (s *fakeChunkStore) SaveChunk(_ context.Context, coord world.ChunkCoord, phase string, chunk world.Chunk) error {
	if s.chunks == nil {
		s.chunks = map[string]world.Chunk{}
	}
	s.saves++
	s.chunks[phase+":"+key(coord)] = chunk
	return nil
}

func key(c world.ChunkCoord) string { return fmtInt(c.X) + "," + fmtInt(c.Y) }
func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	b := [20]byte{}
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestProvider_ReusesCachedChunks(t *testing.T) {
	store := &fakeChunkStore{}
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	p := NewProvider(Config{
		Clock:      world.NewClock(world.ClockConfig{StartAt: now, DayDuration: 10 * time.Minute, NightDuration: 5 * time.Minute}),
		ViewRadius: 2,
		Now:        func() time.Time { return now },
		ChunkStore: store,
	})

	_, err := p.SnapshotForAgent(context.Background(), "a1", world.Point{X: 3, Y: 3})
	if err != nil {
		t.Fatalf("snapshot1 error: %v", err)
	}
	firstSaves := store.saves
	if firstSaves == 0 {
		t.Fatalf("expected chunks to be saved")
	}
	_, err = p.SnapshotForAgent(context.Background(), "a1", world.Point{X: 3, Y: 3})
	if err != nil {
		t.Fatalf("snapshot2 error: %v", err)
	}
	if store.saves != firstSaves {
		t.Fatalf("expected no extra saves on cached read, before=%d after=%d", firstSaves, store.saves)
	}
}
