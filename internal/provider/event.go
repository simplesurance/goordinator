package provider

import (
	"fmt"

	"go.uber.org/zap"
)

type Event struct {
	JSON     []byte
	Provider string

	// Github hook fields, if the value is not available they are empty
	// strings.
	DeliveryID string
	EventType  string
	Repository string
	CommitID   string
	Branch     string
	// PullRequestNr is 0 if it's not available
	PullRequestNr int
}

func (e *Event) String() string {
	return fmt.Sprintf("%s (deliveryID: %s)", e.EventType, e.DeliveryID)
}

func (e *Event) LogFields() []zap.Field {
	fields := make([]zap.Field, 0, 6) // cap == max. size of fields we append

	if e.DeliveryID != "" {
		fields = append(fields, zap.String("github.delivery_id", e.DeliveryID))
	}

	// EventType is not added as logfield, information is not needed

	if e.Repository != "" {
		fields = append(fields, zap.String("github.repository", e.Repository))
	}

	if e.CommitID != "" {
		fields = append(fields, zap.String("github.commit_id", e.CommitID))
	}

	if e.Branch != "" {
		fields = append(fields, zap.String("github.branch", e.Branch))
	}

	if e.PullRequestNr != 0 {
		fields = append(fields, zap.Int("github.pull_request_nr", e.PullRequestNr))
	}

	if e.Branch != "" {
		fields = append(fields, zap.String("github.branch", e.Branch))
	}

	return fields
}
