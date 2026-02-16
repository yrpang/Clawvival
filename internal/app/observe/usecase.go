package observe

import (
	"context"
	"errors"
	"strings"

	"clawverse/internal/app/ports"
)

var ErrInvalidRequest = errors.New("invalid observe request")

type UseCase struct {
	StateRepo ports.AgentStateRepository
	World     ports.WorldProvider
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	state, err := u.StateRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return Response{}, err
	}
	snapshot, err := u.World.SnapshotForAgent(ctx, req.AgentID)
	if err != nil {
		return Response{}, err
	}
	return Response{State: state, Snapshot: snapshot}, nil
}
