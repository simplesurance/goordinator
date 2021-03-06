// Package githubclt provides a github API client.
package githubclt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/go-github/v35/github"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/goorderr"
	"github.com/simplesurance/goordinator/internal/logfields"
)

const DefaultHTTPClientTimeout = time.Minute

const loggerName = "github_client"

var ErrPullRequestIsClosed = errors.New("pull request is closed")

// New returns a new github api client.
func New(oauthAPItoken string) *Client {
	return &Client{
		clt:    newGHClient(oauthAPItoken),
		logger: zap.L().Named(loggerName),
	}
}

func newGHClient(apiToken string) *github.Client {
	if apiToken == "" {
		return github.NewClient(&http.Client{
			Timeout: DefaultHTTPClientTimeout,
		})
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: apiToken},
	)

	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = DefaultHTTPClientTimeout

	return github.NewClient(tc)
}

// Client is an github API client.
// All methods return a goorderr.RetryableError when an operation can be retried.
// This can be e.g. the case when the API ratelimit is exceeded.
type Client struct {
	clt    *github.Client
	logger *zap.Logger
}

func (clt *Client) BranchHeadCommitSHA(ctx context.Context, owner, repo string, branch string) (string, error) {
	resp, _, err := clt.clt.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         branch,
		ListOptions: github.ListOptions{Page: 1, PerPage: 1},
	})

	if err != nil {
		return "", clt.wrapRetryableErrors(err)
	}

	if len(resp) != 1 {
		return "", fmt.Errorf("expected 1 response, got %d", len(resp))
	}

	return resp[0].GetSHA(), nil
}

const (
	StatusError   = "error"
	StatusFailure = "failure"
	StatusPending = "pending"
	StatusSuccess = "success"
)

// CombinedStatus returns the combined check status and the last time the status changed for the ref.
func (clt *Client) CombinedStatus(ctx context.Context, owner, repo, ref string) (string, time.Time, error) {
	var lastChange time.Time

	status, _, err := clt.clt.Repositories.GetCombinedStatus(ctx, owner, repo, ref, nil)
	if err != nil {
		return "", lastChange, clt.wrapRetryableErrors(err)
	}

	for _, s := range status.Statuses {
		if s.GetUpdatedAt().After(lastChange) {
			lastChange = s.GetUpdatedAt()
		}
	}

	switch s := status.GetState(); s {
	case StatusSuccess, StatusPending, StatusFailure:
		return s, lastChange, nil

	default:
		return "", lastChange, fmt.Errorf("github status api returned unsupported status: %q", s)
	}
}

// PullRequestIsUptodateWithBase returns true if the pull request is open and
// contains all changes from it's base branch.
// Additionally it returns the SHA of the head commit for which the status was
// checked.
// If the PR is closed PullRequestIsClosedError is returned.
func (clt *Client) PRIsUptodate(ctx context.Context, owner, repo string, pullRequestNumber int) (isUptodate bool, headSHA string, err error) {
	pr, _, err := clt.clt.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
	if err != nil {
		return false, "", clt.wrapRetryableErrors(err)
	}

	if pr.GetState() == "closed" {
		return false, "", ErrPullRequestIsClosed
	}

	base := pr.GetBase()
	if base == nil {
		return false, "", errors.New("got pull request object with empty base")
	}

	baseBranch := base.GetRef()
	PRbaseSHA := base.GetSHA()

	baseBranchHEADSHA, err := clt.BranchHeadCommitSHA(ctx, owner, repo, baseBranch)
	if err != nil {
		return false, "", fmt.Errorf("could not retrieve head SHA of base branch %q: %w", baseBranch, err)
	}

	head := pr.GetHead()
	if head == nil {
		return false, "", errors.New("got pull request object with empty head")
	}

	return PRbaseSHA == baseBranchHEADSHA, head.GetSHA(), nil
}

// CreateIssueComment creates a comment in a issue or pull request
func (clt *Client) CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error {
	_, _, err := clt.clt.Issues.CreateComment(ctx, owner, repo, issueOrPRNr, &github.IssueComment{Body: &comment})
	return clt.wrapRetryableErrors(err)
}

// UpdateBranch schedules merging the base-branch into a pull request branch.
// If the PR contains all changes of it's base branch, false is returned.
// If it's not uptodate and updating the PR was scheduled at github, true is returned.
// If the PR was updated while the method was executed, a
// goorderr.RetryableError is returned and the operation can be retried.
// If the branch can not be updated automatically because of a merge conflict, an error is returned.
func (clt *Client) UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (bool, error) {
	// 1. Get Commit of PR
	// 2. Check if it is uptodate
	// 3. If not -> Update, specify commit as HEAD branch
	// 4. "expected head sha didn’t match current head ref" -> try again

	// If UpdateBranch is called and the branch is already
	// uptodate, github creates an empty merge commit and changes
	// the branch. Therefore we have to check first if an update is
	// needed.
	isUptodate, prHEADSHA, err := clt.PRIsUptodate(ctx, owner, repo, pullRequestNumber)
	if err != nil {
		return false, fmt.Errorf("evaluating if PR is uptodate with base branch failed: %w", err)
	}

	logger := clt.logger.With(
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
		logfields.PullRequest(pullRequestNumber),
		logfields.Commit(prHEADSHA),
	)

	if isUptodate {
		return false, nil
	}

	_, _, err = clt.clt.PullRequests.UpdateBranch(ctx, owner, repo, pullRequestNumber, &github.PullRequestBranchUpdateOptions{ExpectedHeadSHA: &prHEADSHA})
	if err != nil {
		if _, ok := err.(*github.AcceptedError); ok {
			// It is not clear if the response ensures that
			// the branch will be updated or if the
			// scheduled operation can fail.
			// If this should be the case, we could wait a
			// bit, iterate again and check if the PR is
			// now uptodate.
			return true, nil
		}

		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) {
			if respErr.Response.StatusCode == http.StatusUnprocessableEntity {
				if strings.Contains(respErr.Message, "merge conflict") {
					return false, fmt.Errorf("merge conflict: %w", respErr)
				}

				if strings.Contains(respErr.Message, "expected head sha didn’t match current head ref") {
					logger.Debug("branch changed while trying to sync with base branch",
						logfields.Event("github_branch_update_failed_ref_outdated"),
					)

					return false, goorderr.NewRetryableAnytimeError(err)
				}
			}
		}

		return false, clt.wrapRetryableErrors(err)
	}

	// github seems to always schedule update operations and return an
	// AcceptedError, this condition might never happened
	return true, nil
}

type PRIterator interface {
	Next() (*github.PullRequest, error)
}

type PRIter struct {
	clt *Client

	ctx   context.Context
	owner string
	repo  string

	filterState   string
	sortOrder     string
	sortDirection string

	unseen []*github.PullRequest

	nextPage int
	finished bool
}

// Next returns the next pullRequest.
// When the last result was returned a nil PullRequest is returned.
func (it *PRIter) Next() (*github.PullRequest, error) {
	if len(it.unseen) > 0 {
		result := it.unseen[0]
		it.unseen = it.unseen[1:]

		return result, nil
	}

	if it.finished {
		return nil, nil
	}

	prs, resp, err := it.clt.clt.PullRequests.List(it.ctx, it.owner, it.repo, &github.PullRequestListOptions{
		State:     "open",
		Sort:      it.filterState,
		Direction: it.sortOrder,
		ListOptions: github.ListOptions{
			Page:    it.nextPage,
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, it.clt.wrapRetryableErrors(err)
	}

	if resp.NextPage == 0 || resp.PrevPage+1 == resp.LastPage || len(prs) == 0 {
		it.finished = true
	} else {
		it.nextPage = resp.NextPage
	}

	it.unseen = prs

	return it.Next()
}

// ListPullRequests returns an iterator for receiving all pull requests.
// The parameters state, sort, sortDirection expect the same values then their pendants in the struct github.PullRequestListOptions.
// all pull requests should be returned.
func (clt *Client) ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) PRIterator { // interface is returned to make the method mockable
	return &PRIter{
		clt:           clt,
		ctx:           ctx,
		owner:         owner,
		repo:          repo,
		sortOrder:     sort,
		sortDirection: sortDirection,
		filterState:   state,
		nextPage:      1,
	}
}

func (clt *Client) wrapRetryableErrors(err error) error {
	switch v := err.(type) {
	case *github.RateLimitError:
		clt.logger.Info(
			"rate limit exceeded",
			logfields.Event("github_api_rate_limit_exceeded"),
			zap.Int("github_api_rate_limit", v.Rate.Limit),
			zap.Int("github_api_rate_limit", v.Rate.Limit),
			zap.Time("github_api_rate_limit_reset_time", v.Rate.Reset.Time),
		)

		return goorderr.NewRetryableError(err, v.Rate.Reset.Time)

	case *github.ErrorResponse:
		if v.Response.StatusCode >= 500 && v.Response.StatusCode < 600 {
			return goorderr.NewRetryableAnytimeError(err)
		}
	}

	return err
}
