package replay

import (
	"context"
	"errors"
	"strings"
	"time"

	"clawverse/internal/app/ports"
	"clawverse/internal/domain/survival"
)

var ErrInvalidRequest = errors.New("invalid replay request")

type UseCase struct {
	Events ports.EventRepository
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	events, err := u.Events.ListByAgentID(ctx, req.AgentID, req.Limit)
	if err != nil {
		return Response{}, err
	}
	events = filterByTimeWindow(events, req.OccurredFrom, req.OccurredTo)
	latest := reconstruct(events)
	latest.AgentID = req.AgentID
	return Response{Events: events, LatestState: latest}, nil
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
