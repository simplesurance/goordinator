package autoupdate

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

type httpRespWriter struct {
	http.ResponseWriter
	logger *zap.Logger
}

func newHTTPRespWriter(logger *zap.Logger, resp http.ResponseWriter) *httpRespWriter {
	return &httpRespWriter{
		ResponseWriter: resp,
		logger:         logger,
	}
}

// WriteStr writes a string to the http response write.
// If it is successul true is returned.
// If it fails, the error is logged with info priority and false is returned.
func (rw *httpRespWriter) WriteStr(str string) (wasSuccessful bool) {
	_, err := rw.ResponseWriter.Write([]byte(str))
	if err != nil {
		rw.logger.Info("sending http response failed", zap.Error(err))
		return false
	}

	return true
}

func (a *Autoupdater) HTTPHandlerList(respWr http.ResponseWriter, req *http.Request) {
	const separator = "------------------------------------------------------------------------------"

	var result strings.Builder
	// TODO: write to resp directly instead of to strings.Builder

	resp := newHTTPRespWriter(a.logger, respWr)

	resp.Header().Add("Content-Type", "text/plain")

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	if len(a.queues) == 0 {
		resp.WriteStr("no pull-requests queued for updates\n")
		return
	}

	var queueCnt int
	for _, queue := range a.queues {
		success := resp.WriteStr(fmt.Sprintf(
			"Base: %s/%s %s\n",
			queue.baseBranch.RepositoryOwner,
			queue.baseBranch.Repository,
			queue.baseBranch.Branch,
		))
		if !success {
			return
		}

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

		if queueCnt < len(a.queues)-1 {
			result.WriteString("\n" + separator + "\n\n")
		}

		queueCnt++
	}

	resp.WriteStr(result.String())
}
