package autoupdate

import (
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type PullRequest struct {
	Number    int
	Branch    string
	LogFields []zap.Field

	// lastStatusUpdate is the the last time a github status for the PR changed.
	// It is the zero timestamp if it was never retrieved before.
	lastStatusUpdate time.Time

	enqueueTs time.Time
}

func NewPullRequest(nr int, branch string) (*PullRequest, error) {
	if nr <= 0 {
		return nil, fmt.Errorf("number is %d, must be >0", nr)
	}

	if branch == "" {
		return nil, errors.New("branch is empty")
	}

	return &PullRequest{
		Number: nr,
		Branch: branch,
		LogFields: []zap.Field{
			logfields.PullRequest(nr),
			logfields.Branch(branch),
		},
	}, nil
}

func (p *PullRequest) Equal(other interface{}) bool {
	p1, ok := other.(*PullRequest)
	if !ok {
		return false
	}

	return p.Number == p1.Number
}
