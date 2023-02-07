// Package autoupdate provides automatic serialized updating of GitHub
// pull request branches with their base-branch.
//
// The autoupdater can be used in combination with a automerge feature like the
// one from GitHub and specific branch protection rules to provide a
// serialized merge-queue.
// The GitHub branch protection rules must be configured to require
// >=1 status checks to pass before merging and branches being up to date
// before merging.
//
// The autoupdater reacts on GitHub webhook-events and interacts with the
// GitHub API.
// When a webhook event for an enqueue condition is received, the pull request
// is enqueued for autoupdates.
// When all required Status Checks succeed for the PR, it is uptodate with it's
// base branch and all configured mandatory PR reviewers approved, the
// auto-merge feature will merge the PR into it's base branch.
// Per base-branch, the autoupdater updates automatically the first PR in the
// queue with it's base branch. It serializes updating per basebranch, to avoid
// a race between multiples pull requests that result in unnecessary CI runs and
// merges.
// When updating a pull request branch with it's base-branch is not possible,
// a failed status check for the pull request was reported, or the PR became
// stale updates for it are suspended.
// This prevents that pull requests that can not be merged block the autoupdate
// fifo-queue.
// When a webhook event is received about a positive status check report, the
// base branch or the pull request branch changed, updates will be resumed.
// The pull request will be enqueued in the fifo list for updates again.
//
// pull requests are removed from the autoupdate queue, when it was closed, or
// an inverse trigger event (auto_merge_disabled, unlabeled) was received.
//
// # Components
//
// The main components are the queue and autoupdater.
//
// The autoupdater manages and coordinates operations on queues.
// It listens for GitHub Webhook events, creates/removes removes,
// enqueues/dequeues pull requests for updates and triggers update operations
// on queues. It can also synchronize the queue with the current state at
// GitHub, by querying information via the GitHub API.
// It also provides a minimal webinterface to view the current state.
//
// Queues serialize updates per base-branch. For each base-branch the
// Autoupdater manages one queue.
// pull requests in the queue are either in active or suspended state.
// If they are active, they are queued in FIFO datastructure and update
// operations can be run on the first element in the queue.
// If they are suspended they are currently not considered for autoupdates and
// stored in a separate datastructure.
package autoupdate
