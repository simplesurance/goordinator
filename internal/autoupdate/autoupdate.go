package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v35/github"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	github_prov "github.com/simplesurance/goordinator/internal/provider/github"

	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/logfields"
)

// TODO: run sync periodically?

const loggerName = "autoupdater"
const defPeriodicTriggerInterval = 30 * time.Minute

type GithubClient interface {
	UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (bool, error)
	CombinedStatus(ctx context.Context, owner, repo, ref string) (string, time.Time, error)
	CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error
	ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) githubclt.PRIterator
}

// Retryer is an interface used for running GithubClient methods repeatedly if
// they fail with a temporary error.
type Retryer interface {
	Run(context.Context, func(context.Context) error, []zap.Field) error
}

// Autoupdater updates github pull-requests with their base-branches in a
// serialized manner.
// Per base-branch a seperate queue is kept internally, only the first PR in the queue is updated.
type Autoupdater struct {
	triggerOnAutomerge bool
	triggerLabels      map[string]struct{}
	repositories       map[Repository]struct{}

	periodicTriggerIntv time.Duration

	// TODO: use an interface instead of *provider.Event?
	ch     <-chan *github_prov.Event //except go-github types returned from github.ParseWebHook()
	logger *zap.Logger

	queues        map[BranchID]*queue
	firstSyncDone bool
	lock          sync.Mutex

	ghClient GithubClient
	retryer  Retryer

	wg           sync.WaitGroup
	shutdownChan chan struct{}

	// processedEventCnt counts the number of events that were processed.
	// It is currently only used in testcase to delay checks until a send
	// event was processed.
	processedEventCnt atomic.Uint64
}

type Repository struct {
	Owner          string
	RepositoryName string
}

func (r *Repository) String() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.RepositoryName)
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

func NewAutoupdater(
	ghClient GithubClient,
	eventChan <-chan *github_prov.Event,
	retryer Retryer,
	repos []Repository,
	triggerOnAutomerge bool,
	triggerLabels []string,
	opts ...Opt,
) *Autoupdater {
	repoMap := make(map[Repository]struct{}, len(repos))
	for _, r := range repos {
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
		repositories:        repoMap,
		periodicTriggerIntv: defPeriodicTriggerInterval,
		shutdownChan:        make(chan struct{}, 1),
	}

	for _, opt := range opts {
		opt(&a)
	}

	return &a

}

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

var logFieldEventIgnored = logfields.Event("github_event_ignored")

func (a *Autoupdater) isMonitoredRepository(owner, repositoryName string) bool {
	repo := Repository{
		Owner:          owner,
		RepositoryName: repositoryName,
	}

	_, exist := a.repositories[repo]
	return exist
}

func (a *Autoupdater) EventLoop() {
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
			a.lock.Lock()
			for _, q := range a.queues {
				q.ScheduleUpdateFirstPR(context.Background())
				a.logger.Debug("periodic run scheduled", q.baseBranch.Logfields...)
			}
			a.lock.Unlock()

		case <-a.shutdownChan:
			a.logger.Info("event loop terminating")
			return
		}
	}
}

func (a *Autoupdater) processEvent(ctx context.Context, event *github_prov.Event) {
	defer a.processedEventCnt.Inc()

	logger := a.logger.With(event.LogFields...)

	logger.Debug("event received")

	switch ev := event.Event.(type) {
	case *github.PullRequestEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for unmonitored repository",
				logFieldEventIgnored,
			)

			return
		}

		a.processPullRequestEvent(ctx, logger, ev)

	case *github.PushEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logFieldEventIgnored,
			)

			return
		}

		a.processPushEvent(ctx, logger, ev)

	case *github.StatusEvent:
		if !a.isMonitoredRepository(ev.GetRepo().GetOwner().GetLogin(), ev.GetRepo().GetName()) {
			logger.Debug(
				"event is for repository that is not monitored",
				logFieldEventIgnored,
			)

			return
		}

		a.processStatusEvent(ctx, logger, ev)

	default:
		logger.Debug("event ignored", logFieldEventIgnored)
	}
}

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

	switch ev.GetAction() {
	// TODO: If a Pull-Request is opened and in the open-dialog is
	// already the applied, will we receive a label-add event? Or do we
	// also have to monitor Open-Events for PRs that have the label already?

	case "auto_merge_enabled":
		if !a.triggerOnAutomerge {
			logger.Debug(
				"event ignored, triggerOnAutomerge is disabled",
				logFieldEventIgnored,
			)
			return
		}

		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequest(prNumber, branch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull-request information",
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		if err := a.Enqueue(ctx, bb, pr); err != nil {
			logError(
				logger,
				"ignoring event, could not append pull-request to queue",
				err,
				logFieldEventIgnored,
			)

			return
		}

	case "labeled":
		labelName := ev.GetLabel().GetName()
		logger = logger.With(zap.String("github.label_name", labelName))

		if ev.GetPullRequest().GetState() == "closed" {
			logger.Warn(
				"ignoring event, label was added to a closed pull-request",
				logFieldEventIgnored,
			)

			return
		}

		if labelName == "" {
			logger.Warn(
				"ignoring event, event with action 'labeled' has empty label name",
				logFieldEventIgnored,
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
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequest(prNumber, branch)
		if err != nil {
			logError(
				logger,
				"ignoring event, incomplete pull-request information",
				err,
				logFieldEventIgnored,
			)

			return
		}

		if err := a.Enqueue(ctx, bb, pr); err != nil {
			logger.Error(
				"ignoring event, enqueing pull-request failed",
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

	case "auto_merge_disabled":
		if !a.triggerOnAutomerge {
			logger.Debug(
				"event ignored, triggerOnAutomerge is disabled",
				logFieldEventIgnored,
			)

			return
		}

		fallthrough
	case "closed":
		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete base branch information",
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequest(prNumber, branch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull-request information",
				logFieldEventIgnored,
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
				logFieldEventIgnored,
			)
			return
		}

	case "unlabeled":
		labelName := ev.GetLabel().GetName()
		logger = logger.With(zap.String("github.label_name", labelName))

		if labelName == "" {
			logger.Warn(
				"ignoring event, event with action 'unlabeled' has empty label name",
				logFieldEventIgnored,
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
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		pr, err := NewPullRequest(prNumber, branch)
		if err != nil {
			logger.Warn(
				"ignoring event, incomplete pull-request information",
				logFieldEventIgnored,
				zap.Error(err),
			)

			return
		}

		_, err = a.Dequeue(ctx, bb, pr.Number)
		if err != nil {
			logError(logger, "ignoring event, disabling updates for pr failed", err)

			return
		}

	case "synchronize":
		bb, err := NewBaseBranch(owner, repo, baseBranch)
		if err != nil {
			logger.Warn(
				"can not resume PRs, incomplete base branch information",
				zap.Error(err),
				logFieldEventIgnored,
			)

			return
		}

		err = a.Resume(ctx, bb, prNumber)
		if err != nil && !errors.Is(err, ErrNotFound) {
			logError(logger, "resuming updates for prs failed", err)
		}

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
				logFieldEventIgnored,
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
			logError(logger, "changing base branch failed", err)
			return
		}

		logger.Info("base branch of PR changed, moved PR to new base-branch queue")

	default:
		logger.Debug("ignoring pull-request event",
			logFieldEventIgnored,
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
			logFieldEventIgnored,
			zap.Error(err),
		)

		return
	}
	if err := a.UpdateBranch(ctx, bb); err != nil {
		logger.Error(
			"triggering updates for pr failed",
			logFieldEventIgnored,
			zap.Error(err),
		)
		// TODO: log the error with error priority if
		// if another error then NotExist is returned

		// do not abort here, ResumeAllForBaseBranch() can still run
	}

	a.ResumeAllForBaseBranch(ctx, bb)
}

func (a *Autoupdater) processStatusEvent(ctx context.Context, logger *zap.Logger, ev *github.StatusEvent) {
	branches := ghBranchesAsStrings(ev.Branches)
	owner := ev.GetRepo().GetOwner().GetLogin()
	repo := ev.GetRepo().GetName()
	// TODO: ensure owner and repo are not empty

	logger = logger.With(
		zap.Strings("git.branches", branches),
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
	)

	logger.Debug("event received")

	if len(branches) == 0 {
		logger.Warn(
			"ignorning event, branch field is empty",
			logFieldEventIgnored,
		)

		return
	}

	switch ev.GetState() {
	case "error", "failure":
		err := a.SuspendUpdates(ctx, owner, repo, branches)
		if len(err) != 0 {
			logger.Error("suspending updates failed", zap.Errors("errors", err))
		}

	case "success":
		err := a.ResumeIfStatusIsSuccess(ctx, owner, repo, branches)
		if len(err) != 0 {
			logger.Error("resuming updates failed", zap.Errors("errors", err))
		}

	default:
		logger.Debug("ignoring event with unknown or not relevant status",
			logFieldEventIgnored,
			zap.String("github.status_event.state", ev.GetState()),
		)
	}
}

func (a *Autoupdater) Enqueue(ctx context.Context, baseBranch *BaseBranch, pr *PullRequest) error {
	var q *queue
	var exist bool

	logger := a.logger.With(baseBranch.Logfields...).With(pr.LogFields...)

	a.lock.Lock()
	defer a.lock.Unlock()

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

	return q.Enqueue(pr)
}

// Dequeue disabled autoupdates for a pull-request. If the was pull-request was queued for updates it is returned.
// If it was not, an error is returned.
func (a *Autoupdater) Dequeue(ctx context.Context, baseBranch *BaseBranch, prNumber int) (*PullRequest, error) {
	var q *queue
	var exist bool

	a.lock.Lock()
	defer a.lock.Unlock()

	q, exist = a.queues[baseBranch.BranchID]
	if !exist {
		return nil, fmt.Errorf("no queue for base branch exist: %w", ErrNotFound)
	}

	pr, err := q.Dequeue(prNumber)
	if err != nil {
		return nil, fmt.Errorf("disabling updates for pr failed: %w", err)
	}

	if q.IsEmpty() {
		delete(a.queues, baseBranch.BranchID)
		logger := a.logger.With(pr.LogFields...).With(baseBranch.Logfields...)
		logger.Debug("empty queue for base branch removed")
	}

	return pr, nil
}

// ResumeAllForBaseBranch resumes updates for all branches that are based on baseBranch
func (a *Autoupdater) ResumeAllForBaseBranch(ctx context.Context, baseBranch *BaseBranch) {
	a.lock.Lock()
	defer a.lock.Unlock()

	for bbID, q := range a.queues {
		if bbID == baseBranch.BranchID {
			q.ResumeAll()
		}
	}
}

// SuspendUpdates suspend updates for all Pull-Requests of the given branches.
func (a *Autoupdater) SuspendUpdates(ctx context.Context, owner, repo string, branchNames []string) []error {
	// TODO: iterating over all queues for all base branches and then for all it's queued prs is slow, improve this by e.g.
	// using a map or retrieving the pr number for a branch via the github api somehow
	var errors []error

	a.lock.Lock()
	defer a.lock.Unlock()

	for baseBranch, q := range a.queues {
		if baseBranch.Repository != repo || baseBranch.RepositoryOwner != owner {
			continue
		}
		prs := q.ActivePRsByBranch(branchNames)
		for _, pr := range prs {
			err := q.Suspend(pr.Number)
			if err != nil {
				// TODO add information about base branch to error
				errors = append(errors, fmt.Errorf("suspending updates for pr %d failed: %w", pr.Number, err))
			}
		}

	}

	return errors
}

func (a *Autoupdater) ResumeIfStatusIsSuccess(ctx context.Context, owner, repo string, branchNames []string) []error {
	// TODO: iterating over all queues for all base branches and then for all it's queued prs is slow, improve this by e.g.
	// using a map or retrieving the pr number for a branch via the github api somehow
	var errors []error

	a.lock.Lock()
	defer a.lock.Unlock()

	for baseBranch, q := range a.queues {
		if baseBranch.Repository != repo || baseBranch.RepositoryOwner != owner {
			continue
		}

		prs := q.SuspendedPRsbyBranch(branchNames)
		for _, pr := range prs {
			q.ScheduleResumePRIfStatusSuccessful(ctx, pr)
		}
	}

	return errors
}

// Resume resumes updates for a pull-request.
// If the pull-request is not queued for updates, ErrNotFound is returned.
func (a *Autoupdater) Resume(ctx context.Context, baseBranch *BaseBranch, prNumber int) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return ErrNotFound
	}

	return q.Resume(prNumber)
}

// UpdateBranch triggers updating the first PR queued for updates for the given
// baseBranch
func (a *Autoupdater) UpdateBranch(ctx context.Context, baseBranch *BaseBranch) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	q, exist := a.queues[baseBranch.BranchID]
	if !exist {
		return ErrNotFound
	}

	q.ScheduleUpdateFirstPR(ctx)
	return nil
}

// ChangeBaseBranch moves a pr from the queue for oldBaseBranch to the queue
// for newBaseBranch.
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

func (a *Autoupdater) Start() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.EventLoop()
	}()
}

func (a *Autoupdater) Stop() {
	a.logger.Debug("autoupdater terminating")

	select {
	case <-a.shutdownChan: // already closed
	default:
		close(a.shutdownChan)
	}

	a.logger.Debug("waiting for event-loop to terminate")
	a.wg.Wait()

	a.lock.Lock()
	defer a.lock.Unlock()

	for branchID, q := range a.queues {
		q.Stop()
		delete(a.queues, branchID)
	}

	a.logger.Debug("autoupdater terminated")
}
