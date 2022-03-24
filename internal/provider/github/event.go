package github

import "go.uber.org/zap"

// Event is the preprocessed Github Webhook event
type Event struct {
	// DeliveryID is the unique github ID of the event
	DeliveryID string
	// Type is the github webhook event type returned by github.WebHookType()
	Type string
	// JSON is the event payload as JSON
	JSON []byte
	// Event is the parsed JSON payload as struct type returned by github.ParseWebHook()
	Event     any
	LogFields []zap.Field
}
