package autoupdate

import "time"

type httpListQueue struct {
	RepositoryOwner string
	Repository      string
	BaseBranch      string

	ActivePRs    []*PullRequest
	SuspendedPRs []*PullRequest
}

// httpListData is used as template data when rending the autoupdater list
// page.
type httpListData struct {
	Queues                  []*httpListQueue
	TriggerOnAutomerge      bool
	TriggerLabels           []string
	MonitoredRepositories   []string
	PeriodicTriggerInterval time.Duration
	ProcessedEvents         uint64

	// CreatedAt is the time when this datastructure was creted.
	CreatedAt time.Time
}

func (a *Autoupdater) httpListData() *httpListData {
	result := httpListData{CreatedAt: time.Now()}

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	result.TriggerOnAutomerge = a.triggerOnAutomerge

	for k := range a.triggerLabels {
		result.TriggerLabels = append(result.TriggerLabels, k)
	}

	for k := range a.monitoredRepos {
		result.MonitoredRepositories = append(result.MonitoredRepositories, k.String())
	}

	result.PeriodicTriggerInterval = a.periodicTriggerIntv
	result.ProcessedEvents = a.processedEventCnt.Load()

	for baseBranch, queue := range a.queues {
		queueData := httpListQueue{
			RepositoryOwner: baseBranch.RepositoryOwner,
			Repository:      baseBranch.Repository,
			BaseBranch:      baseBranch.Branch,
		}

		queueData.ActivePRs, queueData.SuspendedPRs = queue.asSlices()

		result.Queues = append(result.Queues, &queueData)
	}

	return &result
}
