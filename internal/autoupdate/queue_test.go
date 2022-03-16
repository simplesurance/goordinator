package autoupdate

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/autoupdate/mocks"
	"github.com/simplesurance/goordinator/internal/goordinator"
)

func TestUpdatePR_DoesNotCallBaseBranchUpdateIfPRIsNotApproved(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	mockctrl := gomock.NewController(t)
	ghClient := mocks.NewMockGithubClient(mockctrl)

	bb, err := NewBaseBranch(repoOwner, repo, "main")
	require.NoError(t, err)
	q := newQueue(bb, zap.L(), ghClient, goordinator.NewRetryer())
	t.Cleanup(q.Stop)

	pr, err := NewPullRequest(1, "testbr", "fho", "test pr", "")
	require.NoError(t, err)

	_, added := q.active.EnqueueIfNotExist(pr.Number, pr)
	require.True(t, added)

	mockPullRequestIsApprovedCall(ghClient, pr.Number, false).Times(1)
	ghClient.EXPECT().UpdateBranch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	q.updatePR(context.Background(), pr)
}