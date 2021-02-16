package goordinator

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
	return matchResulString[*m]
}
