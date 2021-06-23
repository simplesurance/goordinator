// Package autoupdate provides automatic serialized updating of GitHub
// Pull-Request branches with their base-branch.
//
// The autoupdater can be used in combination with the GitHub Automerge feature
// and specific branch protection rules to provide a serialized merge-queue.
// The GitHub branch protection rules are required to be be configured to:
//
// - Require >=1 status checks to pass before merging,
//
// - Require branches to be up to date before merging
//
// When automerge is enabled for a Pull-Requests it is queued for being update
// automatically with it's base branch. When all Status Checks succeed for the
// PR, it is uptodate with it's base branch and all configured mandatory PR
// reviewers approved, the auto-merge feature will merge the PR into it's base branch.
// Per base-branch the autoupdater, updates automatically the first PR in the
// queue with it's base branch. It serializes updating per basebranch, to avoid
// a race between multiples pull-requests that result in unnecessary CI runs and merges.
// The autoupdater merges the base-branch into the Pull-Request branch in the
// following circumstances:
// - The base branch changed and
// - the Pull-Request branch changed and is not uptodate with it's base branch
//   anymore.
// When merging the base-branch into the PR branch is not possible or a failed
// status check for the pull-request was reported, updates for the
// Pull-Requests are suspended and the next PR in the queue will be kept uptodate.
// When later the status checks became positive for the PR, it's branch changed
// or the base-branch changed, updates for it's are resumed. It will be
// enqueued again for updates, and kept uptodate when it is first in the queue.
//
// Components
//
// The main components are the queue and autoupdater.
//
// The autoupdater manages and coordinates operations on queues.
// It listens for Github Webhook events or querues the current state from
// GitHub and triggers operations on the queues.
// For every base-branch for Pull-Requests that are kept uptodate it maintains 1 queue.
//
// A queue serializes updating of Pull-Request branches per baseBranch.
// Enqueued Pull-Requests can be in active or suspended state.
// Active Pull-Requests are managed in a FIFO-queue, the first Pull-Request in
// the branch is kept uptodate with it's base branch.
// Pull-Requests are in suspended are not kept uptodate.
package autoupdate

// TODO: where to put this package level documentary?

/* The autoupdater responds to the following github Events.
   Status-Event:
	A status Event is associated with one or more commits.
	For all branches that contain the commit and are associated to a
	pull-request for that have autoupdates are suspended goordinator
	retrieves the combined status for the pull-request.
	If the combined status is successful and updates for the pull-request
	are currently suspended, updates are resumed.
    Push Event:
        When a push-event is received and it was for:
	    - a base branch for an monitored pull-request, the first PR in the
	      queue for the base-branch is updated,

   Pull-Request Event:
       synchronize action:
	   When a synchronize Pull-Request event is received and the event was
	   for monitored pull-request and updates for it are currently
	   suspendend, updates will be resumed.

When the label and automerge trigger is enabled, it reacts both on them.
If a PR has a trigger and automerge-enabled and one of them is disabled, it is
removed from the queue, despite the other trigger is still active.

Known Issues:
- Pull-Request Review state is not considered,
  if auto_merge is enabled but the PR as not approved, it can not be merged
  automatically and will block the first element in the queue.
  Fix: Query the github graphQL API to get the  PullRequestReviewDecision
       field(https://docs.github.com/en/graphql/reference/enums?query=APP#pullrequestreviewdecision).
       If a PR is not approved yet, suspend it.
       React also Approval events to move the PR to the active queue again when it was approved.
- Rework comment posting, currently only comments are posted when updating with
base branch is not possible, it's difficult for a user to figure out if his pr
is suspendend or in the active queue and at which place. This status should be visible in the PR
*/
