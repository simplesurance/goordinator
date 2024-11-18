package autoupdate

import (
	"fmt"

	"github.com/google/go-github/v59/github"
)

func strPtr(in string) *string {
	return &in
}

func newBasicPullRequestEvent(prNumber int, branchName, baseBranchName string) *github.PullRequestEvent {
	return &github.PullRequestEvent{
		Number:      &prNumber,
		PullRequest: newBasicPullRequest(prNumber, baseBranchName, branchName),
		Repo: &github.Repository{
			Name: strPtr(repo),
			Owner: &github.User{
				Login: strPtr(repoOwner),
			},
		},
	}
}

func newBasicPullRequest(prNumber int, baseBranchName, branchName string) *github.PullRequest {
	return &github.PullRequest{
		Number: &prNumber,
		Base: &github.PullRequestBranch{
			Ref: &baseBranchName,
		},
		Head: &github.PullRequestBranch{
			Ref: strPtr(branchName),
		},
	}
}

func newSyncEvent(prNumber int, branchName, baseBranchName string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("synchronize")

	return pr
}

func newPullRequestLabeledEvent(prNumber int, branchName, baseBranchName, label string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("labeled")
	pr.Label = &github.Label{
		Name: &label,
	}

	return pr
}

func newPullRequestReviewEvent(prNumber int, branchName, baseBranchName, action, state string) *github.PullRequestReviewEvent {
	return &github.PullRequestReviewEvent{
		Action:      strPtr(action),
		Review:      &github.PullRequestReview{State: strPtr(state)},
		PullRequest: newBasicPullRequest(prNumber, baseBranchName, branchName),
		Repo: &github.Repository{
			Name: strPtr(repo),
			Owner: &github.User{
				Login: strPtr(repoOwner),
			},
		},
	}
}

func newPullRequestAutomergeEnabledEvent(prNumber int, branchName, baseBranchName string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("auto_merge_enabled")

	return pr
}

func newPullRequestAutomergeDisabledEvent(prNumber int, branchName, baseBranchName string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("auto_merge_disabled")

	return pr
}

func newPullRequestUnlabeledEvent(prNumber int, branchName, baseBranchName, label string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("unlabeled")
	pr.Label = &github.Label{
		Name: &label,
	}

	return pr
}

func newPullRequestClosedEvent(prNumber int, branchName, baseBranchName string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, baseBranchName)
	pr.Action = strPtr("closed")

	return pr
}

func newPullRequestBaseBranchChangeEvent(prNumber int, branchName, baseBranchName, newBaseBranch string) *github.PullRequestEvent {
	pr := newBasicPullRequestEvent(prNumber, branchName, newBaseBranch)

	base := github.EditBase{
		Ref: &github.EditRef{
			From: &baseBranchName,
		},
	}

	pr.Changes = &github.EditChange{
		Base: &base,
	}

	pr.Action = strPtr("edited")

	return pr
}

func newPushEvent(baseBranch string) *github.PushEvent {
	return &github.PushEvent{
		Ref: strPtr(fmt.Sprintf("refs/heads/%s", baseBranch)),
		Repo: &github.PushEventRepository{
			Name: strPtr(repo),
			Owner: &github.User{
				Login: strPtr(repoOwner),
			},
		},
	}
}

func newStatusEvent(state string, branch ...string) *github.StatusEvent {
	ghBranches := make([]*github.Branch, 0, len(branch))
	for i := range branch {
		ghBranches = append(ghBranches, &github.Branch{Name: &branch[i]})
	}

	return &github.StatusEvent{
		Branches: ghBranches,
		State:    &state,
		Repo: &github.Repository{
			Name: strPtr(repo),
			Owner: &github.User{
				Login: strPtr(repoOwner),
			},
		},
	}
}

func newCheckRunEvent(conclusion string, branches ...string) *github.CheckRunEvent {
	prs := make([]*github.PullRequest, 0, len(branches))
	for _, br := range branches {
		prs = append(prs, newBasicPullRequest(1, "master", br))
	}

	return &github.CheckRunEvent{
		Repo: &github.Repository{
			Name: strPtr(repo),
			Owner: &github.User{
				Login: strPtr(repoOwner),
			},
		},
		CheckRun: &github.CheckRun{
			PullRequests: prs,
			Conclusion:   &conclusion,
		},
	}
}
