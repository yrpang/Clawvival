package status

import (
	"context"
	"errors"
	"strings"

	"clawverse/internal/app/ports"
)

var ErrInvalidRequest = errors.New("invalid status request")

type UseCase struct {
	StateRepo ports.AgentStateRepository
}

func (u UseCase) Execute(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return Response{}, ErrInvalidRequest
	}
	state, err := u.StateRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return Response{}, err
	}
	return Response{State: state}, nil
}
