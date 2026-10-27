package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

func TestRemoveLabelRunnerLogFields(t *testing.T) {
	runner := &RemoveLabelRunner{
		RemoveLabelConfig: &RemoveLabelConfig{
			label: "merge",
		},
		repository:        "repo",
		repositoryOwner:   "owner",
		pullRequestNumber: 42,
	}

	assert.Equal(t, []zap.Field{
		zap.String("action", "github.remove_label"),
		logfields.Label("merge"),
	}, runner.LogFields())
}

func TestUpdateRunnerLogFields(t *testing.T) {
	runner := &UpdateRunner{
		UpdateBranchConfig: &UpdateBranchConfig{},
		repository:         "repo",
		repositoryOwner:    "owner",
		pullRequestNumber:  42,
	}

	assert.Equal(t, []zap.Field{
		zap.String("action", "github.update_base_branch"),
	}, runner.LogFields())
}
