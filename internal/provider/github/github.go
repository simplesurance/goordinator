package github

import (
	"encoding/json"
	"net/http"

	"github.com/google/go-github/v33/github"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider"
	"go.uber.org/zap"
)

const loggerName = "github-event-provider"

// Provider listens for github-webhook http-requests at a http-server handler,
// validates and converts the requests to an Events and forwards it to an event
// channel.
type Provider struct {
	logging       *zap.Logger
	webhookSecret []byte
	c             chan<- *provider.Event
}

type option func(*Provider)

func WithPayloadSecret(secret string) option {
	return func(p *Provider) {
		p.webhookSecret = []byte(secret)
	}
}

func New(eventChan chan<- *provider.Event, opts ...option) *Provider {
	p := Provider{
		c: eventChan,
	}

	for _, o := range opts {
		o(&p)
	}

	if p.logging == nil {
		p.logging = zap.L().Named(loggerName)
	}

	return &p
}

func (p *Provider) HttpHandler(resp http.ResponseWriter, req *http.Request) {
	p.logging.Debug("received a http request", logfields.Event("github_event_received"))

	deliveryID := github.DeliveryID(req)
	hookType := github.WebHookType(req)

	logFields := []zap.Field{
		logfields.EventProvider("github"),
		zap.String("github.delivery_id", deliveryID),
		zap.String("github.webhook_type", hookType),
	}

	logger := p.logging.With(logFields...)

	payload, err := github.ValidatePayload(req, p.webhookSecret)
	if err != nil {
		logger.Info(
			"received invalid http request, payload validation failed",
			logfields.Event("github_http_request_validation_failed"),
			zap.Error(err),
		)
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return
	}

	logger.Debug(
		"received http request",
		logfields.Event("github_event_received"),
		zap.ByteString("http_body", payload),
	)

	event, err := github.ParseWebHook(github.WebHookType(req), payload)
	if err != nil {
		logger.Info(
			"received invalid http request, parsing failed",
			logfields.Event("github_event_parsing_failed"),
			zap.Error(err),
		)
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		logger.Error(
			"could not marshal event into json",
			logfields.Event("github_json_event_marshalling_failed"),
			zap.Error(err),
		)
	}

	ev := provider.Event{
		Json:       eventJSON,
		Provider:   "github",
		LogFields:  logFields,
		DeliveryID: deliveryID,
		EventType:  hookType,
	}

	switch event := event.(type) {
	case *github.PullRequestEvent:
		if repo := event.GetRepo(); repo != nil {
			ev.Repository = repo.GetName()
		}

		if pr := event.GetPullRequest(); pr != nil {
			ev.PullRequestNr = pr.GetNumber()

			if hb := pr.GetHead(); hb != nil {
				ev.CommitID = hb.GetSHA()
				ev.Branch = hb.GetRef()

			}

			logFields = append(
				logFields,
				zap.Int("github.pull_request_nr", ev.PullRequestNr),
				zap.String("github.commit_id", ev.CommitID),
				zap.String("github.branch", ev.Branch),
			)

			logger = logger.With(logFields...)
		}

	default:
		logger.Info("ignoring event, event type is unsupported",
			logfields.Event("github_unsupported_event_received"),
		)

	}

	select {
	case p.c <- &ev:
		logger.Debug("event forwarded to channel",
			logfields.Event("github_event_forwarded"),
		)

	default:
		logger.Warn(
			"event lost, forwarding event to channel failed",
			zap.String("error", "could not forward event to channel, send would have blocked"),
			logfields.Event("github_forwarding_event_failed"),
		)

		http.Error(resp, "queue full", http.StatusServiceUnavailable)
		return
	}
}
