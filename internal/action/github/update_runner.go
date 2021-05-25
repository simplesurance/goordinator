package github

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type UpdateRunner struct {
	*UpdateBranchConfig

	repository        string
	repositoryOwner   string
	pullRequestNumber int
}

func (r *UpdateRunner) Run(ctx context.Context) error {
	changed, err := r.clt.UpdateBranch(ctx, r.repositoryOwner, r.repository, r.pullRequestNumber)
	if err != nil {
		return err
	}

	if changed {
		r.logger.Info("github branch updates with base branch")
	} else {
		r.logger.Info("github branch already uptodate with base branch")
	}

	return nil
}

func (r *UpdateRunner) LogFields() []zap.Field {
	return []zap.Field{
		zap.String("action", "github.update_base_branch"),
		logfields.Repository(r.repository),
		logfields.RepositoryOwner(r.repositoryOwner),
		logfields.PullRequest(r.pullRequestNumber),
	}
}

func (r *UpdateRunner) String() string {
	return fmt.Sprintf("github update base branch: repo: %s/%s, pull-request: #%d", r.repository, r.repositoryOwner, r.pullRequestNumber)
}
