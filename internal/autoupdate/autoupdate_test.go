package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-github/v40/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/autoupdate/mocks"
	"github.com/simplesurance/goordinator/internal/goordinator"
	github_prov "github.com/simplesurance/goordinator/internal/provider/github"
)

const repo = "repo"
const repoOwner = "testman"

const condCheckInterval = 20 * time.Millisecond
const condWaitTimeout = 5 * time.Second

// mustGetActivePR fetches from the base-branch queue of the autoupdater the
// pull request with the given pull request number.
// If the BaseBranch queue or the pull request does not exist, the testcase fails.
func mustGetActivePR(t *testing.T, autoupdater *Autoupdater, baseBranch *BaseBranch, prNumber int) *PullRequest {
	t.Helper()

	autoupdater.queuesLock.Lock()
	defer autoupdater.queuesLock.Unlock()

	queue := autoupdater.queues[baseBranch.BranchID]
	if queue == nil {
		t.Error("queue for base branch does not exist or is nil")
		return nil
	}

	queue.lock.Lock()
	defer queue.lock.Unlock()

	pr := queue.active.Get(prNumber)
	if pr == nil {
		t.Error("pull request does not exist in active queue")
		return nil
	}

	return pr
}

func mustQueueNotExist(t *testing.T, autoupdater *Autoupdater, baseBranch *BaseBranch) {
	t.Helper()

	autoupdater.queuesLock.Lock()
	defer autoupdater.queuesLock.Unlock()

	queue := autoupdater.queues[baseBranch.BranchID]
	if queue != nil {
		t.Errorf("queue for base branch (%s) exist but should not", baseBranch)
		return
	}

	t.Logf("queue for base branch (%s) does not exist", baseBranch)
}

// mockSuccessfulGithubUpdateBranchCall configures the mock to return a
// successful response for the UpdateBranch() call if is called for
// expectedPRNr.
// It is configured as the default, to expect exactly 1 invocation.
func mockSuccessfulGithubUpdateBranchCall(clt *mocks.MockGithubClient, expectedPRNr int, branchChanged bool) *gomock.Call {
	return clt.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Eq(expectedPRNr)).
		Return(branchChanged, nil)
}

func mockFailedGithubUpdateBranchCall(clt *mocks.MockGithubClient, expectedPRNr int) *gomock.Call {
	return clt.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Eq(expectedPRNr)).
		DoAndReturn(func(_ context.Context, owner, repo string, pullRequestNumber int) (bool, error) {
			return false, errors.New("error mocked by mockFailedGithubUpdateBranchCall")
		})
}

func mockSuccesssfulCreateIssueCommentCall(clt *mocks.MockGithubClient, expectedPRNr int) *gomock.Call {
	return clt.
		EXPECT().
		CreateIssueComment(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Eq(expectedPRNr), gomock.Any()).
		DoAndReturn(func(_ context.Context, owner, repo string, pullRequestNumber int, _ string) error {
			return nil
		})
}

func mockSuccessfulPullRequestIsApprovedCall(clt *mocks.MockGithubClient, expectedBranch string) *gomock.Call {
	return mockPullRequestIsApprovedCall(clt, expectedBranch, true)
}

func mockPullRequestIsApprovedCall(clt *mocks.MockGithubClient, expectedBranch string, result bool) *gomock.Call {
	return clt.
		EXPECT().
		PullRequestIsApproved(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Eq(expectedBranch)).
		DoAndReturn(func(_ context.Context, owner, repo, branch string) (bool, error) {
			return result, nil
		})
}

func mockCombindedStatus(clt *mocks.MockGithubClient, returnState string, returnLastChangedTime time.Time, returnErr error) *gomock.Call {
	return clt.EXPECT().
		CombinedStatus(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		DoAndReturn(func(_ context.Context, owner, repo, ref string) (string, time.Time, error) {
			return returnState, returnLastChangedTime, returnErr
		})
}

func (a *Autoupdater) getQueue(key BranchID) *queue {
	a.queuesLock.Lock()
	defer a.queuesLock.Unlock()

	return a.queues[key]
}

func (q *queue) activeLen() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.active.Len()
}

func (q *queue) suspendedLen() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.suspended)
}

func waitForProcessedEventCnt(t *testing.T, a *Autoupdater, wantedLen int) {
	t.Helper()

	require.Eventuallyf(
		t,
		func() bool { return a.processedEventCnt.Load() == uint64(wantedLen) },
		condWaitTimeout,
		condCheckInterval,
		"autoupdater processedEventCnt is: %d, expected: %d", a.processedEventCnt.Load(), wantedLen,
	)
}

func waitForSuspendQueueLen(t *testing.T, q *queue, wantedLen int) {
	t.Helper()

	require.Eventuallyf(
		t,
		func() bool { return q.suspendedLen() == wantedLen },
		condWaitTimeout,
		condCheckInterval,
		"queue %v suspended len is: %d, expected: %d", q.baseBranch, q.suspendedLen(), wantedLen,
	)
}

func waitForActiveQueueLen(t *testing.T, q *queue, wantedLen int) {
	t.Helper()

	require.Eventuallyf(
		t,
		func() bool { return q.activeLen() == wantedLen },
		condWaitTimeout,
		condCheckInterval,
		"queue %v active len is: %d, expected: %d", q.baseBranch, q.activeLen(), wantedLen,
	)
}

func TestEnqueueDequeue(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 10)
	defer close(evChan)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)
	mockSuccessfulGithubUpdateBranchCall(ghClient, pr.Number, true).AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1)

	mustGetActivePR(t, autoupdater, baseBranch, pr.Number)

	_, err = autoupdater.Dequeue(context.Background(), baseBranch, pr.Number)
	require.NoError(t, err)

	mustQueueNotExist(t, autoupdater, baseBranch)
}

func TestSuspendAndResume(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 10)
	defer close(evChan)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)
	mockSuccessfulGithubUpdateBranchCall(ghClient, pr.Number, true).AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "main").AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1)

	updatedPRs, errs := autoupdater.SuspendUpdates(context.Background(), repoOwner, repo, []string{pr.Branch})

	require.Empty(t, errs)
	assert.Contains(t, updatedPRs, pr)

	assert.Equal(t, queue.activeLen(), 0)
	assert.Equal(t, queue.suspendedLen(), 1)

	autoupdater.ResumeAllForBaseBranch(context.Background(), baseBranch)
	assert.Equal(t, queue.activeLen(), 1)

	queue.lock.Lock()
	assert.Empty(t, queue.suspended)
	queue.lock.Unlock()
}

func TestPushToPRBranchResumesPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	mockSuccessfulGithubUpdateBranchCall(ghClient, pr.Number, true).AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, pr.Branch).AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1)
	assert.Equal(t, queue.suspendedLen(), 0)

	_, errs := autoupdater.SuspendUpdates(context.Background(), repoOwner, repo, []string{pr.Branch})
	require.Empty(t, errs)

	assert.Equal(t, queue.activeLen(), 0)
	assert.Equal(t, queue.suspendedLen(), 1)

	evChan <- &github_prov.Event{Event: newSyncEvent(pr.Number, pr.Branch, baseBranch.Branch)}

	waitForProcessedEventCnt(t, autoupdater, 1)
	assert.Equal(t, queue.activeLen(), 1)
	assert.Equal(t, queue.suspendedLen(), 0)
}

func TestPushToBaseBranchTriggersUpdate(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	mockSuccessfulGithubUpdateBranchCall(ghClient, pr.Number, true).Times(2)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1)
	assert.Equal(t, queue.suspendedLen(), 0)

	evChan <- &github_prov.Event{Event: newPushEvent(baseBranch.Branch)}
	waitForProcessedEventCnt(t, autoupdater, 1)
}

func TestPushToBaseBranchResumesPRs(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockSuccessfulPullRequestIsApprovedCall(ghClient, prBranch).AnyTimes()
	mockFailedGithubUpdateBranchCall(ghClient, prNumber)
	mockSuccesssfulCreateIssueCommentCall(ghClient, prNumber)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch := "main"
	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	require.NotNil(t, queue)

	waitForSuspendQueueLen(t, queue, 1)
	require.Equal(t, queue.activeLen(), 0)

	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true)
	evChan <- &github_prov.Event{Event: newPushEvent(baseBranch)}

	waitForProcessedEventCnt(t, autoupdater, 2)
	assert.Equal(t, queue.activeLen(), 1, "active queue len")
	assert.Equal(t, queue.suspendedLen(), 0, "suspended queue len")
}

func TestPRBaseBranchChangeMovesItToAnotherQueue(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).Times(2)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	oldBaseBranch := "main"

	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, oldBaseBranch, triggerLabel)}

	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: oldBaseBranch})
	require.NotNil(t, queue)

	newBaseBranch := "develop"
	evChan <- &github_prov.Event{Event: newPullRequestBaseBranchChangeEvent(prNumber, prBranch, "main", newBaseBranch)}

	waitForProcessedEventCnt(t, autoupdater, 2)

	assert.Nil(
		t,
		autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: oldBaseBranch}),
	)

	queue = autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: newBaseBranch})
	require.NotNil(t, queue, "queue for new base branch does not exist")

	require.Equal(t, queue.activeLen(), 1)
	require.Len(t, queue.suspended, 0)
}

func TestUnlabellingPRDequeuesPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).Times(1)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").Times(1)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	oldBaseBranch := "main"

	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, oldBaseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: oldBaseBranch})
	require.NotNil(t, queue)

	evChan <- &github_prov.Event{Event: newPullRequestUnlabeledEvent(prNumber, prBranch, oldBaseBranch, triggerLabel)}

	waitForProcessedEventCnt(t, autoupdater, 2)

	autoupdater.queuesLock.Lock()
	assert.Len(t, autoupdater.queues, 0)
	autoupdater.queuesLock.Unlock()
}

func TestClosingPRDequeuesPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).Times(1)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch := "main"
	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	require.NotNil(t, queue)
	require.Equal(t, queue.activeLen(), 1)
	require.Len(t, queue.suspended, 0)

	evChan <- &github_prov.Event{Event: newPullRequestClosedEvent(prNumber, prBranch, baseBranch)}
	waitForProcessedEventCnt(t, autoupdater, 2)

	queue = autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	require.Nil(t, queue, "baseBranch queue still exist after only PR for the base branch was closed")
}

func TestSuccessStatusEventResumesPRs(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	pr1, err := NewPullRequest(1, "pr_branch1", "", "", "")
	require.NoError(t, err)

	pr2, err := NewPullRequest(2, "pr_branch2", "", "", "")
	require.NoError(t, err)

	pr3, err := NewPullRequest(3, "pr_branch3", "", "", "")
	require.NoError(t, err)

	ghClient.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		DoAndReturn(func(_ context.Context, owner, repo string, pullRequestNumber int) (bool, error) {
			return false, nil
		}).
		MinTimes(3)

	mockCombindedStatus(ghClient, "failed", time.Now(), nil).Times(3)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch1").AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch2").AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch3").AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch1, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	baseBranch2, err := NewBaseBranch(repoOwner, repo, "develop")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch1, pr1)
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch1, pr2)
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch2, pr3)
	require.NoError(t, err)

	queueBaseBranch1 := autoupdater.getQueue(baseBranch1.BranchID)
	require.NotNil(t, queueBaseBranch1)

	waitForSuspendQueueLen(t, queueBaseBranch1, 2)
	assert.Equal(t, 0, queueBaseBranch1.activeLen(), "active queue")
	assert.Equal(t, 2, queueBaseBranch1.suspendedLen(), "suspend queue")

	queueBaseBranch2 := autoupdater.getQueue(baseBranch2.BranchID)
	require.NotNil(t, queueBaseBranch2)

	waitForSuspendQueueLen(t, queueBaseBranch2, 1)

	assert.Equal(t, 0, queueBaseBranch2.activeLen(), "active queue")
	assert.Equal(t, 1, queueBaseBranch2.suspendedLen(), "suspend queue")

	mockCombindedStatus(ghClient, "success", time.Now(), nil).MinTimes(3)
	evChan <- &github_prov.Event{Event: newStatusEvent("success", pr1.Branch, pr2.Branch, pr3.Branch)}

	waitForSuspendQueueLen(t, queueBaseBranch1, 0)
	assert.Equal(t, 2, queueBaseBranch1.activeLen(), "active queue")
	assert.Equal(t, 0, queueBaseBranch1.suspendedLen(), "suspend queue")

	waitForSuspendQueueLen(t, queueBaseBranch2, 0)

	assert.Equal(t, 1, queueBaseBranch2.activeLen(), "active queue")
	assert.Equal(t, 0, queueBaseBranch2.suspendedLen(), "suspend queue")
}

func TestFailedStatusEventSuspendsFirstPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	pr1, err := NewPullRequest(1, "pr_branch1", "", "", "")
	require.NoError(t, err)

	pr2, err := NewPullRequest(2, "pr_branch2", "", "", "")
	require.NoError(t, err)

	pr3, err := NewPullRequest(3, "pr_branch3", "", "", "")
	require.NoError(t, err)

	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch1").AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch2").AnyTimes()
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch3").AnyTimes()

	ghClient.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		DoAndReturn(func(_ context.Context, owner, repo string, pullRequestNumber int) (bool, error) {
			return false, nil
		}).
		AnyTimes()

	var failChecks atomic.Bool
	ghClient.EXPECT().
		CombinedStatus(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		DoAndReturn(func(_ context.Context, owner, repo, ref string) (string, time.Time, error) {
			if failChecks.Load() {
				if ref == pr1.Branch || ref == pr3.Branch {
					t.Logf("FAILED, PR BRANCH: %q\n", ref)
					return "failed", time.Now(), nil
				}
			}
			t.Logf("SUCCESS, PR BRANCH: %q\n", ref)
			return "success", time.Now(), nil
		}).MinTimes(4)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch1, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	baseBranch2, err := NewBaseBranch(repoOwner, repo, "develop")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch1, pr1)
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch1, pr2)
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch2, pr3)
	require.NoError(t, err)

	queueBaseBranch1 := autoupdater.getQueue(baseBranch1.BranchID)
	require.NotNil(t, queueBaseBranch1)

	waitForActiveQueueLen(t, queueBaseBranch1, 2)
	assert.Equal(t, 0, queueBaseBranch1.suspendedLen(), "suspend queue")

	queueBaseBranch2 := autoupdater.getQueue(baseBranch2.BranchID)
	require.NotNil(t, queueBaseBranch2)

	waitForActiveQueueLen(t, queueBaseBranch2, 1)
	assert.Equal(t, 0, queueBaseBranch2.suspendedLen(), "suspend queue")

	failChecks.Store(true)
	evChan <- &github_prov.Event{Event: newStatusEvent("failure", pr1.Branch, pr2.Branch, pr3.Branch)}

	waitForSuspendQueueLen(t, queueBaseBranch1, 1)
	assert.Equal(t, 1, queueBaseBranch1.activeLen(), "active queue")

	waitForActiveQueueLen(t, queueBaseBranch2, 0)
	assert.Equal(t, 1, queueBaseBranch2.suspendedLen(), "suspend queue")
}

func TestPRIsSuspendedWhenStatusIsStuck(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	triggerLabel := "queue-add"

	pendingStatusCheckTs := time.Now().Add(-2 * time.Hour)
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, false).MinTimes(2)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").MinTimes(2)
	mockCombindedStatus(ghClient, "pending", pendingStatusCheckTs, nil).MinTimes(2)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)
	autoupdater.periodicTriggerIntv = time.Second

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	require.NotNil(t, queue)

	require.Eventually(
		t,
		func() bool { return !queue.getLastRun().IsZero() },
		condWaitTimeout,
		condCheckInterval,
	)

	assert.Equal(t, 1, queue.activeLen(), "active queue")
	assert.Equal(t, 0, queue.suspendedLen())

	queue.staleTimeout = time.Hour
	pr.SetStateUnchangedSince(time.Now().Add(-90 * time.Minute))

	waitForSuspendQueueLen(t, queue, 1)

	assert.Equal(t, 0, queue.activeLen(), "active queue")
	assert.Equal(t, 1, queue.suspendedLen(), "suspend queue")
}

func TestPRIsSuspendedWhenUptodateAndHasFailedStatus(t *testing.T) {
	type testcase struct {
		StatusState string
		StatusError error
	}

	testcases := []testcase{
		{
			StatusState: "error",
		},
		{
			StatusState: "failed",
		},
		{
			StatusState: "",
		},
		{
			StatusState: "notexistingState",
		},
		{
			StatusError: errors.New("status check failed"),
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("state:%q, err:%v", tc.StatusState, tc.StatusError), func(t *testing.T) {
			t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

			evChan := make(chan *github_prov.Event, 1)
			defer close(evChan)

			mockctrl := gomock.NewController(t)
			ghClient := mocks.NewMockGithubClient(mockctrl)

			prNumber := 1
			prBranch := "pr_branch"
			triggerLabel := "queue-add"

			mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, false).Times(1)
			mockCombindedStatus(ghClient, tc.StatusState, time.Now(), tc.StatusError).Times(1)
			mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch")

			retryer := goordinator.NewRetryer()
			autoupdater := NewAutoupdater(
				ghClient,
				evChan,
				retryer,
				[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
				true,
				[]string{triggerLabel},
			)

			autoupdater.Start()
			t.Cleanup(autoupdater.Stop)

			baseBranch := "main"
			evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
			waitForProcessedEventCnt(t, autoupdater, 1)

			queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
			require.NotNil(t, queue)
			assert.Equal(t, queue.activeLen(), 0)
			assert.Len(t, queue.suspended, 1)
		})
	}
}

func TestEnqueueDequeueByAutomergeEvents(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	prBranch := "pr_branch"
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).Times(1)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, prBranch).Times(1)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		nil,
	)
	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	evChan <- &github_prov.Event{Event: newPullRequestAutomergeEnabledEvent(prNumber, prBranch, baseBranch.Branch)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(baseBranch.BranchID)
	assert.Equal(t, queue.activeLen(), 1)
	assert.Equal(t, 0, queue.suspendedLen())

	evChan <- &github_prov.Event{Event: newPullRequestAutomergeDisabledEvent(prNumber, prBranch, baseBranch.Branch)}
	waitForProcessedEventCnt(t, autoupdater, 2)

	assert.Nil(t, autoupdater.getQueue(baseBranch.BranchID), "basebranch queue still exist after automerge was disabled")
}

func TestInitialSync(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	ghClient.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		Return(true, nil).
		AnyTimes()

	prIterNone := mocks.NewMockPRIterator(mockctrl)
	ghClient.
		EXPECT().
		ListPullRequests(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(prIterNone)

	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr1")
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr4")

	prAutoMergeEnabled := newBasicPullRequest(1, "main", "pr1")
	prAutoMergeEnabled.AutoMerge = &github.PullRequestAutoMerge{}

	prWithlabel := newBasicPullRequest(2, "main", "pr2")
	prWithlabel.Labels = []*github.Label{{Name: strPtr("queue-add")}}

	prWithoutTrigger := newBasicPullRequest(3, "main", "pr3")

	prBasedOnOtherPR2 := newBasicPullRequest(4, "pr3", "pr4")
	prBasedOnOtherPR2.AutoMerge = &github.PullRequestAutoMerge{}

	syncPRRequestsRet := []*github.PullRequest{
		prAutoMergeEnabled,
		prWithlabel,
		prWithoutTrigger,
		prBasedOnOtherPR2,
	}

	prIterNone.
		EXPECT().
		Next().
		DoAndReturn(func() (*github.PullRequest, error) {
			if len(syncPRRequestsRet) == 0 {
				return nil, nil
			}

			result := syncPRRequestsRet[0]
			syncPRRequestsRet = syncPRRequestsRet[1:]

			return result, nil
		}).
		Times(len(syncPRRequestsRet) + 1)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{"queue-add"},
	)

	err := autoupdater.Sync(context.Background())
	require.NoError(t, err)
	t.Cleanup(autoupdater.Stop)

	q := autoupdater.getQueue(BranchID{Repository: repo, RepositoryOwner: repoOwner, Branch: "main"})
	require.NotNil(t, q)
	q.lock.Lock()
	assert.NotNil(t, q.active.Get(1), "pr1 was not added to the queue")
	assert.NotNil(t, q.active.Get(2), "pr2 was not added to the queue")
	assert.Nil(t, q.active.Get(3), "pr3 was added to the queue but should not")
	q.lock.Unlock()

	q = autoupdater.getQueue(BranchID{Repository: repo, RepositoryOwner: repoOwner, Branch: "pr3"})
	require.NotNil(t, q)
	q.lock.Lock()
	assert.NotNil(t, q.active.Get(4), "pr4 was not added to the queue")
	q.lock.Unlock()
}

func TestFirstPRInQueueIsUpdatedPeriodically(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	prNumber := 1
	triggerLabel := "queue-add"
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).Times(2)
	mockSuccessfulPullRequestIsApprovedCall(ghClient, "pr_branch").Times(2)

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.periodicTriggerIntv = 2 * time.Second
	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)

	baseBranch, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)

	err = autoupdater.Enqueue(context.Background(), baseBranch, pr)
	require.NoError(t, err)

	time.Sleep(autoupdater.periodicTriggerIntv + time.Second)

	// the mocked GithubUpdateCall asserts that it was called 1x from the period update
}

func TestReviewApprovedEventResumesSuspendedPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))
	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)
	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockCombindedStatus(ghClient, "success", time.Now(), nil).Times(1)
	ghClient.
		EXPECT().
		PullRequestIsApproved(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Eq(prBranch)).
		DoAndReturn(func(_ context.Context, owner, repo, branch string) (bool, error) {
			return false, nil
		})

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch := "main"
	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	// PR should be in suspend queue, it is not approved
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 0)
	assert.Len(t, queue.suspended, 1)

	mockSuccessfulPullRequestIsApprovedCall(ghClient, prBranch).Times(2)
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).AnyTimes()

	evChan <- &github_prov.Event{Event: newPullRequestReviewEvent(prNumber, prBranch, baseBranch, "submitted", "approved")}
	waitForProcessedEventCnt(t, autoupdater, 2)
	assert.Equal(t, queue.activeLen(), 1)
	assert.Len(t, queue.suspended, 0)
}

func TestDismissingApprovalSuspendsActivePR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))
	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)
	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockPullRequestIsApprovedCall(ghClient, prBranch, true).Times(1)
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch := "main"
	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1, "pr not in active queue")
	assert.Len(t, queue.suspended, 0, "pr is suspended")

	mockPullRequestIsApprovedCall(ghClient, prBranch, false).Times(1)
	evChan <- &github_prov.Event{Event: newPullRequestReviewEvent(prNumber, prBranch, baseBranch, "dismissed", "approved")}
	waitForProcessedEventCnt(t, autoupdater, 2)
	assert.Equal(t, queue.activeLen(), 0, "pr is active")
	assert.Len(t, queue.suspended, 1, "pr not suspended")
}

func TestRequestingReviewChangesSuspendsPR(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))
	evChan := make(chan *github_prov.Event, 1)
	defer close(evChan)

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)
	prNumber := 1
	prBranch := "pr_branch"
	triggerLabel := "queue-add"

	mockPullRequestIsApprovedCall(ghClient, prBranch, true).Times(1)
	mockSuccessfulGithubUpdateBranchCall(ghClient, prNumber, true).AnyTimes()

	retryer := goordinator.NewRetryer()
	autoupdater := NewAutoupdater(
		ghClient,
		evChan,
		retryer,
		[]Repository{{OwnerLogin: repoOwner, RepositoryName: repo}},
		true,
		[]string{triggerLabel},
	)

	autoupdater.Start()
	t.Cleanup(autoupdater.Stop)

	baseBranch := "main"
	evChan <- &github_prov.Event{Event: newPullRequestLabeledEvent(prNumber, prBranch, baseBranch, triggerLabel)}
	waitForProcessedEventCnt(t, autoupdater, 1)

	queue := autoupdater.getQueue(BranchID{RepositoryOwner: repoOwner, Repository: repo, Branch: baseBranch})
	require.NotNil(t, queue)
	assert.Equal(t, queue.activeLen(), 1, "pr not in active queue")
	assert.Len(t, queue.suspended, 0, "pr is suspended")

	mockPullRequestIsApprovedCall(ghClient, prBranch, false).Times(1)
	evChan <- &github_prov.Event{Event: newPullRequestReviewEvent(prNumber, prBranch, baseBranch, "submitted", "changes_requested")}
	waitForProcessedEventCnt(t, autoupdater, 2)
	assert.Equal(t, queue.activeLen(), 0, "pr is active")
	assert.Len(t, queue.suspended, 1, "pr not suspended")
}
