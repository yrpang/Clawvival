package ports

import "clawvival/internal/domain/survival"

type ActionMetrics interface {
	RecordSuccess(resultCode survival.ResultCode)
	RecordConflict()
	RecordFailure()
}
