package github

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/goordinator/action"
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

func (c *UpdateBranchConfig) Template(ev action.Event, _ func(string) (string, error)) (action.Runner, error) {
	if ev.GetRepository() == "" {
		return nil, errors.New("repository for event is unknown")
	}

	if ev.GetRepositoryOwner() == "" {
		return nil, errors.New("repository owner for event is unknown")
	}

	if ev.GetPullRequestNr() <= 0 {
		return nil, fmt.Errorf("pull request number for event is unknown (%d <=0)", ev.GetPullRequestNr())
	}

	return &UpdateRunner{
		UpdateBranchConfig: c,
		repository:         ev.GetRepository(),
		repositoryOwner:    ev.GetRepositoryOwner(),
		pullRequestNumber:  ev.GetPullRequestNr(),
	}, nil
}

func (c *UpdateBranchConfig) String() string {
	return "github update base branch"
}

func (c *UpdateBranchConfig) DetailedString() string {
	return c.String()
}
