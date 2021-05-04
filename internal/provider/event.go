package provider

import (
	"fmt"

	"go.uber.org/zap"
)

type Event struct {
	LogFields []zap.Field
	JSON      []byte
	Provider  string

	// Github hook fields, if the value is not available they are empty
	// strings.
	DeliveryID      string
	EventType       string
	Repository      string
	CommitID        string
	Branch          string
	PullRequestName string
	// PullRequestNr is 0 if it's not available
	PullRequestNr int
}

func (e *Event) String() string {
	return fmt.Sprintf("%s (deliveryID: %s)", e.EventType, e.DeliveryID)
}
