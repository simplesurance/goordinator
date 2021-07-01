package autoupdate

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type PullRequest struct {
	Number    int
	Branch    string
	LogFields []zap.Field

	enqueuedSince time.Time

	stateUnchangedSince time.Time
	lock                sync.Mutex // must be held when accessing stateUnchangedSince
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

// Equal returns true if p and other are of type PullRequest and it's Number
// field contain the same value.
func (p *PullRequest) Equal(other interface{}) bool {
	p1, ok := other.(*PullRequest)
	if !ok {
		return false
	}

	return p.Number == p1.Number
}

func (p *PullRequest) GetStateUnchangedSince() time.Time {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.stateUnchangedSince
}

func (p *PullRequest) SetStateUnchangedSinceIfNewer(t time.Time) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.stateUnchangedSince.Before(t) {
		p.stateUnchangedSince = t
	}
}

func (p *PullRequest) SetStateUnchangedSince(t time.Time) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.stateUnchangedSince = t
}

func (p *PullRequest) SetStateUnchangedSinceIfZero(t time.Time) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.stateUnchangedSince.IsZero() {
		p.stateUnchangedSince = t
	}
}
