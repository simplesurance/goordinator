package autoupdate

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type queueMetrics struct {
	activeQueueSize    prometheus.Gauge
	suspendedQueueSize prometheus.Gauge
}

func newQueueMetrics(baseBranch BranchID) (*queueMetrics, error) {
	result := queueMetrics{}

	activeQueueSize, err := metrics.queueSize.GetMetricWith(queueSizeLabels(&baseBranch, stateLabelActiveVal))
	if err != nil {
		return nil, fmt.Errorf("creating active queue size metric failed: %w", err)
	}
	result.activeQueueSize = activeQueueSize

	suspendedQueueSize, err := metrics.queueSize.GetMetricWith(queueSizeLabels(&baseBranch, stateLabelSuspendedVal))
	if err != nil {
		return nil, fmt.Errorf("creating suspended queue size metric failed: %w", err)
	}
	result.suspendedQueueSize = suspendedQueueSize

	return &result, nil
}

func (q *queueMetrics) ActiveQueueSizeInc() {
	if q == nil {
		return
	}

	q.activeQueueSize.Inc()
}

func (q *queueMetrics) ActiveQueueSizeDec() {
	if q == nil {
		return
	}

	q.activeQueueSize.Dec()
}

func (q *queueMetrics) SuspendQueueSizeInc() {
	if q == nil {
		return
	}

	q.suspendedQueueSize.Inc()
}

func (q *queueMetrics) SuspendQueueSizeDec() {
	if q == nil || q.suspendedQueueSize == nil {
		return
	}

	q.suspendedQueueSize.Dec()
}

func queueSizeLabels(branchID *BranchID, state stateLabelVal) prometheus.Labels {
	return prometheus.Labels{
		repositoryLabel: fmt.Sprintf("%s/%s", branchID.RepositoryOwner, branchID.Repository),
		baseBranchLabel: branchID.Branch,
		stateLabel:      string(state),
	}
}
