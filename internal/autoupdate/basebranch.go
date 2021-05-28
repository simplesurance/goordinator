package autoupdate

import (
	"errors"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

// BranchID identifies a github branch
type BranchID struct {
	RepositoryOwner string
	Repository      string
	Branch          string
}

// BaseBranch contains all information to identify a github base branch uniquely.
type BaseBranch struct {
	BranchID
	Logfields []zap.Field
}

func NewBaseBranch(owner, repo, branch string) (*BaseBranch, error) {
	if owner == "" {
		return nil, errors.New("repositoryOwner is empty")
	}

	if repo == "" {
		return nil, errors.New("repository is empty")
	}

	if branch == "" {
		return nil, errors.New("branch is empty")
	}

	return &BaseBranch{
		BranchID: BranchID{
			RepositoryOwner: owner,
			Repository:      repo,
			Branch:          branch,
		},
		Logfields: []zap.Field{
			logfields.Repository(repo),
			logfields.RepositoryOwner(owner),
			logfields.BaseBranch(branch),
		},
	}, nil
}
