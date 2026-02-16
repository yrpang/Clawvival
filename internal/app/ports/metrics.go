package ports

import "clawverse/internal/domain/survival"

type ActionMetrics interface {
	RecordSuccess(resultCode survival.ResultCode)
	RecordConflict()
	RecordFailure()
}
