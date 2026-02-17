package inmemory

import (
	"testing"

	"clawvival/internal/domain/survival"
)

func TestRecorderSnapshot(t *testing.T) {
	r := NewRecorder()
	r.RecordSuccess(survival.ResultOK)
	r.RecordSuccess(survival.ResultGameOver)
	r.RecordConflict()
	r.RecordFailure()

	s := r.Snapshot()
	if s.ActionTotal != 4 {
		t.Fatalf("expected total 4, got %d", s.ActionTotal)
	}
	if s.ActionSuccess != 2 {
		t.Fatalf("expected success 2, got %d", s.ActionSuccess)
	}
	if s.ActionConflict != 1 {
		t.Fatalf("expected conflict 1, got %d", s.ActionConflict)
	}
	if s.ActionFailure != 1 {
		t.Fatalf("expected failure 1, got %d", s.ActionFailure)
	}
	if s.ByResultCode[string(survival.ResultOK)] != 1 {
		t.Fatalf("expected result ok count 1")
	}
	if s.ByResultCode[string(survival.ResultGameOver)] != 1 {
		t.Fatalf("expected result game_over count 1")
	}
}
