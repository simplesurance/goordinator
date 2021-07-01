package autoupdate

import (
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

var (
	logEventUpdatesSuspended = logfields.Event("updates_suspended")
	logEventUpdatesResumed   = logfields.Event("updates_resumed")

	logEventEnqeued  = logfields.Event("enqueued")
	logEventDequeued = logfields.Event("dequeued")

	logReasonPRClosed = logFieldReason("pull_request_closed")
)

func logFieldReason(reason string) zap.Field {
	return zap.String("reason", reason)
}
