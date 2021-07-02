package autoupdate

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type PRSpecifierType uint8

const (
	PRSpecifierTypeUndefined PRSpecifierType = iota
	PullRequestNumber
	PullRequestBranch
)

type PRSpecifier interface {
	Type() PRSpecifierType
	String() string
	LogField() zap.Field
}

type PRBranch struct {
	BranchName string
}

func (p *PRBranch) Type() PRSpecifierType {
	return PullRequestBranch
}

func (p *PRBranch) String() string {
	return p.BranchName
}

func (p *PRBranch) LogField() zap.Field {
	return logfields.Branch(p.BranchName)
}

type PRNumber struct {
	Number int
}

func (p *PRNumber) Type() PRSpecifierType {
	return PullRequestNumber
}

func (p *PRNumber) String() string {
	return fmt.Sprint(p.Number)
}

func (p *PRNumber) LogField() zap.Field {
	return logfields.PullRequest(p.Number)
}
