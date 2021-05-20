package github

import (
	"net/http"
	"strings"

	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider"

	"github.com/google/go-github/v35/github"
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

func WithPayloadSecret(secret string) option { // nolint:golint // returning unexported field is fine here
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

func (p *Provider) HTTPHandler(resp http.ResponseWriter, req *http.Request) {
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

	ev := extractEventInfo(event)
	ev.JSON = payload
	ev.Provider = "github"
	ev.DeliveryID = deliveryID
	ev.EventType = hookType

	logger = logger.With(ev.LogFields()...)

	select {
	case p.c <- ev:
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

type pushEventRepoGetter interface {
	GetRepo() *github.PushEventRepository
}

type repoGetter interface {
	GetRepo() *github.Repository
}

type refGetter interface {
	GetRef() string
}

type pullRequestGetter interface {
	GetPullRequest() *github.PullRequest
}

func extractEventInfo(ghEvent interface{}) *provider.Event {
	var result provider.Event

	if v, ok := ghEvent.(pushEventRepoGetter); ok {
		if repo := v.GetRepo(); repo != nil {
			result.Repository = repo.GetName()

			if owner := repo.GetOwner(); owner != nil {
				result.RepositoryOwner = owner.GetLogin()
			}
		}

	} else if v, ok := ghEvent.(repoGetter); ok {
		if repo := v.GetRepo(); repo != nil {
			result.Repository = repo.GetName()

			if owner := repo.GetOwner(); owner != nil {
				result.RepositoryOwner = owner.GetLogin()
			}
		}
	}

	if v, ok := ghEvent.(refGetter); ok {
		ref := v.GetRef()
		if strings.HasPrefix(ref, "refs/heads/") {
			result.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}

	if v, ok := ghEvent.(pullRequestGetter); ok {
		if pr := v.GetPullRequest(); pr != nil {
			result.PullRequestNr = pr.GetNumber()

			if head := pr.GetHead(); head != nil {
				result.CommitID = head.GetSHA()
				// ref in PullRequestEvent contains **only**
				// the branch name without 'refs/heads/ prefix
				result.Branch = head.GetRef()
			}
		}
	}

	return &result
}
