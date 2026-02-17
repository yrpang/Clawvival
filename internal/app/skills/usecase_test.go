package skills

import (
	"context"
	"errors"
	"testing"
)

func TestUseCase_IndexAndFile(t *testing.T) {
	provider := fakeProvider{
		index: []byte(`{"skills":[{"name":"demo"}]}`),
		files: map[string][]byte{"demo.md": []byte("content")},
	}
	uc := UseCase{Provider: provider}

	index, err := uc.Index(context.Background())
	if err != nil {
		t.Fatalf("index error: %v", err)
	}
	if string(index) != `{"skills":[{"name":"demo"}]}` {
		t.Fatalf("unexpected index: %q", string(index))
	}

	file, err := uc.File(context.Background(), "demo.md")
	if err != nil {
		t.Fatalf("file error: %v", err)
	}
	if string(file) != "content" {
		t.Fatalf("unexpected file: %q", string(file))
	}
}

func TestUseCase_PropagatesProviderError(t *testing.T) {
	wantErr := errors.New("boom")
	uc := UseCase{Provider: fakeProvider{err: wantErr}}

	if _, err := uc.Index(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("expected index error %v, got %v", wantErr, err)
	}
	if _, err := uc.File(context.Background(), "x.md"); !errors.Is(err, wantErr) {
		t.Fatalf("expected file error %v, got %v", wantErr, err)
	}
}

type fakeProvider struct {
	index []byte
	files map[string][]byte
	err   error
}

func (p fakeProvider) Index(_ context.Context) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.index, nil
}

func (p fakeProvider) File(_ context.Context, path string) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	if b, ok := p.files[path]; ok {
		return b, nil
	}
	return nil, errors.New("not found")
}
