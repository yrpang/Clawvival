package staticskills

import (
	"context"
	"os"
	"path/filepath"
)

type Provider struct {
	Root string
}

func (p Provider) Index(_ context.Context) ([]byte, error) {
	return os.ReadFile(filepath.Join(p.Root, "index.json"))
}

func (p Provider) File(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(p.Root, path))
}
