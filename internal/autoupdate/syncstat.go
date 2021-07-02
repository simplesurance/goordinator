package autoupdate

import (
	"time"

	"go.uber.org/zap"
)

type syncStat struct {
	StartTime time.Time
	EndTime   time.Time
	Seen      uint
	Enqueued  uint
	Dequeued  uint
	Failures  uint
}

func (s *syncStat) LogFields() []zap.Field {
	return []zap.Field{
		zap.Duration("sync_duration", s.EndTime.Sub(s.StartTime)),
		zap.Uint("pr_sync.seen", s.Seen),
		zap.Uint("pr_sync.failures", s.Failures),
		zap.Uint("pr_sync.enqueued", s.Enqueued),
		zap.Uint("pr_sync.dequeued", s.Dequeued),
		zap.Uint("pr_sync.out_of_sync", s.Enqueued+s.Dequeued),
	}
}
