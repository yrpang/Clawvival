package replay

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawvival/internal/app/ports"
	"clawvival/internal/domain/survival"
)

var ErrInvalidRequest = errors.New("invalid replay request")

type UseCase struct {
	Events ports.EventRepository
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	fetchLimit := limit
	if hasFilters(req) {
		fetchLimit = 0
	}
	events, err := u.Events.ListByAgentID(ctx, req.AgentID, fetchLimit)
	if err != nil {
		return Response{}, err
	}
	events = filterByTimeWindow(events, req.OccurredFrom, req.OccurredTo)
	events = filterBySession(events, req.SessionID)
	events = applyLimit(events, limit)
	latest := reconstruct(events)
	latest.AgentID = req.AgentID
	return Response{Events: events, LatestState: latest}, nil
}

func hasFilters(req Request) bool {
	return req.OccurredFrom > 0 || req.OccurredTo > 0 || strings.TrimSpace(req.SessionID) != ""
}

func applyLimit(events []survival.DomainEvent, limit int) []survival.DomainEvent {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[:limit]
}

func filterByTimeWindow(events []survival.DomainEvent, from, to int64) []survival.DomainEvent {
	if from <= 0 && to <= 0 {
		return events
	}
	out := make([]survival.DomainEvent, 0, len(events))
	for _, evt := range events {
		ts := evt.OccurredAt.Unix()
		if from > 0 && ts < from {
			continue
		}
		if to > 0 && ts > to {
			continue
		}
		out = append(out, evt)
	}
	return out
}

func filterBySession(events []survival.DomainEvent, sessionID string) []survival.DomainEvent {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return events
	}
	out := make([]survival.DomainEvent, 0, len(events))
	for _, evt := range events {
		if evt.Payload == nil {
			continue
		}
		got, _ := evt.Payload["session_id"].(string)
		if got != sessionID {
			continue
		}
		out = append(out, evt)
	}
	return out
}

func reconstruct(events []survival.DomainEvent) survival.AgentStateAggregate {
	state := survival.AgentStateAggregate{}
	for _, evt := range events {
		after, ok := evt.Payload["state_after"].(map[string]any)
		if !ok {
			continue
		}
		state.Vitals.HP = int(num(after["hp"]))
		state.Vitals.Hunger = int(num(after["hunger"]))
		state.Vitals.Energy = int(num(after["energy"]))
		state.Position.X = int(num(after["x"]))
		state.Position.Y = int(num(after["y"]))
		break
	}
	return state
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		if t, ok := v.(time.Time); ok {
			return float64(t.Unix())
		}
		return 0
	}
}
