package autoupdate

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

func (a *Autoupdater) HTTPHandlerList(resp http.ResponseWriter, req *http.Request) {
	var result strings.Builder
	// TODO: write to resp directly instead of to strings.Builder

	resp.Header().Add("Content-Type", "text/plain")

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for _, queue := range a.queues {
		result.WriteString(fmt.Sprintf(
			"Base: %s/%s %s\n",
			queue.baseBranch.RepositoryOwner,
			queue.baseBranch.Repository,
			queue.baseBranch.Branch,
		))

		var i int
		queue.active.Foreach(func(pr *PullRequest) bool {
			result.WriteString(fmt.Sprintf(
				"\t#%-4d PR: %4d\tAdded: %s, \t%s\n",
				i, pr.Number, pr.enqueuedSince.Format(time.RFC822), "active",
			))
			i++
			return true
		})

		for k, pr := range queue.suspended {
			result.WriteString(fmt.Sprintf(
				"\tPR: %-4d\tAdded: %s, \t%s\n", k, pr.enqueuedSince.Format(time.RFC822), "suspended",
			))
		}
	}

	_, err := resp.Write([]byte(result.String()))
	if err != nil {
		a.logger.Info(
			"writing to http-response writer failed",
			zap.Error(err),
		)
	}
}
