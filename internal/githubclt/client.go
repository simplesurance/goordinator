// Package githubclt provides a github API client.
package githubclt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/shurcooL/githubv4"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/simplesurance/goordinator/internal/goorderr"
	"github.com/simplesurance/goordinator/internal/logfields"
)

const DefaultHTTPClientTimeout = time.Minute

const loggerName = "github_client"

var ErrPullRequestIsClosed = errors.New("pull request is closed")

// New returns a new github api client.
func New(oauthAPItoken string) *Client {
	httpClient := newHTTPClient(oauthAPItoken)
	return &Client{
		restClt:    github.NewClient(httpClient),
		graphQLClt: githubv4.NewClient(httpClient),
		logger:     zap.L().Named(loggerName),
	}
}

func newHTTPClient(apiToken string) *http.Client {
	if apiToken == "" {
		return &http.Client{
			Timeout: DefaultHTTPClientTimeout,
		}
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: apiToken},
	)

	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = DefaultHTTPClientTimeout

	return tc
}

// Client is an github API client.
// All methods return a goorderr.RetryableError when an operation can be retried.
// This can be e.g. the case when the API ratelimit is exceeded.
type Client struct {
	restClt    *github.Client
	graphQLClt *githubv4.Client
	logger     *zap.Logger
}

// BranchIsBehindBase returns true if branch is based on an old commit of baseBranch.
// If it is based on older commit, false is returned.
func (clt *Client) BranchIsBehindBase(ctx context.Context, owner, repo, baseBranch, branch string) (behind bool, err error) {
	cmp, _, err := clt.restClt.Repositories.CompareCommits(ctx, owner, repo, baseBranch, branch, &github.ListOptions{PerPage: 1})
	if err != nil {
		return false, clt.wrapRetryableErrors(err)
	}

	if cmp.BehindBy == nil {
		return false, goorderr.NewRetryableAnytimeError(errors.New("github returned a nil BehindBy field"))
	}

	return *cmp.BehindBy > 0, nil
}

// PullRequestIsUptodateWithBase returns true if the pull request is open and
// contains all changes from it's base branch.
// Additionally it returns the SHA of the head commit for which the status was
// checked.
// If the PR is closed PullRequestIsClosedError is returned.
func (clt *Client) PRIsUptodate(ctx context.Context, owner, repo string, pullRequestNumber int) (isUptodate bool, headSHA string, err error) {
	pr, _, err := clt.restClt.PullRequests.Get(ctx, owner, repo, pullRequestNumber)
	if err != nil {
		return false, "", clt.wrapRetryableErrors(err)
	}

	if pr.GetState() == "closed" {
		return false, "", ErrPullRequestIsClosed
	}

	prHead := pr.GetHead()
	if prHead == nil {
		return false, "", errors.New("got pull request object with empty head")
	}

	prHeadSHA := prHead.GetSHA()
	if prHeadSHA == "" {
		return false, "", errors.New("got pull request object with empty head sha")
	}

	if pr.GetMergeableState() == "behind" {
		return false, prHeadSHA, nil
	}

	prBranch := prHead.GetRef()
	if prBranch == "" {
		return false, "", errors.New("got pull request object with empty ref field")
	}

	base := pr.GetBase()
	if base == nil {
		return false, "", errors.New("got pull request object with empty base field")
	}

	baseBranch := base.GetRef()
	if baseBranch == "" {
		return false, "", errors.New("got pull request object with empty base ref field")
	}

	isBehind, err := clt.BranchIsBehindBase(ctx, owner, repo, baseBranch, prBranch)
	if err != nil {
		return false, "", fmt.Errorf("evaluating if branch is behind base failed: %w", err)
	}

	return !isBehind, prHeadSHA, nil
}

// CreateIssueComment creates a comment in a issue or pull request
func (clt *Client) CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error {
	_, _, err := clt.restClt.Issues.CreateComment(ctx, owner, repo, issueOrPRNr, &github.IssueComment{Body: &comment})
	return clt.wrapRetryableErrors(err)
}

// UpdateBranch schedules merging the base-branch into a pull request branch.
// If the PR contains all changes of it's base branch, false is returned for changed
// If it's not uptodate and updating the PR was scheduled at github, true is returned for changed and scheduled.
// If the PR was updated while the method was executed, a
// goorderr.RetryableError is returned and the operation can be retried.
// If the branch can not be updated automatically because of a merge conflict,
// an error is returned.
func (clt *Client) UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (changed, scheduled bool, err error) {
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
		return false, false, fmt.Errorf("evaluating if PR is uptodate with base branch failed: %w", err)
	}

	logger := clt.logger.With(
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
		logfields.PullRequest(pullRequestNumber),
		logfields.Commit(prHEADSHA),
	)

	if isUptodate {
		logger.Debug("branch is uptodate with base branch, skipping running update branch operation",
			logfields.Event("github_branch_uptodate_with_base"))
		return false, false, nil
	}

	_, _, err = clt.restClt.PullRequests.UpdateBranch(ctx, owner, repo, pullRequestNumber, &github.PullRequestBranchUpdateOptions{ExpectedHeadSHA: &prHEADSHA})
	if err != nil {
		if _, ok := err.(*github.AcceptedError); ok {
			// It is not clear if the response ensures that
			// the branch will be updated or if the
			// scheduled operation can fail.
			// If this should be the case, we could wait a
			// bit, iterate again and check if the PR is
			// now uptodate.
			logger.Debug("updating branch with base branch scheduled",
				logfields.Event("github_branch_update_with_base_scheduled"))
			return true, true, nil
		}

		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) {
			if respErr.Response.StatusCode == http.StatusUnprocessableEntity {
				if strings.Contains(respErr.Message, "merge conflict") {
					return false, false, fmt.Errorf("merge conflict: %w", respErr)
				}

				if strings.Contains(respErr.Message, "expected head sha didn’t match current head ref") {
					logger.Debug("branch changed while trying to sync with base branch",
						logfields.Event("github_branch_update_failed_ref_outdated"),
					)

					return false, false, goorderr.NewRetryableAnytimeError(err)
				}
			}
		}

		return false, false, clt.wrapRetryableErrors(err)
	}

	logger.Debug("branch was updated with base branch",
		logfields.Event("github_branch_update_with_base_triggered"))
	// github seems to always schedule update operations and return an
	// AcceptedError, this condition might never happened
	return true, false, nil
}

// AddLabel adds a label to Pull-Request or Issue.
func (clt *Client) AddLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error {
	if label == "" {
		// by default github removes all labels when none is provided,
		// we do not need this functionality, as safe guard fail if
		// because of a bug an empty label value is passed:
		return errors.New("provided label is empty")
	}
	_, _, err := clt.restClt.Issues.AddLabelsToIssue(ctx, owner, repo, pullRequestOrIssueNumber, []string{label})
	return clt.wrapRetryableErrors(err)
}

// RemoveLabel removes a label from a Pull-Request or issue.
// If the issue or PR does not have the label, the operation succeeds.
func (clt *Client) RemoveLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error {
	_, err := clt.restClt.Issues.RemoveLabelForIssue(
		ctx,
		owner,
		repo,
		pullRequestOrIssueNumber,
		label,
	)
	if err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) {
			if respErr.Response.StatusCode == http.StatusNotFound {
				clt.logger.Debug("removing label returned a not found response, interpreting it as success",
					logfields.RepositoryOwner(owner),
					logfields.Repository(repo),
					logfields.PullRequest(pullRequestOrIssueNumber),
					logfields.Label(label),
					logfields.Event("github_remove_label_returned_not_found"),
					zap.Error(err),
				)

				return nil
			}

			return clt.wrapGraphQLRetryableErrors(err)
		}
	}

	return nil
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

	prs, resp, err := it.clt.restClt.PullRequests.List(it.ctx, it.owner, it.repo, &github.PullRequestListOptions{
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

var graphQlHTTPStatusErrRe = regexp.MustCompile(`^non-200 OK status code: ([0-9]+) .*`)

func (clt *Client) wrapGraphQLRetryableErrors(err error) error {
	matches := graphQlHTTPStatusErrRe.FindStringSubmatch(err.Error())
	if len(matches) != 2 {
		return err
	}

	errcode, atoiErr := strconv.Atoi(matches[1])
	if atoiErr != nil {
		clt.logger.Info(
			"parsing http code from error string failed",
			zap.Error(atoiErr),
			zap.String("error_string", err.Error()),
			zap.String("http_errcode", matches[1]),
		)
		return err
	}

	if errcode >= 500 && errcode < 600 {
		return goorderr.NewRetryableAnytimeError(err)
	}

	return err
}
