package goordinator

import "fmt"

// MatchResult represents the result matching a rule against an event.
type MatchResult uint8

const (
	MatchResultUndefined MatchResult = 0
	EventSourceMismatch              = iota
	RuleMismatch
	Match
)

var matchResulString = [...]string{
	MatchResultUndefined: "undefined",
	EventSourceMismatch:  "event source mismatch",
	RuleMismatch:         "condition mismatch",
	Match:                "rule matches",
}

func (m *MatchResult) String() string {
	// it can not be <0 because it's type is uint8
	if int(*m) > len(matchResulString)-1 {
		return fmt.Sprintf("unsupported MatchResult value: %d", m)
	}

	return matchResulString[*m]
}
