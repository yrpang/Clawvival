package staticskills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProvider_IndexAndFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"skills":[]}`), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := Provider{Root: root}
	index, err := p.Index(context.Background())
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if string(index) != `{"skills":[]}` {
		t.Fatalf("unexpected index content: %q", string(index))
	}

	b, err := p.File(context.Background(), "demo.md")
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected file content: %q", string(b))
	}
}

func TestProvider_FileRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Dir(root)
	outsidePath := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsidePath) })

	p := Provider{Root: root}

	if _, err := p.File(context.Background(), "../outside.txt"); err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}
}
