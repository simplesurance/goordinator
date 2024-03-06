package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v59/github"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	github_prov "github.com/simplesurance/goordinator/internal/provider/github"

	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/logfields"
)

const loggerName = "autoupdater"

const defPeriodicTriggerInterval = 30 * time.Minute

// GithubClient defines the methods of a GithubAPI Client that are used by the
// autoupdate implementation.
type GithubClient interface {
	UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (changed bool, scheduled bool, err error)
	CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error
	ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) githubclt.PRIterator
	ReadyForMerge(ctx context.Context, owner, repo string, prNumber int) (*githubclt.ReadyForMergeStatus, error)
	RemoveLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error
	AddLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error
}

// Retryer defines methods for running GithubClient operations repeately if
// they fail with a temporary error.
type Retryer interface {
	Run(context.Context, func(context.Context) error, []zap.Field) error
}

// Autoupdater implements processing webhook events, querying the GitHub API
// and enqueueing/dequeuing/triggering updating pull requests with the
// base-branch.
// Pull request branch updates are serialized per base-branch.
type Autoupdater struct {
	triggerOnAutomerge bool
	triggerLabels      map[string]struct{}
	headLabel          string
	monitoredRepos     map[Repository]struct{}

	// periodicTriggerIntv defines the time span between triggering
	// updates for the first pull request in the queues periodically.
	periodicTriggerIntv time.Duration

	ch     <-chan *github_prov.Event
	logger *zap.Logger

	// queues contains a queue for each base-branch for which pull requests
	// are queued for autoupdates.
	queues map[BranchID]*queue
	// queuesLock must be hold when accessing queues
	queuesLock sync.Mutex

	ghClient GithubClient
	retryer  Retryer

	// wg is a waitgroup for the event-loop go-routine.
	wg sync.WaitGroup
	// shutdownChan can be closed to communicate to the event-loop go-routine to terminate.
	shutdownChan chan struct{}

	// processedEventCnt counts the number of events that were processed.
	// It is currently only used in testcase to delay checks until a send
	// event was processed.
	processedEventCnt atomic.Uint64
}

// Repository is a datastructure that uniquely identifies a GitHub Repository.
type Repository struct {
	OwnerLogin     string
	RepositoryName string
}

func (r *Repository) String() string {
	return fmt.Sprintf("%s/%s", r.OwnerLogin, r.RepositoryName)
}

// DryRun is an option for NewAutoupdater.
// If it is enabled all GitHub API operation that could result in a change will
// be simulated and always succeed.
func DryRun(enabled bool) Opt {
	return func(a *Autoupdater) {
		if !enabled {
			return
		}

		a.ghClient = NewDryGithubClient(a.ghClient, a.logger)
		a.logger.Info("dry run enabled")
	}
}

type Opt func(*Autoupdater)

// NewAutoupdater creates an Autoupdater instance.
// Only webhook events for repositories listed in monitoredRepositories are processed.
// At least one trigger (triggerOnAutomerge or a label in triggerLabels) must
// be provided to trigger enqueuing pull requests for autoupdates via webhook events.
// When multiple event triggers are configured, the autoupdater reacts on each
// received Event individually.
// headLabel is the name of the GitHub label that is applied to the PR that is the first in the queue.
func NewAutoupdater(
	ghClient GithubClient,
	eventChan <-chan *github_prov.Event,
	retryer Retryer,
	monitoredRepositories []Repository,
	triggerOnAutomerge bool,
	triggerLabels []string,
	headLabel string,
	opts ...Opt,
) *Autoupdater {
	repoMap := make(map[Repository]struct{}, len(monitoredRepositories))
	for _, r := range monitoredRepositories {
		repoMap[r] = struct{}{}
	}

	a := Autoupdater{
		ghClient:            ghClient,
		ch:                  eventChan,
		logger:              zap.L().Named(loggerName),
		queues:              map[BranchID]*queue{},
		retryer:             retryer,
		wg:                  sync.WaitGroup{},
		triggerOnAutomerge:  triggerOnAutomerge,
		triggerLabels:       toStrSet(triggerLabels),
		headLabel:           headLabel,
		monitoredRepos:      repoMap,
		periodicTriggerIntv: defPeriodicTriggerInterval,
		shutdownChan:        make(chan struct{}, 1),
	}

	for _, opt := range opts {
		opt(&a)
	}

	return &a

}

// branchRefToRef returns ref without a leading refs/heads/ prefix.
func branchRefToRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func ghBranchesAsStrings(branches []*github.Branch) []string {
	result := make([]string, 0, len(branches))

	for _, branch := range branches {
		result = append(result, branch.GetName())
	}

	return result
}

// isMonitoredRepository returns true if the repository is listed in the a.monitoredRepos.
func (a *Autoupdater) isMonitoredRepository(owner, repositoryName string) bool {
	repo := Repository{
		OwnerLogin:     owner,
		RepositoryName: repositoryName,
	}

	_, exist := a.monitoredRepos[repo]
	return exist
}

// eventLoop receives GitHub webhook events from the eventChan and trigger
// periodic update operations on the first element in the queues.
// Updates are triggered periodically every a.periodicTriggerIntv, to prevent
// that pull requests became stuck because GitHub webhook event was missed.
// The eventLoop terminates when a.shutdownChan is closed.
func (a *Autoupdater) eventLoop() {
	a.logger.Info("autoupdater event loop started")

	periodicTrigger := time.NewTicker(a.periodicTriggerIntv)
	defer periodicTrigger.Stop()

	for {
		select {
		case event, open := <-a.ch:
			if !open {
				a.logger.Info("autoupdater event loop terminated")
				return
			}

			a.processEvent(context.Background(), event)

		case <-periodicTrigger.C:
			a.queuesLock.Lock()
			for _, q := range a.queues {
				q.ScheduleUpdate(context.Background())
				a.logger.Debug("periodic run scheduled", q.baseBranch.Logfields...)
			}
			a.queuesLock.Unlock()

		case <-a.shutdownChan:
			a.logger.Info("event loop terminating")
			return
		}
	}
}

// processEvent processes GitHub webhook events.
//
// Only events for repositories listed in a.monitoredRepositories are
// processed.
// auto_merge_enabled/auto_merge_disabled events are only processed when
// a.triggerOnAutomergeis enabled.
// labeled/unlabeled events are only processed if the label is listed in
// a.triggerLabels.
//
// The following actions are triggered on events:
//
// Enqueue a pull request on:
//
//   - PullRequestEvent auto_merge_enabled
//   - PullRequestEvent labeled
//
// Dequeue a pull request on
//
//   - PullRequestEvent closed
//   - PullRequestEvent auto_merge_disabled
//   - PullRequestEvent unlabeled
//
// Move a pull request to another base branch queue on:
//
//   - PullRequestEvent edited and Base object is set
//
// Resume updates for a suspended pull request on:
//
//   - PullRequestEvent synchronize for the pr branch
//   - PushEvent for it's base-branch
//   - StatusEvent with success state and the StatusCheckRollup is successful
//     and ReviewDecision approved.
//   - CheckRunEvent with a neutral, success or skipped check conclusion and
//     the StatusCheckRollup is successful and ReviewDecision approved.
//   - PullRequestReviewEvent with action submitted and state approved
//
// Trigger update with base-branch on:
//
//   - PushEvent for a base branch
//   - (updates for pull request branches on git-push are triggered via
//     PullRequest synchronize events)
//   - StatusEvent with error or failure state
//   - CheckRunEvent with cancelled, failure, timed_out or action_required
//     conclusion
//
// Other events are ignored and a debug message is logged for those.
func (a *Autoupdater) processEvent(ctx context.Context, event *github_prov.Event) {
	defer func() {
		a.processedEventCnt.Add(1)
		metrics.ProcessedEventsInc()
	}()

	logger := a.logger.With(event.LogFields...)

	switch ev := event.Event.(type) {
	case *github.PullRequestEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for unmonitored repository",
				logEventEventIgnored,
			)

			return
		}

		a.processPullRequestEvent(ctx, logger, ev)

	case *github.PushEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logEventEventIgnored,
			)

			return
		}

		a.processPushEvent(ctx, logger, ev)

	case *github.StatusEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logEventEventIgnored,
			)

			return
		}

		a.processStatusEvent(ctx, logger, ev)

	case *github.CheckRunEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logEventEventIgnored,
			)

			return
		}
		a.processCheckRunEvent(ctx, logger, ev)

	case *github.PullRequestReviewEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logEventEventIgnored,
			)

			return
		}

		a.processPullRequestReviewEvent(ctx, logger, ev)

	default:
		logger.Debug("event ignored", logEventEventIgnored)
	}
}

// logError logs an error with the given message and fields.
// If the error is of type ErrNotFound or ErrAlreadyExists the message is
// logged with Info priority, otherwise with Error priority.
func logError(logger *zap.Logger, msg string, err error, fields ...zapcore.Field) {
	fields = append([]zapcore.Field{zap.Error(err)}, fields...)

	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlreadyExists) {
		logger.Info(msg, fields...)
		return
	}

	logger.Error(msg, fields...)
}

func (a *Autoupdater) processPullRequestEvent(ctx context.Context, logger *zap.Logger, ev *github.PullRequestEvent) {
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()
	baseBranch := ev.GetPullRequest().GetBase().GetRef()

	prNumber := ev.GetNumber()
	// does not have the `refs/heads` prefix:
	branch := ev.GetPullRequest().GetHead().GetRef()

	logger = logger.With(
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
		logfields.Branch(branch),
		logfields.PullRequest(prNumber),
		zap.String("github.pull_request_event.action", ev.GetAction()),
	)

	logger.Debug("event received")

	switch action := ev.GetAction(); action {
	// TODO: If a pull request is opened and in the open-dialog is
	// already the applied, will we receive a label-add event? Or do we
	// also have to monitor Open-Events for PRs that have the label already?

	case "auto_merge_enabled":
		if !a.triggerOnAutomerge {
			logger.Debug(
				"event ignored, triggerOnAutomerge is disabled",
				logEventEventIgnored,
			)
			return
		}

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequestFromEvent(ev.GetPullRequest())
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull request information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		if err := a.Enqueue(ctx, bb, pr); err != nil {
			logError(
				logger,
				"ignoring event, could not append pull request to queue",
				err,
				logEventEventIgnored,
			)

			return
		}

		logger.Info(
			"pull request enqueued for updates",
			logEventEnqeued,
			logFieldReason("auto_merge_enabled"),
		)

	case "labeled":
		labelName := ev.GetLabel().GetName()
		logger = logger.With(zap.String("github.label_name", labelName))

		if ev.GetPullRequest().GetState() == "closed" {
			logger.Warn(
				"ignoring event, label was added to a closed pull request",
				logEventEventIgnored,
			)

			return
		}

		if labelName == "" {
			logger.Warn(
				"ignoring event, event with action 'labeled' has empty label name",
				logEventEventIgnored,
			)

			return
		}

		if _, exist := a.triggerLabels[labelName]; !exist {
			return
		}

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequestFromEvent(ev.GetPullRequest())
		if err != nil {
			logError(
				logger,
				"ignoring event, incomplete pull request information",
				err,
				logEventEventIgnored,
			)

			return
		}

		if err := a.Enqueue(ctx, bb, pr); err != nil {
			logger.Error(
				"ignoring event, enqueing pull request failed",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		logger.Info(
			"pull request enqueued for updates",
			logEventEnqeued,
			logFieldReason("labeled"),
		)

	case "auto_merge_disabled":
		if !a.triggerOnAutomerge {
			logger.Debug(
				"event ignored, triggerOnAutomerge is disabled",
				logEventEventIgnored,
			)

			return
		}

		fallthrough
	case "closed":
		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequestFromEvent(ev.GetPullRequest())
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull request information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		_, err = a.Dequeue(ctx, bb, pr.Number)
		if err != nil {
			logError(
				logger,
				"ignoring event, disabling updates for pr failed",
				err,
				logEventEventIgnored,
			)
			return
		}

		var reason zap.Field
		if action == "closed" {
			reason = logReasonPRClosed
		} else {
			reason = logFieldReason(action)
		}

		logger.Info(
			"pull request dequeued for updates",
			logEventDequeued,
			reason,
		)

	case "unlabeled":
		labelName := ev.GetLabel().GetName()
		logger = logger.With(zap.String("github.label_name", labelName))

		if labelName == "" {
			logger.Warn(
				"ignoring event, event with action 'unlabeled' has empty label name",
				logEventEventIgnored,
			)

			return
		}

		if _, exist := a.triggerLabels[labelName]; !exist {
			return
		}

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequestFromEvent(ev.GetPullRequest())
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull request information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		_, err = a.Dequeue(ctx, bb, pr.Number)
		if err != nil {
			logError(logger, "ignoring event, disabling updates for pr failed", err)

			return
		}

		logger.Info(
			"pull request dequeued for updates",
			logEventDequeued,
			logFieldReason("unlabeled"),
		)

	case "synchronize":
		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"can not resume PRs, incomplete base branch information",
				zap.Error(err),
				logEventEventIgnored,
			)

			return
		}

		_, err = a.TriggerUpdateIfFirst(ctx, bb, &PRNumber{Number: prNumber})
		if err == nil {
			logger.Info(
				"update for pull request triggered",
				logfields.Event("pull_request_update_triggered"),
				logFieldReason("branch_changed"),
			)

			// Resume not necessary, PR is alreay in the active
			// queue and the first element
			return
		}

		if !errors.Is(err, ErrNotFound) {
			logger.Error("triggering update for first pr failed", zap.Error(err))
		}

		err = a.Resume(ctx, bb, prNumber)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				logger.Debug("no pull requests suspended for the base branch")
				return
			}

			logger.Error("resuming updates for prs failed", zap.Error(err))

			return
		}

		logger.Info(
			"updates resumed, pr branch changed",
			logEventUpdatesResumed,
			logFieldReason("branch_changed"),
		)

		return

	case "edited":
		changes := ev.GetChanges()
		if changes == nil || changes.Base == nil || changes.Base.Ref.From == nil {
			return
		}

		oldBaseBranch, err := NewBaseBranch(owner, repo, *changes.Base.Ref.From)
		if err != nil {
			logger.Warn(
				"ignoring event, incomple old base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			return
		}

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"can not enqueue PR after base branch change, incomplete new base branch information",
				zap.Error(err),
			)

			return
		}

		if err := a.ChangeBaseBranch(ctx, oldBaseBranch, bb, prNumber); err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlreadyExists) {
				logger.Debug("changing base branch for pr failed failed, pr not qeueud",
					zap.Error(err))
				return
			}

			logger.Error("changing base branch for pr failed", zap.Error(err))
			return
		}

		logger.Info(
			"pr was moved to another base-branch queue",
			logfields.Event("base_branch_queue_changed"),
			logFieldReason("base_branch_changed"),
			zap.String("git.old_base_branch", oldBaseBranch.Branch),
			logfields.BaseBranch(bb.Branch),
		)

	default:
		logger.Debug("ignoring irrelevant pull request event",
			logEventEventIgnored,
		)
	}
}

func (a *Autoupdater) processPushEvent(ctx context.Context, logger *zap.Logger, ev *github.PushEvent) {
	// The changed branch could be a base-branch for other
	// PRs or a branch that is queued for autoupdates
	// itself.
	branch := branchRefToRef(ev.GetRef())
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()

	logger = logger.With(
		logfields.Branch(branch),
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
	)

	logger.Debug("event received")

	bb, err := NewBaseBranch(owner, repo, branch)
	if err != nil {
		logger.Warn(
			"ignoring event, incomplete branch information",
			logEventEventIgnored,
			zap.Error(err),
		)

		return
	}
	if err := a.UpdateBranch(ctx, bb); err != nil {
		if !errors.Is(err, ErrNotFound) {
			logger.Error(
				"triggering updates for pr failed",
				zap.Error(err),
				logEventEventIgnored,
			)
		}
	}

	a.ResumeAllForBaseBranch(ctx, bb)
}
func (a *Autoupdater) processPullRequestReviewEvent(ctx context.Context, logger *zap.Logger, ev *github.PullRequestReviewEvent) {
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()
	prNumber := ev.GetPullRequest().GetNumber()
	branch := ev.GetPullRequest().GetHead().GetRef()
	reviewState := ev.GetReview().GetState()
	action := ev.GetAction()
	submittedAt := ev.GetReview().GetSubmittedAt()

	logger = logger.With(
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
		logfields.Branch(branch),
		logfields.PullRequest(prNumber),
		zap.String("github.pull_request.review.state", reviewState),
		zap.String("github.pull_request.review.action", action),
		zap.Time("github.pull_request.review.submitted_at", submittedAt.Time),
	)

	switch reviewState {
	case "approved":
		if action == "submitted" {
			a.ResumeIfStatusPositive(ctx, owner, repo, []string{branch})
			return
		}

		if action != "dismissed" {
			logger.Debug("event ignored, irrelevant action value", logEventEventIgnored)
			return
		}

		fallthrough

	case "changes_requested":
		baseBranch := ev.GetPullRequest().GetBase().GetRef()
		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)
			return
		}

		_, err = a.TriggerUpdateIfFirst(ctx, bb, &PRNumber{Number: prNumber})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return
			}

			logger.Error("triggering update for first pr failed", zap.Error(err))
			return
		}

		logger.Info(
			"update for pull request triggered",
			logfields.Event("pull_request_update_triggered"),
			logFieldReason("pr_review_changes_requested"),
		)

		return

	default:
		logger.Debug("event ignored, irrelevant review state", logEventEventIgnored)
		return
	}
}

func prBranches(prs []*github.PullRequest) []string {
	res := make([]string, 0, len(prs))

	for _, pr := range prs {
		branch := pr.GetHead().GetRef()
		// should not happen that it is empty
		if branch != "" {
			res = append(res, pr.GetHead().GetRef())
		}
	}

	return res
}

func (a *Autoupdater) processCheckRunEvent(ctx context.Context, logger *zap.Logger, ev *github.CheckRunEvent) {
	checkRun := ev.GetCheckRun()
	branches := prBranches(checkRun.PullRequests)
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()

	logger = logger.With(
		zap.Strings("git.branches", branches),
		logfields.CheckConclusion(checkRun.GetConclusion()),
		logfields.CheckStatus(checkRun.GetStatus()),
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
	)

	logger.Debug("event received")

	if len(branches) == 0 {
		logger.Info(
			"ignoring event, pull request or branches fields are empty",
			logEventEventIgnored,
		)

		return
	}

	for _, pr := range checkRun.PullRequests {
		baseBranch := pr.GetBase().GetRef()
		prNumber := pr.GetNumber()

		logger = logger.With(
			logfields.BaseBranch(baseBranch),
			logfields.PullRequest(prNumber),
		)

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring check run event for pull-request, incomplete base branch information",
				logEventEventIgnored,
				zap.Error(err),
			)

			err := a.SetPRStaleSinceIfNewer(ctx, bb, pr.GetNumber(), time.Now())
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					logger.Debug(
						"pr not queued, can not update stale timestamp",
						logEventEventIgnored,
					)
					continue
				}
				logger.Error(
					"updating stale timestamp failed",
					zap.Error(err),
					logfields.Event("updating_stale_timestamp_failed"),
				)
			}
		}
	}

	switch checkRun.GetConclusion() {
	case "cancelled", "failure", "timed_out", "action_required":
		for _, branch := range branches {
			pr, err := a.TriggerUpdateIfFirstAllQueues(ctx, owner, repo, &PRBranch{BranchName: branch})
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					logger.Debug("processed checkRun event for branch that is not queued for updates")
				} else {
					logger.Error("triggering update failed", zap.Error(err))
				}

				continue
			}

			logger.With(pr.LogFields...).Info(
				"update triggered, negative check run conclusion received",
				logFieldReason("check_run_result_negative"),
				logEventUpdatesSuspended,
			)
		}

	case "", "neutral", "success", "skipped":
		a.ResumeIfStatusPositive(ctx, owner, repo, branches)

	default: // stale event is ignored
		logger.Info("ignoring event with irrelevant or unsupported check run conclusion",
			logEventEventIgnored,
		)
	}
}

func (a *Autoupdater) processStatusEvent(ctx context.Context, logger *zap.Logger, ev *github.StatusEvent) {
	branches := ghBranchesAsStrings(ev.Branches)
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()

	logger = logger.With(
		zap.Strings("git.branches", branches),
		logfields.StatusState(ev.GetState()),
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
	)

	logger.Debug("event received")

	if len(branches) == 0 {
		logger.Info(
			"ignorning event, pull request or branches fields are empty",
			logEventEventIgnored,
		)

		return
	}

	notFound := a.SetPRStaleSinceIfNewerByBranch(ctx, owner, repo, branches, ev.GetUpdatedAt().Time)
	if len(notFound) > 0 {
		logger.Debug(
			"no pr queued for branches, can not update stale timestamp",
			zap.Strings("not_found_branches", notFound),
			logEventEventIgnored,
		)
	}

	switch ev.GetState() {
	case "error", "failure":
		for _, branch := range branches {
			pr, err := a.TriggerUpdateIfFirstAllQueues(ctx, owner, repo, &PRBranch{BranchName: branch})
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					logger.Debug("processed status event for branch that is not queued for updates")
				} else {
					logger.Error("triggering  update failed", zap.Error(err))
				}

				continue
			}

			logger.With(pr.LogFields...).Info(
				"update triggered, negative status check event received",
				logFieldReason("status_check_negative"),
				logEventUpdatesSuspended,
			)
		}

	case "pending", "success":
		a.ResumeIfStatusPositive(ctx, owner, repo, branches)

	default:
		logger.Debug("ignoring event with irrelevant or unsupported status",
			logEventEventIgnored,
		)
	}
}

// Enqueue appends the pull request to the autoupdate queue for baseBranch.
// When it becomes the first element in the queue, it will be kept uptodate with it's baseBranch.
// If the pr is already enqueued a ErrAlreadyExists error is returned.
//
// If no queue for the baseBranch exist, it will be created.
func (a *Autoupdater) Enqueue(_ context.Context, baseBranch *BaseBranch, pr *PullRequest) error {
	var q *queue
	var exist bool

	logger := a.logger.With(baseBranch.Logfields...).With(pr.LogFields...)

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist = a.queues[baseBranch.BranchID]
	if !exist {
		q = newQueue(
			baseBranch,
			a.logger,
			a.ghClient,
			a.retryer,
		)

		a.queues[baseBranch.BranchID] = q
		logger.Debug("queue for base branch created",
			logfields.Event("update_queue_created"),
		)
	}

	if err := q.Enqueue(pr); err != nil {
		return err
	}

	metrics.EnqueueOpsInc(&baseBranch.BranchID)

	return nil
}

// Dequeue removes the pull request with number prNumber from the autoupdate queue of baseBranch.
// This disables keeping the pull request update with baseBranch.
// If no pull request is queued with prNumber a ErrNotFound error is returned.
//
// If the pull request was the only element in the baseBranch queue, the queue is removed.
func (a *Autoupdater) Dequeue(_ context.Context, baseBranch *BaseBranch, prNumber int) (*PullRequest, error) {
	var q *queue
	var exist bool

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist = a.queues[baseBranch.BranchID]
	if !exist {
		return nil, fmt.Errorf("no queue for base branch exist: %w", ErrNotFound)
	}

	pr, err := q.Dequeue(prNumber)
	if err != nil {
		return nil, fmt.Errorf("disabling updates for pr failed: %w", err)
	}

	metrics.DequeueOpsInc(&baseBranch.BranchID)

	if q.IsEmpty() {
		q.Stop()
		delete(a.queues, baseBranch.BranchID)

		logger := a.logger.With(pr.LogFields...).With(baseBranch.Logfields...)

		logger.Debug("empty queue for base branch removed")
	}

	return pr, nil
}

// ResumeAllForBaseBranch resumes updates for all Pull Requests that are based
// on baseBranch and for which updates are currently suspended.
func (a *Autoupdater) ResumeAllForBaseBranch(_ context.Context, baseBranch *BaseBranch) {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for bbID, q := range a.queues {
		if bbID == baseBranch.BranchID {
			q.ResumeAll()
		}
	}
}

// SuspendUpdates suspend updates for all pull requests that are queued and
// their branch name matches one of branchNames.
// It returns a list of pull requests for which updates were suspended.
func (a *Autoupdater) SuspendUpdates(
	_ context.Context,
	owner string,
	repo string,
	branchNames []string,
) ([]*PullRequest, []error) {
	var errors []error
	var updatedPrs []*PullRequest

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for baseBranch, q := range a.queues {
		if baseBranch.Repository != repo || baseBranch.RepositoryOwner != owner {
			continue
		}
		prs := q.ActivePRsByBranch(branchNames)
		for _, pr := range prs {
			err := q.Suspend(pr.Number)
			if err != nil {
				errors = append(errors, fmt.Errorf(
					"suspending updates for pr %d, base branch: %q failed: %w",
					pr.Number, baseBranch, err,
				))
			}
			updatedPrs = append(updatedPrs, pr)
		}
	}

	return updatedPrs, errors
}

// SetPRStaleSinceIfNewerByBranch sets the staleSince timestamp of the PRs for
// the given branch names to updatdAt, if it is newer then the current
// staleSince timestamp.
// The method returns a list of branch names for that no queued PR could be
// found.
func (a *Autoupdater) SetPRStaleSinceIfNewerByBranch(
	_ context.Context,
	owner, repo string,
	branchNames []string,
	updatedAt time.Time,
) (notFound []string) {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	missing := toStrSet(branchNames)
	for baseBranch, q := range a.queues {
		if baseBranch.Repository != repo || baseBranch.RepositoryOwner != owner {
			continue
		}

		missing = q.SetPRStaleSinceIfNewerByBranch(branchNames, updatedAt)
	}

	return strSetToSlice(missing)
}

// SetPRStaleSinceIfNewer sets the staleSince timestamp of the PR to updatedAt,
// if it is newer then the current staleSince timestamp.
// If the PR is not queued for autoupdates, ErrNotFound is returned.
func (a *Autoupdater) SetPRStaleSinceIfNewer(
	_ context.Context,
	baseBranch *BaseBranch,
	prNumber int,
	updatedAt time.Time,
) error {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return ErrNotFound
	}

	return q.SetPRStaleSinceIfNewer(prNumber, updatedAt)
}

// ResumeIfStatusPositive calls ScheduleResumePRIfStatusPositive for all queued
// PRs of the passed branchNames.
func (a *Autoupdater) ResumeIfStatusPositive(ctx context.Context, owner, repo string, branchNames []string) {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for baseBranch, q := range a.queues {
		if baseBranch.Repository != repo || baseBranch.RepositoryOwner != owner {
			continue
		}

		prs := q.SuspendedPRsbyBranch(branchNames)
		for _, pr := range prs {
			q.ScheduleResumePRIfStatusPositive(ctx, pr)
		}
	}
}

// Resume resumes updates for a pull request.
// If the pull request is not queued for updates and in suspended state
// ErrNotFound is returned.
func (a *Autoupdater) Resume(_ context.Context, baseBranch *BaseBranch, prNumber int) error {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return ErrNotFound
	}

	return q.Resume(prNumber)
}

// UpdateBranch triggers updating the first PR queued for updates for the given
// baseBranch.
//
// See documentation on queue for more information.
func (a *Autoupdater) UpdateBranch(ctx context.Context, baseBranch *BaseBranch) error {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return ErrNotFound
	}

	q.ScheduleUpdate(ctx)
	return nil
}

// ChangeBaseBranch dequeues a Pull Request from the queue oldBaseBranch and
// enqueues it at the queue for newBaseBranch.
func (a *Autoupdater) ChangeBaseBranch(
	ctx context.Context,
	oldBaseBranch, newBaseBranch *BaseBranch,
	prNumber int,
) error {
	pr, err := a.Dequeue(ctx, oldBaseBranch, prNumber)
	if err != nil {
		return fmt.Errorf("could not remove pr from queue for old base branch: %w", err)
	}

	if err := a.Enqueue(ctx, newBaseBranch, pr); err != nil {
		return fmt.Errorf("could not enqueue in queue for new base branch: %w", err)

	}

	return nil
}

// TriggerUpdateIfFirst schedules the update operation for the first pull
// request in the queue if it matches prSpec.
// If an update was triggered, the PullRequest is returned.
// If the first PR does not match prSpec, ErrNotFound is returned.
func (a *Autoupdater) TriggerUpdateIfFirst(
	ctx context.Context,
	baseBranch *BaseBranch,
	prSpec PRSpecifier,
) (*PullRequest, error) {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return nil, ErrNotFound
	}

	return a._triggerUpdateIfFirst(ctx, q, prSpec)
}

func (a *Autoupdater) _triggerUpdateIfFirst(
	ctx context.Context,
	q *queue,
	prSpec PRSpecifier,
) (*PullRequest, error) {
	logger := a.logger.With(q.baseBranch.Logfields...).With(prSpec.LogField())

	// there is a chance of a race here, the pr might not be first anymore
	// when ScheduleUpdateFirstPR() is called, this does not matter, if it
	// happens we run the operation one more time then necessary.
	first := q.FirstActive()
	if first == nil {
		logger.Debug("update not trigger, pr is not first in queue")
		return nil, ErrNotFound
	}

	switch v := prSpec.(type) {
	case *PRNumber:
		if first.Number == v.Number {
			q.ScheduleUpdate(ctx)
			return first, nil
		}

	case *PRBranch:
		if first.Branch == v.BranchName {
			q.ScheduleUpdate(ctx)
			return first, nil
		}

	default:
		logger.DPanic("unsupported type received", zap.String("type", fmt.Sprintf("%T", v)))
		return nil, fmt.Errorf("unsupported type of prSpec parameter: %T", v)
	}

	return nil, ErrNotFound
}

// TriggerUpdateIfFirstAllQueues does the same then
// _triggerUpdateIfFirst but does not require to specify the base
// branch name.
func (a *Autoupdater) TriggerUpdateIfFirstAllQueues(
	ctx context.Context,
	repoOwner string,
	repo string,
	prSpec PRSpecifier,
) (*PullRequest, error) {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for branchID, q := range a.queues {
		if branchID.Repository != repo || branchID.RepositoryOwner != repoOwner {
			continue
		}

		pr, err := a._triggerUpdateIfFirst(ctx, q, prSpec)
		if err == nil {
			return pr, nil
		}

		if errors.Is(err, ErrNotFound) {
			continue
		}
		return nil, fmt.Errorf("queue: %s: %w", q.String(), err)
	}

	return nil, ErrNotFound
}

// Start starts the event-loop in a go-routine.
// The event-loop reads events from the eventChan and processes them.
func (a *Autoupdater) Start() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.eventLoop()
	}()
}

// Stop stops the event-loop and waits until it terminates.
// All queues will be deleted, operations that are in progress will be canceled.
func (a *Autoupdater) Stop() {
	a.logger.Debug("autoupdater terminating")

	select {
	case <-a.shutdownChan: // already closed
	default:
		close(a.shutdownChan)
	}

	a.logger.Debug("waiting for event-loop to terminate")
	a.wg.Wait()

	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	for branchID, q := range a.queues {
		q.Stop()
		delete(a.queues, branchID)
	}

	a.logger.Debug("autoupdater terminated")
}

func (a *Autoupdater) HTTPService() *HTTPService {
	return NewHTTPService(a)
}
