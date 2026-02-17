package staticskills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Provider struct {
	Root string
}

func (p Provider) Index(_ context.Context) ([]byte, error) {
	return os.ReadFile(filepath.Join(p.Root, "index.json"))
}

func (p Provider) File(_ context.Context, path string) ([]byte, error) {
	safePath, err := secureJoin(p.Root, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(safePath)
}

var ErrInvalidSkillsPath = errors.New("invalid skills filepath")

func secureJoin(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", ErrInvalidSkillsPath
	}
	if filepath.IsAbs(rel) {
		return "", ErrInvalidSkillsPath
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Clean(filepath.Join(rootAbs, rel))
	prefix := rootAbs + string(filepath.Separator)
	if target != rootAbs && !strings.HasPrefix(target, prefix) {
		return "", ErrInvalidSkillsPath
	}
	return target, nil
}
