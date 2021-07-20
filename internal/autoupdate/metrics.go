package autoupdate

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

const metricNamespace = "goordinator_autoupdater"

const (
	queueOperationsMetricName = "queue_operations_total"
	githubEventsMetricName    = "processed_github_events_total"
	queuedPRCountMetricName   = "queued_prs_count"
)

const (
	baseBranchLabel = "base_branch"
	repositoryLabel = "repository"
	operationLabel  = "operation"
	stateLabel      = "state"
)

type operationLabelVal string

const (
	operationLabelEnqueueVal operationLabelVal = "enqueue"
	operationLabelDequeueVal operationLabelVal = "dequeue"
)

type stateLabelVal string

const (
	stateLabelActiveVal    stateLabelVal = "active"
	stateLabelSuspendedVal stateLabelVal = "suspended"
)

type metricCollector struct {
	logger          *zap.Logger
	queueOps        *prometheus.CounterVec
	processedEvents prometheus.Counter
	queueSize       *prometheus.GaugeVec
}

var metrics = newMetricCollector()

func newMetricCollector() *metricCollector {
	return &metricCollector{
		logger: zap.L().Named(loggerName).Named("metrics"),
		queueOps: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricNamespace,
				Name:      queueOperationsMetricName,
				Help:      "count of queue operations",
			},
			[]string{repositoryLabel, baseBranchLabel, operationLabel},
		),
		processedEvents: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: metricNamespace,
				Name:      githubEventsMetricName,
				Help:      "count of processed github webhook events",
			},
		),
		queueSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricNamespace,
				Name:      queuedPRCountMetricName,
				Help:      "count of processed github webhook events",
			},
			[]string{repositoryLabel, baseBranchLabel, stateLabel},
		),
	}
}

func (m *metricCollector) logGetMetricFailed(metricName string, err error) {
	m.logger.Warn(
		"could not record metric",
		zap.String("metric", metricName),
		logfields.Event("recording_metric_failed"),
		zap.Error(err),
	)
}

func enqueueOpsLabels(branchID *BranchID, operation operationLabelVal) prometheus.Labels {
	return prometheus.Labels{
		repositoryLabel: fmt.Sprintf("%s/%s", branchID.RepositoryOwner, branchID.Repository),
		baseBranchLabel: branchID.Branch,
		operationLabel:  string(operation),
	}
}

func (m *metricCollector) EnqueueOpsInc(baseBranchID *BranchID) {
	cnt, err := m.queueOps.GetMetricWith(enqueueOpsLabels(baseBranchID, operationLabelEnqueueVal))
	if err != nil {
		m.logGetMetricFailed("enqueue_ops_total", err)
		return
	}

	cnt.Inc()
}

func (m *metricCollector) DequeueOpsInc(baseBranchID *BranchID) {
	cnt, err := m.queueOps.GetMetricWith(enqueueOpsLabels(baseBranchID, operationLabelDequeueVal))
	if err != nil {
		m.logGetMetricFailed(queueOperationsMetricName, err)
		return
	}

	cnt.Inc()
}

func (m *metricCollector) ProcessedEventsInc() {
	m.processedEvents.Inc()
}
