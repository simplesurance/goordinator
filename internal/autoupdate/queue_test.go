package autoupdate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/autoupdate/mocks"
	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/goordinator"
)

func TestUpdatePR_DoesNotCallBaseBranchUpdateIfPRIsNotApproved(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	mockSuccessfulGithubRemoveLabelQueueHeadCall(ghClient, 1).Times(1)

	bb, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)
	q := newQueue(bb, zap.L(), ghClient, goordinator.NewRetryer(), "first")
	t.Cleanup(q.Stop)

	pr, err := NewPullRequest(1, "testbr", "fho", "test pr", "")
	require.NoError(t, err)

	_, added := q.active.EnqueueIfNotExist(pr.Number, pr)
	require.True(t, added)

	mockReadyForMergeStatus(
		ghClient, pr.Number,
		githubclt.ReviewDecisionChangesRequested, githubclt.CIStatusPending,
	).AnyTimes()
	ghClient.EXPECT().UpdateBranch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	q.updatePR(context.Background(), pr)
}

func TestUpdatePRWithBaseReturnsChangedWhenScheduled(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	var updateBranchCalls int32
	ghClient.
		EXPECT().
		UpdateBranch(gomock.Any(), gomock.Eq(repoOwner), gomock.Eq(repo), gomock.Any()).
		DoAndReturn(func(context.Context, string, string, int) (*githubclt.UpdateBranchResult, error) {
			if updateBranchCalls == 0 {
				updateBranchCalls = updateBranchCalls + 1
				return &githubclt.UpdateBranchResult{HeadCommitID: headCommitID, Changed: true, Scheduled: true}, nil
			}
			updateBranchCalls = updateBranchCalls + 1
			return &githubclt.UpdateBranchResult{HeadCommitID: headCommitID, Changed: false, Scheduled: false}, nil
		}).
		Times(2)

	bb, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)
	q := newQueue(bb, zap.L(), ghClient, goordinator.NewRetryer(), "first")

	pr, err := NewPullRequest(1, "pr_branch", "", "", "")
	require.NoError(t, err)
	changed, headCommit, err := q.updatePRWithBase(context.Background(), pr, zap.L(), nil)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, headCommitID, headCommit)
	q.Stop()
}
