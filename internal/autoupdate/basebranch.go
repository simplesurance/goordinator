package autoupdate

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

// BranchID identifies a github branch uniquely
type BranchID struct {
	RepositoryOwner string
	Repository      string
	Branch          string
}

func (b *BranchID) String() string {
	return fmt.Sprintf("%s/%s branch: %s", b.RepositoryOwner, b.Repository, b.Branch)
}

// BaseBranch represents a base branch of a pull request
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

func (b *BaseBranch) String() string {
	return b.BranchID.String()
}
