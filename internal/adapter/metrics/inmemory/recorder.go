package inmemory

import (
	"sync"

	"clawverse/internal/domain/survival"
)

type Snapshot struct {
	ActionTotal    uint64            `json:"action_total"`
	ActionSuccess  uint64            `json:"action_success"`
	ActionConflict uint64            `json:"action_conflict"`
	ActionFailure  uint64            `json:"action_failure"`
	ByResultCode   map[string]uint64 `json:"by_result_code"`
}

type Recorder struct {
	mu       sync.Mutex
	success  uint64
	conflict uint64
	failure  uint64
	byResult map[string]uint64
}

func NewRecorder() *Recorder {
	return &Recorder{
		byResult: map[string]uint64{},
	}
}

func (r *Recorder) RecordSuccess(resultCode survival.ResultCode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.success++
	r.byResult[string(resultCode)]++
}

func (r *Recorder) RecordConflict() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conflict++
}

func (r *Recorder) RecordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failure++
}

func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := Snapshot{
		ActionSuccess:  r.success,
		ActionConflict: r.conflict,
		ActionFailure:  r.failure,
		ActionTotal:    r.success + r.conflict + r.failure,
		ByResultCode:   make(map[string]uint64, len(r.byResult)),
	}
	for k, v := range r.byResult {
		out.ByResultCode[k] = v
	}
	return out
}

func (r *Recorder) SnapshotAny() any {
	return r.Snapshot()
}
