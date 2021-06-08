package autoupdate

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/githubclt"
)

// DryGithubClient is a github-client that does not do any changes on github.
// All operations that could cause a change are simluated and always succed.
// All all other operations are forwarded to wrapped github-client.
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

func (c *DryGithubClient) UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (bool, error) {
	c.logger.Info("simulated updating of github branch, returning is uptodate")
	return false, nil
}

func (c *DryGithubClient) CombinedStatus(ctx context.Context, owner, repo, ref string) (string, time.Time, error) {
	return c.clt.CombinedStatus(ctx, owner, repo, ref)
}

func (c *DryGithubClient) CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error {
	c.logger.Info("simulated creating of github issue comment, no comment created on github")
	return nil
}

func (c *DryGithubClient) ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) githubclt.PRIterator {
	return c.clt.ListPullRequests(ctx, owner, repo, state, sort, sortDirection)
}
