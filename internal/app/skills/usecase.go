package skills

import (
	"context"

	"clawverse/internal/app/ports"
)

type UseCase struct {
	Provider ports.SkillsProvider
}

func (u UseCase) Index(ctx context.Context) ([]byte, error) {
	return u.Provider.Index(ctx)
}

func (u UseCase) File(ctx context.Context, path string) ([]byte, error) {
	return u.Provider.File(ctx, path)
}
