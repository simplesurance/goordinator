// Package githubclt provides a github API client.
package githubclt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v59/github"
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
		restClt: github.NewClient(httpClient),
		logger:  zap.L().Named(loggerName),
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
	restClt *github.Client
	logger  *zap.Logger
}

type UpdateBranchResult struct {
	Changed      bool
	Scheduled    bool
	HeadCommitID string
}

// BranchIsBehindBase returns true if the head reference contains all changes of base.
func (clt *Client) BranchIsBehindBase(ctx context.Context, owner, repo, base, head string) (behind bool, err error) {
	cmp, _, err := clt.restClt.Repositories.CompareCommits(ctx, owner, repo, base, head, &github.ListOptions{PerPage: 1})
	if err != nil {
		return false, clt.wrapRetryableErrors(err)
	}

	if cmp.BehindBy == nil {
		return false, goorderr.NewRetryableAnytimeError(errors.New("github returned a nil BehindBy field"))
	}

	return *cmp.BehindBy > 0, nil
}

// UpdateBranch schedules merging the base-branch into a pull request branch.
// If the PR contains all changes of it's base branch, false is returned for changed
// If it's not up-to-date and updating the PR was scheduled at github, true is returned for changed and scheduled.
// If the PR was updated while the method was executed, a
// goorderr.RetryableError is returned and the operation can be retried.
// If the branch can not be updated automatically because of a merge conflict,
// an error is returned.
func (clt *Client) UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (*UpdateBranchResult, error) {
	// 1. Get Commit of PR
	// 2. Check if it is up-to-date
	// 3. If not -> Update, specify commit as HEAD branch
	// 4. "expected head sha didn’t match current head ref" -> try again

	// If UpdateBranch is called and the branch is already
	// up-to-date, github creates an empty merge commit and changes
	// the branch. Therefore we have to check first if an update is
	// needed.
	isUptodate, prHEADSHA, err := clt.PRIsUptodate(ctx, owner, repo, pullRequestNumber)
	if err != nil {
		return nil, fmt.Errorf("evaluating if PR is up-to-date with base branch failed: %w", err)
	}

	logger := clt.logger.With(
		logfields.RepositoryOwner(owner),
		logfields.Repository(repo),
		logfields.PullRequest(pullRequestNumber),
		logfields.Commit(prHEADSHA),
	)

	if isUptodate {
		logger.Debug("branch is up-to-date with base branch, skipping running update branch operation",
			logfields.Event("github_branch_uptodate_with_base"))
		return &UpdateBranchResult{HeadCommitID: prHEADSHA}, nil
	}

	_, _, err = clt.restClt.PullRequests.UpdateBranch(ctx, owner, repo, pullRequestNumber, &github.PullRequestBranchUpdateOptions{ExpectedHeadSHA: &prHEADSHA})
	if err != nil {
		if _, ok := err.(*github.AcceptedError); ok { // nolint:errorlint // errors.As not needed here
			// It is not clear if the response ensures that the
			// branch will be updated or if the scheduled operation
			// can fail.
			logger.Debug("updating branch with base branch scheduled",
				logfields.Event("github_branch_update_with_base_scheduled"))
			return &UpdateBranchResult{Scheduled: true, Changed: true}, nil
		}

		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) {
			if respErr.Response.StatusCode == http.StatusUnprocessableEntity {
				if strings.Contains(respErr.Message, "merge conflict") {
					return nil, fmt.Errorf("merge conflict: %w", respErr)
				}

				if strings.Contains(respErr.Message, "expected head sha didn’t match current head ref") {
					logger.Debug("branch changed while trying to sync with base branch",
						logfields.Event("github_branch_update_failed_ref_outdated"),
					)

					return nil, goorderr.NewRetryableAnytimeError(err)
				}
			}
		}

		return nil, clt.wrapRetryableErrors(err)
	}

	logger.Debug("branch was updated with base branch",
		logfields.Event("github_branch_update_with_base_triggered"))
	// github seems to always schedule update operations and return an
	// AcceptedError, this condition might never happened
	return &UpdateBranchResult{Changed: true, HeadCommitID: prHEADSHA}, nil
}

// PRIsUptodate returns true if the pull request is open and
// contains all changes from it's base branch.
// Additionally it returns the SHA of the head commit for which the status was
// checked.
// If the PR is closed ErrPullRequestIsClosed is returned.
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

	isBehind, err := clt.BranchIsBehindBase(ctx, owner, repo, baseBranch, prHeadSHA)
	if err != nil {
		return false, "", fmt.Errorf("evaluating if branch is behind base failed: %w", err)
	}

	return !isBehind, prHeadSHA, nil
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

			return clt.wrapRetryableErrors(err)
		}
	}

	return nil
}

func (clt *Client) wrapRetryableErrors(err error) error {
	switch v := err.(type) { // nolint:errorlint // errors.As not needed here
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
