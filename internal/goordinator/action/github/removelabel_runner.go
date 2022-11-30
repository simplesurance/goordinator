package github

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type RemoveLabelRunner struct {
	*RemoveLabelConfig
	repository        string
	repositoryOwner   string
	pullRequestNumber int
}

func (r *RemoveLabelRunner) LogFields() []zap.Field {
	return []zap.Field{
		zap.String("action", "github.remove_label"),
		logfields.Repository(r.repository),
		logfields.RepositoryOwner(r.repositoryOwner),
		logfields.PullRequest(r.pullRequestNumber),
	}
}

func (r *RemoveLabelRunner) Run(ctx context.Context) error {
	return r.clt.RemoveLabel(ctx, r.repositoryOwner, r.repository, r.pullRequestNumber, r.label)
}

func (r *RemoveLabelRunner) String() string {
	return fmt.Sprintf(
		"github remove label: repo: %s/%s, pull request: #%d, label: %s",
		r.repository, r.repositoryOwner, r.pullRequestNumber, r.label,
	)
}
