package ports

import "context"

type SkillsProvider interface {
	Index(ctx context.Context) ([]byte, error)
	File(ctx context.Context, path string) ([]byte, error)
}
