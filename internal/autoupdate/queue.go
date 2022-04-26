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
	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/logfields"
)

// DefStaleTimeout is the default stale timeout.
// A pull request is considered as stale, when it is the first element in the
// queue it's state has not changed for longer then this timeout.
const DefStaleTimeout = 3 * time.Hour

// retryTimeout defines the maximum duration for which GitHub operation is
// retried on a temporary error. The longer the duration is, the longer it
// blocks the first element in the queue.
const retryTimeout = 20 * time.Minute

// queue implements a queue for automatically updating pull request branches
// with their base branch.
// Enqueued pull requests can either be in active or suspended state.
// Suspended pull requests are not updated.
// Active pull requests are stored in a FIFO-queue. The first pull request in
// the queue is kept uptodate with it's base branch.
//
// When the first element in the active queue changes, the q.updatePR()
// operation runs for the pull request.
// The update operation on the first active PR can also be triggered via
// queue.ScheduleUpdateFirstPR().
type queue struct {
	baseBranch BaseBranch

	// active contains pull requests enqueued for being kept uptodate
	active *orderedMap
	// suspended contains pull requests that are not kept uptodate
	suspended map[int]*PullRequest
	lock      sync.Mutex

	logger *zap.Logger

	ghClient GithubClient
	retryer  Retryer

	// actionPool is a go-routine pool that runs operations on active pull
	// requests asynchronously. The pool only contains 1 Go-Routine, to
	// ensure updates are run synchronously.
	actionPool *routines.Pool
	// executing contains a pointer to a runningTask struct describing the current or
	// last running pull request for that an action was run.
	// It's cancelFunc field is used is used to cancel actions for a
	// pull request when it is suspended while an update operation for it
	// is executed.
	executing atomic.Value // stored type: *runningTask

	// lastRun contains a time.Time struct holding the timestamp of the
	// last action() run, when action() has not be run yet it contains the
	// zero Time
	lastRun atomic.Value // stored type: time.Time

	updatePRRuns uint64 // atomic must be accessed via atomic functions

	staleTimeout time.Duration

	metrics *queueMetrics
}

func newQueue(base *BaseBranch, logger *zap.Logger, ghClient GithubClient, retryer Retryer) *queue {
	q := queue{
		baseBranch:   *base,
		active:       newOrderedMap(),
		suspended:    map[int]*PullRequest{},
		logger:       logger.Named("queue").With(base.Logfields...),
		ghClient:     ghClient,
		retryer:      retryer,
		actionPool:   routines.NewPool(1),
		staleTimeout: DefStaleTimeout,
	}

	q.setLastRun(time.Time{})

	if qm, err := newQueueMetrics(base.BranchID); err == nil {
		q.metrics = qm
	} else {
		q.logger.Warn(
			"could not create prometheus metrics",
			logfields.Event("creating_queue_metrics_failed"),
			zap.Error(err),
		)
	}

	return &q
}

func (q *queue) String() string {
	return fmt.Sprintf("queue for base branch: %s", q.baseBranch.String())
}

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

func (q *queue) getUpdateRuns() uint64 {
	return atomic.LoadUint64(&q.updatePRRuns)
}

func (q *queue) incUpdateRuns() {
	atomic.AddUint64(&q.updatePRRuns, 1)
}

// cancelActionForPR cancels a running update operation for the given pull
// request number.
// If none is running, nothing is done.
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

// IsEmpty returns true if the queue contains no active and suspended
// pull requests.
func (q *queue) IsEmpty() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.active.Len() == 0 && len(q.suspended) == 0
}

func (q *queue) _activePullRequests() []*PullRequest {
	return q.active.AsSlice()
}

func (q *queue) _suspendedPullRequests() []*PullRequest {
	result := make([]*PullRequest, 0, len(q.suspended))

	for _, v := range q.suspended {
		result = append(result, v)
	}

	return result
}

func (q *queue) asSlices() (activePRs, suspendedPRs []*PullRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q._activePullRequests(), q._suspendedPullRequests()
}

func (q *queue) _enqueueActive(pr *PullRequest) error {
	logger := q.logger.With(pr.LogFields...)

	pr.EnqueuedSince = time.Now()
	newFirstElemen, added := q.active.EnqueueIfNotExist(pr.Number, pr)
	if !added {
		return fmt.Errorf("pull request already exist in active queue: %w", ErrAlreadyExists)
	}

	q.metrics.ActiveQueueSizeInc()

	if newFirstElemen == nil {
		q.logger.Debug(
			"pull request appended to active queue",
			logfields.Event("pull_request_enqueued"),
		)

		return nil
	}

	logger.Debug(
		"pull request appended to active queue, first element changed, scheduling action",
		logfields.Event("pull_request_enqueued"),
	)

	q.scheduleUpdate(context.Background(), newFirstElemen)

	return nil
}

// Enqueue appends a pull request to the active queue.
// If it is the only element in the queue, the update operation is run for it.
// If it already exist, ErrAlreadyExists is returned.
func (q *queue) Enqueue(pr *PullRequest) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	if _, exist := q.suspended[pr.Number]; exist {
		return fmt.Errorf("pull request already exist in suspended queue: %w", ErrAlreadyExists)
	}

	return q._enqueueActive(pr)
}

func (q *queue) _dequeueSuspended(prNumber int) (*PullRequest, error) {
	pr, exist := q.suspended[prNumber]

	if !exist {
		return nil, ErrNotFound
	}

	delete(q.suspended, prNumber)
	q.metrics.SuspendQueueSizeDec()

	return pr, nil
}

func (q *queue) _dequeueActive(prNumber int) (removedPR *PullRequest, newFirstPr *PullRequest) {
	pr, newFirstElem := q.active.Dequeue(prNumber)
	if pr != nil {
		q.metrics.ActiveQueueSizeDec()
	}

	return pr, newFirstElem
}

// Dequeue removes the pull request with the given number from the active or
// suspended list.
// If an update operation is currently running for it, it is canceled.
// If the pull request does not exist in the queue, ErrNotFound is returned.
func (q *queue) Dequeue(prNumber int) (*PullRequest, error) {
	q.lock.Lock()

	if pr, err := q._dequeueSuspended(prNumber); err == nil {
		q.lock.Unlock()

		logger := q.logger.With(pr.LogFields...)
		logger.Debug(
			"pull request removed from suspend queue",
			logfields.Event("pull_request_dequeued"),
		)

		pr.SetStateUnchangedSince(time.Time{})

		return pr, nil
	} else if !errors.Is(err, ErrNotFound) {
		q.logger.DPanic("_dequeue_suspended returned unexpected error", zap.Error(err))
	}

	removed, newFirstElem := q._dequeueActive(prNumber)
	q.lock.Unlock()

	if removed == nil {
		return nil, ErrNotFound
	}

	q.cancelActionForPR(prNumber)
	removed.SetStateUnchangedSince(time.Time{})

	logger := q.logger.With(removed.LogFields...)

	logger.Debug(
		"pull request removed from active queue",
		logfields.Event("pull_request_dequeued"),
	)

	if newFirstElem == nil {
		return removed, nil
	}

	logger.Debug("removing pr changed first element, triggering action")

	q.scheduleUpdate(context.Background(), newFirstElem)

	return removed, nil
}

// Suspend suspends updates for the pull request with the given number.
// If an update operation is currently running for it, it is canceled.
// If is not active or not queued ErrNotFound is returned.
func (q *queue) Suspend(prNumber int) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	pr, newFirstElem := q._dequeueActive(prNumber)
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
	q.metrics.SuspendQueueSizeInc()

	if newFirstElem == nil {
		return nil
	}

	logger.Debug(
		"moving branch to suspend queue changed first element, triggering update",
		logfields.Event("pull_request_updates_suspended"),
	)

	q.scheduleUpdate(context.Background(), newFirstElem)

	return nil
}

// ResumeAll resumes updates for all pull request for that updating is
// currently suspended.
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

		_, _ = q._dequeueSuspended(prNum)
		logger.Info(
			"autoupdates for pr resumed",
			logfields.Event("pull_request_updates_resumed"),
		)
	}
}

// Resume resumes updates for the pull request with the given number.
// If the pull request is not queued and suspended ErrNotFound is returned.
// If the pull request is the only active pull request, the update operation is run for it.
func (q *queue) Resume(prNumber int) error {
	q.lock.Lock()
	pr, err := q._dequeueSuspended(prNumber)
	q.lock.Unlock()

	if err != nil {
		return err
	}

	logger := q.logger.With(pr.LogFields...)

	if err := q.Enqueue(pr); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			logger.Warn("pr was in active and suspend queue, removed it from suspend queue")
			return nil
		}

		return fmt.Errorf("enqueing previously suspended pr failed: %w", err)
	}

	return nil
}

func (q *queue) FirstActive() *PullRequest {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.active.First()
}

// ScheduleUpdate schedules updating the first pull request in the queue.
func (q *queue) ScheduleUpdate(ctx context.Context) {
	first := q.FirstActive()
	if first == nil {
		q.logger.Debug("ScheduleUpdateFirstPR was called but active queue is empty")
		return
	}

	q.scheduleUpdate(ctx, first)
}

func (q *queue) scheduleUpdate(ctx context.Context, pr *PullRequest) {
	q.actionPool.Queue(func() {
		ctx, cancelFunc := context.WithCancel(ctx)
		defer cancelFunc()

		q.setExecuting(&runningTask{pr: pr.Number, cancelFunc: cancelFunc})

		q.updatePR(ctx, pr)
	})

	q.logger.With(pr.LogFields...).
		Debug("update scheduled", logfields.Event("update_scheduled"))
}

func isPRIsClosedErr(err error) bool {
	const wantedErrStr = "pull request is closed"

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

// updatePR updates runs the update operation for the pull request.
// If the base-branch contains changes that are not in the pull request branch,
// updating it, by merging the base-branch into the PR branch, is schedule via
// the GitHub API.
// If updating is not possible because a merge-conflict exist or another error
// happened, a comment is posted to the pull request and updating the
// pull request is suspended.
// If it is already uptodate, it's GitHub check and status state is retrieved.
// If it is in a failed or error state, the pull request is suspended.
// If the status is successful, nothing is done and the pull request is kept as
// first element in the active queue.
// If the pull request was not updated, it's GitHub check status did not change
// and it is the first element in the queue longer then q.staleTimeout it is
// suspended.
func (q *queue) updatePR(ctx context.Context, pr *PullRequest) {
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

	defer q.incUpdateRuns()

	status, err := q.prReadyForMergeStatus(ctx, pr)
	if err != nil {
		logger.Error(
			"checking if pr merge status failed",
			logfields.Event("pr_merge_status_check_failed"),
			zap.Error(err),
		)
		return
	}

	logger = logger.With(
		logfields.ReviewDecision(status.ReviewDecision),
		logfields.StatusCheckRollupState(status.StatusCheckRollupState),
	)

	if status.ReviewDecision != githubclt.ReviewDecisionApproved {
		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it is not approved, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)

			return
		}

		logger.Info(
			"updates suspended, pr is not approved",
			logFieldReason("pr_not_approved"),
			logEventUpdatesSuspended,
			zap.Error(err),
		)

		return
	}

	logger.Debug("pr is approved")

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
				"updating branch with base branch failed, pull request is closed, removing PR from queue",
				logfields.Event("branch_update_failed"),
				zap.Error(baseBranchUpdateErr),
			)

			if _, err := q.Dequeue(pr.Number); err != nil {
				logger.Error(
					"removing pr from queue after failed update failed",
					logfields.Event("branch_update_failed"),
					zap.Error(err),
				)
				return
			}

			logger.Info(
				"pull request dequeued for updates",
				logEventDequeued,
				logReasonPRClosed,
			)

			return
		}

		if errors.Is(baseBranchUpdateErr, context.Canceled) {
			logger.Debug(
				"updating branch with base branch was cancelled",
				logfields.Event("branch_update_cancelled"),
			)

			return
		}

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

		logger.Info(
			"updates suspended, updating pr branch with base branch failed",
			logFieldReason("update_with_branch_failed"),
			logEventUpdatesSuspended,
			zap.Error(baseBranchUpdateErr),
		)

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

	if q.isPRStale(pr) {
		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's stale, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
			return
		}

		logger.Info(
			"updates suspended, pull request is stale",
			logFieldReason("stale"),
			zap.Time("last_pr_status_change", pr.GetStateUnchangedSince()),
			zap.Duration("stale_timeout", q.staleTimeout),
		)

		return
	}

	logger.Debug(
		"pull request is not stale",
		logfields.Event("pr_not_stale"),
		zap.Time("last_pr_status_change", pr.GetStateUnchangedSince()),
		zap.Duration("stale_timeout", q.staleTimeout),
	)

	logger = logger.With(logfields.StatusCheckRollupState(status.StatusCheckRollupState))

	switch status.StatusCheckRollupState {
	case githubclt.StatusStateSuccess:
		logger.Info(
			"pull request is uptodate, approved and status checks are successful",
			logfields.Event("pr_ready_to_merge"),
		)

	case "", githubclt.StatusStatePending, githubclt.StatusStateExpected:
		logger.Info(
			"pull request is uptodate, approved and status checks are pending",
			logfields.Event("pr_status_pending"),
		)

	case githubclt.StatusStateError, githubclt.StatusStateFailure:
		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's PR status is negative, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)
			return
		}

		logger.Info(
			"updates suspended, status check is negative",
			logFieldReason("status_check_negative"),
			logEventUpdatesSuspended,
			zap.Error(err),
		)

	default:
		logger.Warn(
			"pull request status check rollup state has unexpected value, suspending autoupdates for PR",
			logfields.Event("pr_status_check_rollup_state_invalid"),
		)

		if err := q.Suspend(pr.Number); err != nil {
			logger.Error(
				"suspending PR because it's status check rollup state has an invalid value, failed",
				logfields.Event("suspending_pr_updates_failed"),
				zap.Error(err),
			)

			return
		}

		logger.Info(
			"updates suspended, status check rollup value invalid",
			logFieldReason("status_check_rollup_state_invalid"),
			logEventUpdatesSuspended,
			zap.Error(err),
		)
	}
}

// prReadyForMergeStatus runs GitHubClient.ReadyForMergeStatus() and retries if
// it failed with a retryable error.
// The method blocks until the request was successful, a non-retryable error
// happened or the context expired.
func (q *queue) prReadyForMergeStatus(ctx context.Context, pr *PullRequest) (*githubclt.PRStatus, error) {
	var status *githubclt.PRStatus

	loggingFields := pr.LogFields

	ctx, cancelFunc := context.WithTimeout(ctx, retryTimeout)
	defer cancelFunc()

	err := q.retryer.Run(ctx, func(ctx context.Context) error {
		var err error

		status, err = q.ghClient.ReadyForMergeStatus(
			ctx,
			q.baseBranch.RepositoryOwner,
			q.baseBranch.Repository,
			pr.Number,
		)
		return err
	}, loggingFields)

	return status, err
}

func (q *queue) prsByBranch(branchNames map[string]struct{}) (
	prs []*PullRequest, notFound map[string]struct{},
) {
	q.lock.Lock()
	defer q.lock.Unlock()

	suspendedPrs, missing := q._suspendedPRsbyBranch(branchNames)
	activePrs, notFound := q._activePRsByBranch(missing)

	return append(suspendedPrs, activePrs...), notFound
}

// ActivePRsByBranch returns all pull requests that are in active state and for
// one of the branches in branchNames.
func (q *queue) ActivePRsByBranch(branchNames []string) []*PullRequest {
	branchSet := toStrSet(branchNames)

	q.lock.Lock()
	defer q.lock.Unlock()

	prs, _ := q._activePRsByBranch(branchSet)
	return prs
}

func (q *queue) _activePRsByBranch(branchSet map[string]struct{}) (
	prs []*PullRequest, notFound map[string]struct{},
) {
	var result []*PullRequest
	notFound = cpBranchNames(branchSet)

	q.active.Foreach(func(pr *PullRequest) bool {
		if _, exist := branchSet[pr.Branch]; exist {
			result = append(result, pr)
			delete(notFound, pr.Branch)
		}

		return true
	})

	return result, notFound
}

// SuspendedPRsbyBranch returns all pull requests that are in suspended state
// and for one of the branches in branchNames.
func (q *queue) SuspendedPRsbyBranch(branchNames []string) []*PullRequest {
	branchSet := toStrSet(branchNames)

	q.lock.Lock()
	defer q.lock.Unlock()

	prs, _ := q._suspendedPRsbyBranch(branchSet)
	return prs
}

func cpBranchNames(in map[string]struct{}) map[string]struct{} {
	res := make(map[string]struct{}, len(in))
	for k := range in {
		res[k] = struct{}{}
	}

	return res
}

func (q *queue) _suspendedPRsbyBranch(branchSet map[string]struct{}) (
	prs []*PullRequest, notfound map[string]struct{},
) {
	var result []*PullRequest
	notFound := cpBranchNames(branchSet)

	for _, pr := range q.suspended {
		if _, exist := branchSet[pr.Branch]; exist {
			result = append(result, pr)
			delete(notFound, pr.Branch)
		}
	}

	return result, notFound
}

func (q *queue) resumeIfPRMergeStatusPositive(ctx context.Context, logger *zap.Logger, pr *PullRequest) error {
	if _, exist := q.suspended[pr.Number]; !exist {
		return ErrNotFound
	}

	status, err := q.prReadyForMergeStatus(ctx, pr)
	if err != nil {
		return fmt.Errorf("retrieving ready for merge status failed: %w", err)
	}

	logger.Debug(
		"retrieved ready-to-merge-status",
		logfields.ReviewDecision(status.ReviewDecision),
		logfields.StatusCheckRollupState(status.StatusCheckRollupState),
	)

	if status.ReviewDecision != githubclt.ReviewDecisionApproved {
		logger.Info("updates for prs are not resumed, reviewdecision is not positive")
		return nil
	}

	switch status.StatusCheckRollupState {
	case "", githubclt.StatusStateExpected, githubclt.StatusStatePending, githubclt.StatusStateSuccess:
		if err := q.Resume(pr.Number); err != nil {
			return fmt.Errorf("resuming updates failed: %w", err)
		}

		logger.Info(
			"updates resumed, pr is approved and status check rollup is positive",
			logEventUpdatesResumed,
		)

		return nil

	default:
		logger.Info("updates for prs are not resumed, status check rollup state is unsuccessful")
		return nil
	}
}

// ScheduleResumePRIfStatusPositive schedules resuming autoupdates for a pull
// request when it's approved and it's check and status state is success,
// pending  or expected and it's review status is approved.
func (q *queue) ScheduleResumePRIfStatusPositive(ctx context.Context, pr *PullRequest) {
	q.actionPool.Queue(func() {
		logger := q.logger.With(pr.LogFields...)

		ctx, cancelFunc := context.WithCancel(ctx)
		defer cancelFunc()

		ctx, cancelFunc = context.WithTimeout(ctx, retryTimeout)
		defer cancelFunc()

		q.setExecuting(&runningTask{pr: pr.Number, cancelFunc: cancelFunc})

		err := q.resumeIfPRMergeStatusPositive(ctx, logger, pr)
		if err != nil && !errors.Is(err, ErrNotFound) {
			q.logger.With(pr.LogFields...).Info(
				"resuming updates if pr merge status positive failed",
				zap.Error(err),
			)
		}
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

// SetPRStaleSinceIfNewerByBranch sets the timestamp to when the last change on
// the PR happened to t, if t is newer then the current value, for the passed
// branches.
// The function returns a Set of branch names for that no PR in the queue could
// be found.
func (q *queue) SetPRStaleSinceIfNewerByBranch(branchNames []string, t time.Time) (
	notFound map[string]struct{}) {

	branchSet := toStrSet(branchNames)
	prs, notFound := q.prsByBranch(branchSet)

	for _, pr := range prs {
		pr.SetStateUnchangedSinceIfNewer(t)
	}

	return notFound
}

// SetPRStaleSinceIfNewer if a PullRequest with the given number exist
// in the active queue or dequeued list, it's unchangedSince timestamp is set to
// t, if it is newer.
// If it is older, nothing is done.
// If a PR with the given number can not be found, ErrNotFound is returned.
func (q *queue) SetPRStaleSinceIfNewer(prNumber int, t time.Time) error {
	pr := q.getPullRequest(prNumber)
	if pr == nil {
		return ErrNotFound
	}

	pr.SetStateUnchangedSinceIfNewer(t)
	return nil
}

// getPullRequest returns the PullRequest with the given PrNumber if it exist in
// the suspended list or active queue.
// If it does not, nil is returned.
func (q *queue) getPullRequest(prNumber int) *PullRequest {
	q.lock.Lock()
	defer q.lock.Unlock()

	pr, exist := q.suspended[prNumber]
	if exist {
		return pr
	}

	return q.active.Get(prNumber)
}
