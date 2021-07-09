package goordinator

import (
	"fmt"
	"strings"

	go_github "github.com/google/go-github/v35/github"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider/github"
)

type pushEventRepoGetter interface {
	GetRepo() *go_github.PushEventRepository
}

type repoGetter interface {
	GetRepo() *go_github.Repository
}

type refGetter interface {
	GetRef() string
}

type pullRequestGetter interface {
	GetPullRequest() *go_github.PullRequest
}

// Event is processed by the event-loop and used for templating actions.
// It's public fields are accessible via template statements defined in the goordinater config file.
type Event struct {
	JSON     []byte
	Provider string

	// Github hook fields, if the value is not available they are empty
	// strings.
	DeliveryID      string
	EventType       string
	RepositoryOwner string
	Repository      string
	BaseBranch      string
	CommitID        string
	Branch          string
	// PullRequestNr is 0 if it's not available
	PullRequestNr int

	LogFields []zap.Field
}

func (e *Event) String() string {
	return fmt.Sprintf("%s/%s (deliveryID: %s)", e.Provider, e.EventType, e.DeliveryID)
}

func (e *Event) GetRepository() string {
	return e.Repository
}

func (e *Event) GetRepositoryOwner() string {
	return e.RepositoryOwner
}

func (e *Event) GetPullRequestNr() int {
	return e.PullRequestNr
}

func fromProviderEvent(event *github.Event) *Event {
	result := extractEventInfo(event.Event)
	result.JSON = event.JSON
	result.Provider = "github"
	result.DeliveryID = event.DeliveryID
	result.EventType = event.Type
	result.LogFields = eventlogFields(event, result)

	return result
}

func extractEventInfo(ghEvent interface{}) *Event {
	var result Event

	if v, ok := ghEvent.(pushEventRepoGetter); ok {
		if repo := v.GetRepo(); repo != nil {
			result.Repository = repo.GetName()
			result.RepositoryOwner = repo.GetOwner().GetLogin()
		}
	} else if v, ok := ghEvent.(repoGetter); ok {
		if repo := v.GetRepo(); repo != nil {
			result.Repository = repo.GetName()
			result.RepositoryOwner = repo.GetOwner().GetLogin()
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

			result.BaseBranch = pr.GetBase().GetRef()
		}
	}

	return &result
}

func eventlogFields(providerEvent *github.Event, ev *Event) []zap.Field {
	result := providerEvent.LogFields

	if ev.Repository != "" {
		result = append(result, logfields.Repository(ev.Repository))
	}

	if ev.RepositoryOwner != "" {
		result = append(result, logfields.RepositoryOwner(ev.RepositoryOwner))
	}

	if ev.BaseBranch != "" {
		result = append(result, logfields.BaseBranch(ev.BaseBranch))
	}

	if ev.CommitID != "" {
		result = append(result, zap.String("github.commit_id", ev.CommitID))
	}

	if ev.Branch != "" {
		result = append(result, zap.String("github.branch", ev.Branch))
	}

	if ev.PullRequestNr != 0 {
		result = append(result, logfields.PullRequest(ev.PullRequestNr))
	}

	if ev.Branch != "" {
		result = append(result, zap.String("github.branch", ev.Branch))
	}

	return result
}
