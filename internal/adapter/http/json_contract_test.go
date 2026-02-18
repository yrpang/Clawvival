package httpadapter

import (
	"encoding/json"
	"testing"
	"time"

	"clawvival/internal/app/action"
	"clawvival/internal/app/observe"
	"clawvival/internal/app/replay"
	"clawvival/internal/app/status"
	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

func TestResponseJSONUsesSnakeCase(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	state := survival.AgentStateAggregate{
		AgentID: "a1",
		Vitals: survival.Vitals{
			HP:     90,
			Hunger: 40,
			Energy: 70,
		},
		Position:   survival.Position{X: 1, Y: 2},
		Home:       survival.Position{X: 0, Y: 0},
		Inventory:  map[string]int{"wood": 2},
		Dead:       false,
		DeathCause: survival.DeathCauseUnknown,
		Version:    3,
		UpdatedAt:  now,
	}
	event := survival.DomainEvent{
		Type:       "test_event",
		OccurredAt: now,
		Payload:    map[string]any{"ok": true},
	}
	snapshot := world.Snapshot{
		TimeOfDay:          "day",
		ThreatLevel:        1,
		VisibilityPenalty:  0,
		NearbyResource:     map[string]int{"wood": 3},
		Center:             world.Point{X: 1, Y: 2},
		ViewRadius:         3,
		VisibleTiles:       []world.Tile{{X: 1, Y: 2, Kind: world.TileGrass, Zone: world.ZoneSafe, Biome: world.BiomePlain, Passable: true, BaseThreat: 1}},
		NextPhaseInSeconds: 60,
		PhaseChanged:       false,
		PhaseFrom:          "day",
		PhaseTo:            "night",
	}

	cases := []struct {
		name    string
		payload any
		want    []string
		notWant []string
	}{
		{
			name:    "observe",
			payload: observe.Response{State: state, Snapshot: snapshot},
			want:    []string{"agent_state", "snapshot", "world_time_seconds", "time_of_day", "next_phase_in_seconds"},
			notWant: []string{"State", "Snapshot", "state"},
		},
		{
			name:    "action",
			payload: action.Response{UpdatedState: state, Events: []survival.DomainEvent{event}, ResultCode: survival.ResultOK},
			want:    []string{"updated_state", "events", "result_code"},
			notWant: []string{"UpdatedState", "Events", "ResultCode"},
		},
		{
			name:    "status",
			payload: status.Response{State: state, TimeOfDay: "day", NextPhaseInSeconds: 60},
			want:    []string{"agent_state", "time_of_day", "next_phase_in_seconds"},
			notWant: []string{"State", "TimeOfDay", "NextPhaseInSeconds", "state"},
		},
		{
			name:    "replay",
			payload: replay.Response{Events: []survival.DomainEvent{event}, LatestState: state},
			want:    []string{"events", "latest_state"},
			notWant: []string{"Events", "LatestState"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			for _, key := range tc.want {
				if _, ok := got[key]; !ok {
					t.Fatalf("expected key %q in %s", key, string(b))
				}
			}
			for _, key := range tc.notWant {
				if _, ok := got[key]; ok {
					t.Fatalf("unexpected key %q in %s", key, string(b))
				}
			}
			if tc.name == "observe" {
				stateMap := asMap(got["agent_state"])
				if _, ok := stateMap["agent_id"]; !ok {
					t.Fatalf("expected nested snake_case key agent_state.agent_id in %s", string(b))
				}
				if _, ok := stateMap["AgentID"]; ok {
					t.Fatalf("unexpected nested key agent_state.AgentID in %s", string(b))
				}
				snapshotMap := asMap(got["snapshot"])
				if _, ok := snapshotMap["time_of_day"]; !ok {
					t.Fatalf("expected nested snake_case key snapshot.time_of_day in %s", string(b))
				}
				if _, ok := snapshotMap["TimeOfDay"]; ok {
					t.Fatalf("unexpected nested key snapshot.TimeOfDay in %s", string(b))
				}
			}
		})
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
