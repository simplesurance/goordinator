package autoupdate

import (
	"context"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/githubclt"
)

// DryGithubClient is a github-client that does not do any changes on github.
// All operations that could cause a change are simulated and always succeed.
// All all other operations are forwarded to a wrapped GithubClient.
type DryGithubClient struct {
	clt    GithubClient
	logger *zap.Logger
}

func NewDryGithubClient(clt GithubClient, logger *zap.Logger) *DryGithubClient {
	return &DryGithubClient{
		clt:    clt,
		logger: logger.Named("dry_github_client"),
	}
}

func (c *DryGithubClient) UpdateBranch(context.Context, string, string, int) (bool, bool, error) {
	c.logger.Info("simulated updating of github branch, returning is uptodate")
	return false, false, nil
}

func (c *DryGithubClient) ReadyForMerge(context.Context, string, string, int) (*githubclt.ReadyForMergeStatus, error) {
	c.logger.Info("simulated fetching ready for merge status, pr is approved, all checks successful")

	return &githubclt.ReadyForMergeStatus{
		ReviewDecision: githubclt.ReviewDecisionApproved,
		CIStatus:       githubclt.CIStatusSuccess,
	}, nil
}

func (c *DryGithubClient) CreateIssueComment(context.Context, string, string, int, string) error {
	c.logger.Info("simulated creating of github issue comment, no comment created on github")
	return nil
}

func (c *DryGithubClient) ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) githubclt.PRIterator {
	return c.clt.ListPullRequests(ctx, owner, repo, state, sort, sortDirection)
}

func (*DryGithubClient) RemoveLabel(context.Context, string, string, int, string) error {
	return nil
}
