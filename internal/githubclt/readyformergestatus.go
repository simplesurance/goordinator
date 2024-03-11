package githubclt

import (
	"context"
	"fmt"

	"github.com/shurcooL/githubv4"
)

// CIStatus abstracts the multiple result values of GitHub check runs and
// Commit statuses into a single value.
type CIStatus string

const (
	CIStatusSuccess CIStatus = "SUCCESS"
	CIStatusPending CIStatus = "PENDING"
	CIStatusFailure CIStatus = "FAILURE"
)

// ReviewDecision is the result of a pull request review.
type ReviewDecision string

const (
	ReviewDecisionApproved         = ReviewDecision(githubv4.PullRequestReviewDecisionApproved)
	ReviewDecisionChangesRequested = ReviewDecision(githubv4.PullRequestReviewDecisionChangesRequested)
	ReviewDecisionReviewRequired   = ReviewDecision(githubv4.PullRequestReviewDecisionReviewRequired)
)

// CIJobStatus is the status of a CI job.
// It represents the status of GitHub CheckRuns and Commit statuses.
type CIJobStatus struct {
	Name     string
	Status   CIStatus
	Required bool
}

// ReadyForMergeStatus represent the information deciding if a pull request is
// ready to be merged. It contains the review decision and  CI job results.
type ReadyForMergeStatus struct {
	ReviewDecision ReviewDecision
	CIStatus       CIStatus
	Statuses       []*CIJobStatus
	Commit         string
}

// ReadyForMerge returns the [review decision] and [status check rollup] for a PR.
//
// The returned [ReadyForMergeStatus.CIStatus] is [CIStatusPending], if one or
// more checks or status are in pending state.
// It is [CIStatusSuccess], if no check or status is in pending state and all
// required ones succeeded.
// If a required check or status is in failed state the CIStatus is
// [CIStatusFailure].
//
// [status check rollup]: https://docs.github.com/en/graphql/reference/objects#statuscheckrollup
// [review decision]: https://docs.github.com/en/graphql/reference/enums#pullrequestreviewdecision
func (clt *Client) ReadyForMerge(ctx context.Context, owner, repo string, prNumber int) (*ReadyForMergeStatus, error) {
	queryResult, err := clt.reviewAndCIStatus(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, clt.wrapGraphQLRetryableErrors(err)
	}

	statuses, err := toCIJobStatuses(queryResult.RequiredStatusCheckContexts, queryResult.CheckRuns, queryResult.StatusContext)
	if err != nil {
		return nil, err
	}
	return &ReadyForMergeStatus{
		ReviewDecision: ReviewDecision(queryResult.ReviewDecision),
		CIStatus:       overallCIStatus(queryResult.StatusCheckRollupState, statuses),
		Statuses:       statuses,
		Commit:         queryResult.Commit,
	}, nil
}

func overallCIStatus(statusCheckRollupState githubv4.StatusState, statuses []*CIJobStatus) CIStatus {
	if statusCheckRollupState == githubv4.StatusStatePending {
		return CIStatusPending
	}

	result := CIStatusSuccess
	for _, status := range statuses {
		if status.Status == CIStatusPending {
			result = CIStatusPending
			continue
		}

		if status.Required && status.Status == CIStatusFailure {
			return CIStatusFailure
		}
	}

	return result
}

func toCIJobStatuses(
	requiredChecks []string,
	checkRuns []*queryCheckStatus,
	commitStatuses []*queryStatusContext,
) ([]*CIJobStatus, error) {
	statusesByName := make(map[string]*CIJobStatus, len(checkRuns)+len(commitStatuses)+len(requiredChecks))
	for _, context := range requiredChecks {
		if _, exists := statusesByName[context]; exists {
			return nil, fmt.Errorf("found 2 required status with the same context values: %q, context values must be unique", context)
		}

		statusesByName[context] = &CIJobStatus{
			Name:     context,
			Status:   CIStatusPending,
			Required: true,
		}
	}

	for _, run := range checkRuns {
		status, err := checkRunResultToCiStatus(run.Status, run.Conclusion)
		if err != nil {
			return nil, fmt.Errorf("converting checkRun %q CIstatus failed: %w", run.Name, err)
		}

		if entry, exists := statusesByName[run.Name]; exists {
			entry.Status = status
			continue
		}

		statusesByName[run.Name] = &CIJobStatus{
			Name:     run.Name,
			Status:   status,
			Required: false,
		}
	}

	for _, commitStatus := range commitStatuses {
		status, err := contextStatusStateToCIStatus(commitStatus.State)
		if err != nil {
			return nil, fmt.Errorf("converting %q status context to CIstatus failed: %w",
				commitStatus.Context, err)
		}
		if entry, exists := statusesByName[commitStatus.Context]; exists {
			entry.Status = status
			continue
		}

		statusesByName[commitStatus.Context] = &CIJobStatus{
			Name:     commitStatus.Context,
			Status:   status,
			Required: false,
		}
	}

	result := make([]*CIJobStatus, 0, len(statusesByName))
	for _, status := range statusesByName {
		result = append(result, status)
	}

	return result, nil
}

func checkRunResultToCiStatus(status githubv4.CheckStatusState, conclusion githubv4.CheckConclusionState) (CIStatus, error) {
	switch status {
	case githubv4.CheckStatusStateInProgress,
		githubv4.CheckStatusStatePending,
		githubv4.CheckStatusStateQueued,
		githubv4.CheckStatusStateRequested,
		githubv4.CheckStatusStateWaiting:
		return CIStatusPending, nil

	case (githubv4.CheckStatusStateCompleted):
		return checkConclusiontoCIStatus(conclusion)

	default:
		return "", fmt.Errorf("unsupported status value: %q", status)
	}
}

func checkConclusiontoCIStatus(conclusion githubv4.CheckConclusionState) (CIStatus, error) {
	switch conclusion {
	case githubv4.CheckConclusionStateCancelled,
		githubv4.CheckConclusionStateFailure,
		githubv4.CheckConclusionStateStale,
		githubv4.CheckConclusionStateStartupFailure,
		githubv4.CheckConclusionStateTimedOut:
		return CIStatusFailure, nil

	case (githubv4.CheckConclusionStateActionRequired):
		return CIStatusPending, nil

	case githubv4.CheckConclusionStateNeutral,
		githubv4.CheckConclusionStateSkipped,
		githubv4.CheckConclusionStateSuccess:
		return CIStatusSuccess, nil
	default:
		return "", fmt.Errorf("unsupported conclusion value: %q", conclusion)
	}
}

type queryCheckStatus struct {
	Name       string
	Conclusion githubv4.CheckConclusionState
	Status     githubv4.CheckStatusState
}

type queryStatusContext struct {
	State   githubv4.StatusState
	Context string
}

type queryCIStatusResult struct {
	ReviewDecision              githubv4.PullRequestReviewDecision
	StatusCheckRollupState      githubv4.StatusState
	RequiredStatusCheckContexts []string
	CheckRuns                   []*queryCheckStatus
	StatusContext               []*queryStatusContext
	Commit                      string
}

func (clt *Client) reviewAndCIStatus(ctx context.Context, owner, repo string, prNumber int) (*queryCIStatusResult, error) {
	type graphQLQueryCIStatus struct {
		Repository struct {
			PullRequest struct {
				ReviewDecision githubv4.PullRequestReviewDecision

				BaseRef struct {
					BranchProtectionRule struct {
						// RequiredStatusCheckContexts
						// contains required commit
						// statuses and checkRuns.
						RequiredStatusCheckContexts []string
					}
				}

				Commits struct {
					Nodes []struct {
						Commit struct {
							Oid               string
							StatusCheckRollup struct {
								State    githubv4.StatusState
								Contexts struct {
									PageInfo struct {
										EndCursor   string
										HasNextPage bool
									}
									Edges []struct {
										Node struct {
											CheckRun      queryCheckStatus   `graphql:"... on CheckRun"`
											StatusContext queryStatusContext `graphql:"... on StatusContext"`
										}
									}
								} `graphql:"contexts(first: $contextsFirst, after: $contextsAfter)"`
							}
						}
					}
				} `graphql:"commits(last: $commitsLast)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	var prHEADCommitID string
	var result queryCIStatusResult

	vars := map[string]any{
		"owner":         githubv4.String(owner),
		"name":          githubv4.String(repo),
		"number":        githubv4.Int(prNumber),
		"commitsLast":   githubv4.Int(1),
		"contextsFirst": githubv4.Int(100),
		"contextsAfter": (*githubv4.String)(nil),
	}
	page := -1
	for {
		page++
		var q graphQLQueryCIStatus

		err := clt.graphQLClt.Query(ctx, &q, vars)
		if err != nil {
			return nil, err
		}

		commitsNode := q.Repository.PullRequest.Commits.Nodes[0].Commit

		if prHEADCommitID == "" {
			prHEADCommitID = commitsNode.Oid
		} else if prHEADCommitID != commitsNode.Oid {
			vars["contextsAfter"] = (*githubv4.String)(nil)
			prHEADCommitID = ""

			continue
		}

		for _, edge := range commitsNode.StatusCheckRollup.Contexts.Edges {
			node := edge.Node
			if node.CheckRun.Name != "" && node.StatusContext.Context != "" {
				return nil, fmt.Errorf("internal error: node contains checkRun and context, expecting only one")
			}

			if node.CheckRun.Name != "" {
				result.CheckRuns = append(result.CheckRuns, &node.CheckRun)
				continue
			}

			result.StatusContext = append(result.StatusContext, &node.StatusContext)
		}

		pageInfo := commitsNode.StatusCheckRollup.Contexts.PageInfo
		if !pageInfo.HasNextPage {
			result.ReviewDecision = q.Repository.PullRequest.ReviewDecision
			result.StatusCheckRollupState = commitsNode.StatusCheckRollup.State
			result.RequiredStatusCheckContexts = q.Repository.PullRequest.BaseRef.BranchProtectionRule.RequiredStatusCheckContexts
			result.Commit = prHEADCommitID

			return &result, nil
		}

		if pageInfo.EndCursor == "" {
			return nil, fmt.Errorf("retrieving all contexts failed, HasNextPage is %s, expected non-empty EndCursor", pageInfo.EndCursor)
		}

		vars["contextsAfter"] = pageInfo.EndCursor
	}
}

func contextStatusStateToCIStatus(state githubv4.StatusState) (CIStatus, error) {
	switch state {
	case githubv4.StatusStateError,
		githubv4.StatusStateFailure:
		return CIStatusFailure, nil

	case githubv4.StatusStateExpected,
		githubv4.StatusStatePending:
		return CIStatusPending, nil

	case githubv4.StatusStateSuccess:
		return CIStatusSuccess, nil

	default:
		return "", fmt.Errorf("unsupported status state value: %q", state)
	}
}
