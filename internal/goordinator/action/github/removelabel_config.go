package github

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/goordinator/action"
	"github.com/simplesurance/goordinator/internal/maputils"
)

type RemoveLabelConfig struct {
	clt    *githubclt.Client
	label  string
	logger *zap.Logger
}

func NewRemoveLabelConfigFromMap(clt *githubclt.Client, m map[string]any) (*RemoveLabelConfig, error) {
	const loggerName = "action.github.remove_label"

	label, err := maputils.StrVal(m, "label")
	if err != nil {
		return nil, err
	}

	if label == "" {
		return nil, errors.New("label is not set")
	}

	return &RemoveLabelConfig{
		clt:    clt,
		label:  label,
		logger: zap.L().Named(loggerName),
	}, nil
}

func (c *RemoveLabelConfig) Render(ev action.Event, renderFunc func(string) (string, error)) (action.Runner, error) {
	var err error
	newConfig := *c

	if ev.GetRepository() == "" {
		return nil, errors.New("repository for event is unknown")
	}

	if ev.GetRepositoryOwner() == "" {
		return nil, errors.New("repository owner for event is unknown")
	}

	if ev.GetPullRequestNr() <= 0 {
		return nil, fmt.Errorf("pull request number for event is unknown (%d <=0)", ev.GetPullRequestNr())
	}

	newConfig.label, err = renderFunc(newConfig.label)
	if err != nil {
		return nil, fmt.Errorf("templating label failed: %w", err)
	}

	return &RemoveLabelRunner{
		RemoveLabelConfig: &newConfig,
		repository:        ev.GetRepository(),
		repositoryOwner:   ev.GetRepositoryOwner(),
		pullRequestNumber: ev.GetPullRequestNr(),
	}, nil
}

func (c *RemoveLabelConfig) String() string {
	return "github remove label"
}

func (c *RemoveLabelConfig) DetailedString() string {
	return c.String()
}
