package github

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/action"
	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/provider"
)

// Config is the configuration of a github action
type UpdateBranchConfig struct {
	logger *zap.Logger

	clt *githubclt.Client
}

func NewUpdateBranchConfig(clt *githubclt.Client) *UpdateBranchConfig {
	const loggerName = "action.github.update_branch"

	return &UpdateBranchConfig{
		clt:    clt,
		logger: zap.L().Named(loggerName),
	}
}

func (c *UpdateBranchConfig) Template(ev *provider.Event, _ func(string) (string, error)) (action.Runner, error) {
	if ev.Repository == "" {
		return nil, errors.New("repository for event is unknown")
	}

	if ev.RepositoryOwner == "" {
		return nil, errors.New("repository owner for event is unknown")
	}

	if ev.PullRequestNr <= 0 {
		return nil, fmt.Errorf("pull request number for event is unknown (%d <=0)", ev.PullRequestNr)
	}

	return &UpdateRunner{
		UpdateBranchConfig: c,
		repository:         ev.Repository,
		repositoryOwner:    ev.RepositoryOwner,
		pullRequestNumber:  ev.PullRequestNr,
	}, nil
}

func (c *UpdateBranchConfig) String() string {
	return "github update base branch"
}

func (c *UpdateBranchConfig) DetailedString() string {
	return c.String()
}
