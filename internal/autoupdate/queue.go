package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/autoupdate/routines"
	"github.com/simplesurance/goordinator/internal/logfields"
)

// DefStaleTimeout is the default stale timeout.
// A Pull-Request is considered as stale, when it is the first element in the
// queue it's state has not changed for longer then the timeout.
// A state change means, that it was not updated with it's base branch, it's PR
// check status did not change and it became the first element later.
const DefStaleTimeout = 3 * time.Hour

// retryTimeout how long a github operation related to autoupdating retried at most
const retryTimeout = 20 * time.Minute

// queue implements a queue for autoupdating pull-request branches with their
// base branch.
// Enqueued Pull-Requests can either be in active or suspended state.
// Suspended Pull-Requests are not updated.
// Active Pull-Requests are stored in a FIFO-queue. The first pull-request in
// the queue is updated. Other active Pull-Requests are enqueued for being updated.
//
// A update operation can be triggered manually via queue.ScheduleUpdateFirstPR().
// It is also run automatically whenever the first element in active fifo-list changed.
// The Update operation tries to update the pull-requests with it's base-branch.
// If updating failed, the pull-request is suspended.
// If it is already uptodate, it's GitHub combined status check is retrieved.
// If it is in a failed state, the pull-request is suspended.
// If the pull-request status became stale it is also suspended.
// A Pull-Requests status is stale when it is uptodate and no status was
// reported or it's status is pending for longer then a the staleTimeout.
// TODO: continue doc
type queue struct {
	baseBranch BaseBranch

	active    *orderedMap
	suspended map[int]*PullRequest
	lock      sync.Mutex

	logger *zap.Logger

	ghClient GithubClient
	retryer  Retryer

	actionPool *routines.Pool
	executing  atomic.Value // stored type: *runningTask

	lastRun      atomic.Value // stored type: time.Time
	staleTimeout time.Duration
}

func newQueue(base *BaseBranch, logger *zap.Logger, ghClient GithubClient, retryer Retryer) *queue {
	q := queue{
		baseBranch:   *base,
		active:       newOrderedMap(),
		suspended:    map[int]*PullRequest{},
		logger:       logger.Named("queue").With(base.Logfields...),
		ghClient:     ghClient,
		retryer:      retryer,
		actionPool:   routines.NewPool(1), // one routine only, actions should run consecutively per basebranch
		staleTimeout: DefStaleTimeout,
	}

	q.setLastRun(time.Time{})

	return &q
}

// runningTask represents the task for that an update operation is currently
// running. The execution can be cancelled via the cancelFunc.
type runningTask struct {
	pr         int
	cancelFunc context.CancelFunc
}

func (q *queue) getExecuting() *runningTask {
	v := q.executing.Load()
	if v == nil {
		return nil
	}

	return v.(*runningTask)
}

func (q *queue) setExecuting(v *runningTask) {
	q.executing.Store(v)
}

func (q *queue) setLastRun(t time.Time) {
	q.lastRun.Store(t)
}

func (q *queue) getLastRun() time.Time {
	return q.lastRun.Load().(time.Time)
}

func (q *queue) cancelActionForPR(prNumber int) {
	if running := q.getExecuting(); running != nil {
		if running.pr == prNumber {
			running.cancelFunc()
			q.logger.Debug(
				"cancelled running task for pr",
				logfields.Event("task_cancelled"),
				logfields.PullRequest(prNumber),
			)
		}
	}
}

func (q *queue) IsEmpty() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.active.Len() == 0 && len(q.suspended) == 0
}

func (q *queue) _enqueueActive(pr *PullRequest) error {
	logger := q.logger.With(pr.LogFields...)

	pr.enqueuedSince = time.Now()
	newFirstElemen, added := q.active.EnqueueIfNotExist(pr.Number, pr)
	if !added {
		return fmt.Errorf("pull-request already exist in active queue: %w", ErrAlreadyExists)
	}

	if newFirstElemen == nil {
		q.logger.Debug(
			"pull-request appended to active queue",
			logfields.Event("pull_request_enqueued"),
		)

		return nil
	}

	logger.Debug(
		"pull-request appended to active queue, first element changed, scheduling action",
		logfields.Event("pull_request_enqueued"),
	)

	q.scheduleAction(context.Background(), newFirstElemen)

	return nil
}

// Enqueue enqueues a PR for automatic updates with it's base branch.
func (q *queue) Enqueue(pr *PullRequest) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	if _, exist := q.suspended[pr.Number]; exist {
		return fmt.Errorf("pull-request already exist in suspended queue: %w", ErrAlreadyExists)
	}

	return q._enqueueActive(pr)
}

func (q *queue) Dequeue(prNumber int) (*PullRequest, error) {
	q.lock.Lock()

	if pr, exist := q.suspended[prNumber]; exist {
		logger := q.logger.With(pr.LogFields...)

		delete(q.suspended, prNumber)
		q.lock.Unlock()

		logger.Info(
			"pull-request removed from suspend queue",
			logfields.Event("pull_request_dequeued"),
		)

		pr.SetStateUnchangedSince(time.Time{})

		return pr, nil
	}

	removed, newFirstElem := q.active.Dequeue(prNumber)
	q.lock.Unlock()

	if removed == nil {
		return nil, ErrNotFound
	}

	q.cancelActionForPR(prNumber)
	removed.SetStateUnchangedSince(time.Time{})

	logger := q.logger.With(removed.LogFields...)

	logger.Info(
		"pull-request removed from auto-update queue",
		logfields.Event("pull_request_dequeued"),
	)

	if newFirstElem == nil {
		return removed, nil
	}

	logger.Debug("removing pr changed first element, triggering action")

	q.scheduleAction(context.Background(), newFirstElem)

	return removed, nil
}

// Suspend moves a branch from the queue to the Suspend queue
func (q *queue) Suspend(prNumber int) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	pr, newFirstElem := q.active.Dequeue(prNumber)
	if pr == nil {
		return fmt.Errorf("pr not in active queue: %w", ErrNotFound)
	}

	if _, exist := q.suspended[prNumber]; exist {
		q.logger.DPanic("pr was in active and suspend queue, removed it from active queue")
		return nil
	}

	q.cancelActionForPR(prNumber)
	pr.SetStateUnchangedSince(time.Time{})

	logger := q.logger.With(pr.LogFields...)

	q.suspended[prNumber] = pr
	logger.Info(
		"updates for pr suspended",
		logfields.Event("pull_request_updates_suspended"),
	)

	if newFirstElem == nil {
		return nil
	}

	logger.Debug(
		"moving branch to suspend queue changed first element, triggering update",
		logfields.Event("pull_request_updates_suspended"),
	)

	q.scheduleAction(context.Background(), newFirstElem)

	return nil
}

func (q *queue) ResumeAll() {
	q.lock.Lock()
	defer q.lock.Unlock()

	for prNum, pr := range q.suspended {
		logger := q.logger.With(pr.LogFields...)

		if err := q._enqueueActive(pr); err != nil {
			logger.Error(
				"could not move PR from suspended to active state",
				logfields.Event("enqueue_failed"),
				zap.Error(err),
			)

			continue
		}

		delete(q.suspended, prNum)
		logger.Info(
			"autoupdates for pr resumed",
			logfields.Event("pull_request_updates_resumed"),
		)
	}
}

func (q *queue) Resume(prNumber int) error {
	q.lock.Lock()
	pr, exist := q.suspended[prNumber]
	if exist {
		delete(q.suspended, prNumber)
	}
	q.lock.Unlock()

	if !exist {
		return ErrNotFound
	}

	logger := q.logger.With(pr.LogFields...)

	if err := q.Enqueue(pr); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			q.logger.Warn("pr was in active and suspend queue, removed it from suspend queue")
			return nil
		}

		return fmt.Errorf("enqueing previously suspended pr failed: %w", err)
	}

	logger.Info(
		"pr moved from suspended to active queue",
		logfields.Event("pull_request_updates_resumed"),
	)

	return nil
}

func (q *queue) first() *PullRequest {
	q.lock.Lock()
	defer q.lock.Unlock()

	if first := q.active.First(); first != nil {
		return first
	}

	return nil
}

func (q *queue) ScheduleUpdateFirstPR(ctx context.Context) {
	first := q.first()
	if first == nil {
		q.logger.Debug("ScheduleUpdateFirstPR was called but queue is empty")
		return
	}

	q.scheduleAction(ctx, first)
}

func (q *queue) scheduleAction(ctx context.Context, pr *PullRequest) {
	q.actionPool.Queue(func() {
		ctx, cancelFunc := context.WithCancel(ctx)
		defer cancelFunc()

		q.setExecuting(&runningTask{pr: pr.Number, cancelFunc: cancelFunc})

		q.action(ctx, pr)
	})

	q.logger.With(pr.LogFields...).
		Debug("update scheduled", logfields.Event("update_scheduled"))
}

func isPRIsClosedErr(err error) bool {
	const wantedErrStr = "pull-request is closed"

	if unWrappedErr := errors.Unwrap(err); unWrappedErr != nil {
		if strings.Contains(unWrappedErr.Error(), wantedErrStr) {
			return true
		}
	}

	return strings.Contains(err.Error(), wantedErrStr)
}

// isPRStale returns true if the FirstElementSince timestamp is older then
// q.staleTimeout.
func (q *queue) isPRStale(pr *PullRequest) bool {
	lastStatusChange := pr.GetStateUnchangedSince()

	if lastStatusChange.IsZero() {
		// This can be caused by a race when action() is running and
		// the PR is dequeuend/suspended in the meantime.
		q.logger.Debug("stateUnchangedSince timestamp of pr is zero", pr.LogFields...)
		return false
	}

	return lastStatusChange.Add(q.staleTimeout).Before(time.Now())
}

func (q *queue) action(ctx context.Context, pr *PullRequest) {
	var branchChanged bool

	loggingFields := pr.LogFields
	logger := q.logger.With(loggingFields...)

	defer q.setExecuting(nil)

	// q.setLastRun() is wrapped in a func to evaluate time.Now() on
	// function exit instead of start
	defer func() { q.setLastRun(time.Now()) }()

	pr.SetStateUnchangedSinceIfZero(time.Now())

	ctx, cancelFunc := context.WithTimeout(ctx, retryTimeout)
	defer cancelFunc()

	baseBranchUpdateErr := q.retryer.Run(ctx, func(ctx context.Context) error {
		var err error
		branchChanged, err = q.ghClient.UpdateBranch(
			ctx,
			q.baseBranch.RepositoryOwner,
			q.baseBranch.Repository,
			pr.Number,
		)
		return err
	},
		loggingFields,
	)

	if baseBranchUpdateErr != nil {
		if isPRIsClosedErr(baseBranchUpdateErr) {
			logger.Info(
				"updating branch with base branch failed, pull-request is closed, removing PR from queue",
				logfields.Event("branch_update_failed"),
				zap.Error(baseBranchUpdateErr),
			)

			if _, err := q.Dequeue(pr.Number); err != nil {
				logger.Error(
					"removing pr from queue after failed update failed",
					logfields.Event("branch_update_failed"),
					zap.Error(err),
				)
			}

			return
		}

		if errors.Is(baseBranchUpdateErr, context.Canceled) {
			logger.Debug(
				"updatating branch with base branch was cancelled",
				logfields.Event("branch_update_cancelled"),
			)

			return
		}

		logger.Info(
			"updating branch with base branch failed, suspending autoupdates for branch",
			logfields.Event("branch_update_failed"),
			zap.Error(baseBranchUpdateErr),
		)

		// use a new context, otherwise it is forwarded for an
		// action on another branch, and cancelling action for
		// one branch, would cancel multiple others
		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR after branch update failed, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
			return
		}

		// the current ctx got cancelled in q.Suspend(), use
		// another context to prevent that posting the comment
		// gets cancelled, use a shorter timeout to prevent that this
		// operations blocks the queue unnecessary long, use a  shorter
		// timeout to prevent that this operations blocks the queue
		// unnecessary long
		ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelFunc()
		err := q.ghClient.CreateIssueComment(
			ctx,
			q.baseBranch.RepositoryOwner,
			q.baseBranch.Repository,
			pr.Number,
			fmt.Sprintf("goordinator: automatic base-branch updates suspended, updating branch failed:\n```%s```", baseBranchUpdateErr.Error()),
		)
		if err != nil {
			logger.Error("posting comment to github PR failed", zap.Error(err))
		}

		return
	}

	if branchChanged {
		logger.Info(
			"updating branch with changes from base branch scheduled",
			logfields.Event("github_branch_update_scheduled"),
		)

		pr.SetStateUnchangedSinceIfNewer(time.Now())

		return
	}

	state, lastChange, err := q.prCombinedStatus(ctx, pr)
	if err != nil {
		logger.Error(
			"retrieving status of pull-request failed, suspending PR to prevent that it blocks the queue",
			logfields.Event("autoupdate_suspended"),
			zap.Error(err),
		)

		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR after retrieving its status failed, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
		}

		return
	}

	pr.SetStateUnchangedSinceIfNewer(lastChange)

	logger = logger.With(zap.String("github.pull_request.combined_status", state))
	logger.Debug("retrieved combined pr status")

	if q.isPRStale(pr) {
		logger.Info(
			"pull-request is stale, suspending pr updates",
			logfields.Event("pr_is_stale"),
			zap.Time("last_pr_status_change", pr.GetStateUnchangedSince()),
			zap.Duration("stale_timeout", q.staleTimeout),
		)

		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's stale, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
		}

		return
	}

	logger.Debug(
		"pull-request is not stale",
		logfields.Event("pr_not_stale"),
		zap.Time("last_pr_status_change", pr.GetStateUnchangedSince()),
		zap.Duration("stale_timeout", q.staleTimeout),
	)

	switch state {
	case "success":
		logger.Info(
			"pull-request is uptodate and status checks are successful",
			logfields.Event("pr_ready_to_merge"),
		)

	case "pending":
		logger.Info(
			"pull-request is uptodate and status checks are pending",
			logfields.Event("pr_status_pending"),
		)

	case "failure", "error":
		logger.Info(
			"pull-request is uptodate and status check are negative, suspending autoupdates for branch",
			logfields.Event("autoupdate_suspended"),
			zap.Error(err),
		)

		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's PR status is negative, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
		}

	default:
		logger.Warn(
			"pull-request combined status has unexpected value, suspending autoupdates for PR",
			logfields.Event("pr_combined_status_unexpected"),
		)

		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's PR status has invalid value, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
		}
	}
}

// prCombinedStatus returns the combined status from Github for a PR.
// The method blocks until the request was successful, a non-retryable error
// happened or the context expired.
func (q *queue) prCombinedStatus(ctx context.Context, pr *PullRequest) (string, time.Time, error) {
	var state string
	var lastChange time.Time

	loggingFields := pr.LogFields

	err := q.retryer.Run(ctx, func(ctx context.Context) error {
		var err error

		state, lastChange, err = q.ghClient.CombinedStatus(
			ctx,
			q.baseBranch.RepositoryOwner,
			q.baseBranch.Repository,
			pr.Branch,
		)
		return err
	}, loggingFields)

	return state, lastChange, err
}

func toStrSet(sl []string) map[string]struct{} {
	result := make(map[string]struct{}, len(sl))

	for _, elem := range sl {
		result[elem] = struct{}{}
	}

	return result
}

func (q *queue) ActivePRsByBranch(branchNames []string) []*PullRequest {
	// TODO: make the lookup more efficient, do not iterate over the whole list
	var result []*PullRequest

	branchSet := toStrSet(branchNames)

	q.lock.Lock()
	defer q.lock.Unlock()

	q.active.Foreach(func(pr *PullRequest) bool {
		if _, exist := branchSet[pr.Branch]; exist {
			result = append(result, pr)
		}

		return true
	})

	return result
}

func (q *queue) SuspendedPRsbyBranch(branchNames []string) []*PullRequest {
	// TODO: make the lookup more efficient, do not iterate over the whole list
	var result []*PullRequest

	branchSet := toStrSet(branchNames)

	q.lock.Lock()
	defer q.lock.Unlock()

	for _, pr := range q.suspended {
		if _, exist := branchSet[pr.Branch]; exist {
			result = append(result, pr)
		}
	}

	return result
}
func (q *queue) resumeIfPRStatusIsSuccessful(ctx context.Context, pr *PullRequest) (bool, error) {
	status, _, err := q.prCombinedStatus(ctx, pr)
	if err != nil {
		return false, err
	}

	if status == "success" {
		if err := q.Resume(pr.Number); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (q *queue) ScheduleResumePRIfStatusSuccessful(ctx context.Context, pr *PullRequest) {
	q.actionPool.Queue(func() {
		ctx, cancelFunc := context.WithCancel(ctx)
		defer cancelFunc()

		ctx, cancelFunc = context.WithTimeout(ctx, retryTimeout)
		defer cancelFunc()

		q.setExecuting(&runningTask{pr: pr.Number, cancelFunc: cancelFunc})

		_, _ = q.resumeIfPRStatusIsSuccessful(ctx, pr)
	})

	q.logger.With(pr.LogFields...).
		Debug("checking PR status scheduled", logfields.Event("status_check_scheduled"))
}

func (q *queue) Stop() {
	q.logger.Debug("terminating")

	q.lock.Lock()
	// empty the qeueues to prevent that more work is scheduled
	q.active = newOrderedMap()
	q.suspended = map[int]*PullRequest{}
	q.lock.Unlock()
	if running := q.getExecuting(); running != nil {
		running.cancelFunc()
	}

	q.logger.Debug("waiting for routines to terminate")
	q.actionPool.Wait()
	q.logger.Debug("terminated")
}
