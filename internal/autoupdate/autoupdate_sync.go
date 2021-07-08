package autoupdate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-github/v35/github"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

type syncAction int

const (
	undefined syncAction = iota
	enqueue
	dequeue
)

// Sync synchronized the states of the autoupdater queues with the current
// pull request state at GitHub.
// Pull request information is queried from github.
// If a PR meets a condition to be enqueued for auto-updates it is enqueued.
// If it meets a condition for not being autoupdated, it is dequeued.
func (a *Autoupdater) Sync(ctx context.Context) error {
	for repo := range a.monitoredRepos {
		err := a.sync(ctx, repo.OwnerLogin, repo.RepositoryName)
		if err != nil {
			return fmt.Errorf("syncing %s failed: %w", repo, err)
		}
	}

	return nil
}

func (a *Autoupdater) sync(ctx context.Context, owner, repo string) error {
	/* TODO: when using this method to clean-up the queues during runtime, there is the chance of a race.
	   We might receive and process a later PR Closed event, remove it from
	   the queue and then retrieve from the API an earlier PR state and add
	   the closed pr to the queue again.

	   Possible solution: honor the pushed_at timestamp in the events and
	   PullRequest objects, that would mean we would have to store those
	   timestamps for already dequeued PRs
	*/

	stats := syncStat{StartTime: time.Now()}

	logger := a.logger.With(
		logfields.Repository(repo),
		logfields.RepositoryOwner(owner),
	)

	logger.Info(
		"starting synchronization",
		logfields.Event("sync_started"),
	)

	var stateFilter string
	a.queuesLock.Lock()
	// if no pull requests are queued for updates, there are no
	// pull requests that can be removed from the queue. Therefore it is
	// sufficient to get information for open prs from GitHub.
	if len(a.queues) == 0 {
		stateFilter = "open"
	} else {
		stateFilter = "all"
	}
	a.queuesLock.Unlock()

	// TODO: could we query less pull requests by ignoring PRs that are
	// closed and were last changed before goordinator started?
	it := a.ghClient.ListPullRequests(ctx, owner, repo, stateFilter, "asc", "created")
	for {
		var pr *github.PullRequest

		// TODO: use a lower timeout for the retries, otherwise we might get stuck here for too long on startup
		err := a.retryer.Run(ctx, func(ctx context.Context) error {
			var err error
			pr, err = it.Next()
			return err
		}, nil)
		if err != nil {
			return err
		}

		if pr == nil { // iteration finished, no more results
			break
		}

		stats.Seen++

		// redefine variable, to make PR fields scoped to this iteration
		logger := logger.With(logfields.PullRequest(pr.GetNumber()))

		switch action := a.evaluateAction(pr); action {
		case enqueue:
			err := a.enqueuePR(ctx, owner, repo, pr)
			if errors.Is(err, ErrAlreadyExists) {
				logger.Debug(
					"queue in-sync, pr is enqueued",
					logfields.Event("queue_in_sync"),
				)
				break
			}
			if err != nil {
				stats.Failures++
				logger.Warn(
					"adding pr to queue failed",
					logEventEventIgnored,
					zap.Error(err),
				)
				break
			}

			stats.Enqueued++
			logger.Info(
				"queue was out of sync, pr enqueue",
				logfields.Event("queue_out_of_sync"),
			)

		case dequeue:
			err := a.dequeuePR(ctx, owner, repo, pr)
			if errors.Is(err, ErrNotFound) {
				logger.Debug(
					"queue in-sync, pr not queued",
					logfields.Event("queue_in_sync"),
					zap.Error(err),
				)
				break
			}

			if err != nil {
				stats.Failures++
				logger.Warn(
					"dequeing pull request failed",
					logEventEventIgnored,
					zap.Error(err),
				)
				break
			}

			stats.Dequeued++
			logger.Info(
				"queue was out of sync, pr dequeued",
				logfields.Event("queue_out_of_sync"),
			)

		case undefined:
			continue

		default:
			logger.DPanic(
				"evaluateAction() returned unexpected value",
				zap.Int("value", int(action)),
			)
		}
	}

	stats.EndTime = time.Now()

	logger.Info("synchronization finished",
		stats.LogFields()...,
	)

	return nil
}

func (a *Autoupdater) enqueuePR(ctx context.Context, repoOwner, repo string, ghPr *github.PullRequest) error {
	bb, err := NewBaseBranch(repoOwner, repo, ghPr.GetBase().GetRef())
	if err != nil {
		return fmt.Errorf("incomplete base branch information: %w", err)
	}

	pr, err := NewPullRequest(ghPr.GetNumber(), ghPr.GetHead().GetRef())
	if err != nil {
		return fmt.Errorf("incomplete pr information: %w", err)
	}

	return a.Enqueue(ctx, bb, pr)
}

func (a *Autoupdater) dequeuePR(ctx context.Context, repoOwner, repo string, ghPr *github.PullRequest) error {
	bb, err := NewBaseBranch(repoOwner, repo, ghPr.GetBase().GetRef())
	if err != nil {
		return fmt.Errorf("incomplete base branch information: %w", err)
	}

	prNumber := ghPr.GetNumber()
	if prNumber <= 0 {
		return fmt.Errorf("invalid pr number: %d", prNumber)
	}

	_, err = a.Dequeue(ctx, bb, prNumber)
	return err
}

func (a *Autoupdater) evaluateAction(pr *github.PullRequest) syncAction {
	if pr.GetState() == "closed" {
		return dequeue
	}

	if a.triggerOnAutomerge && pr.GetAutoMerge() != nil {
		return enqueue
	}

	if len(a.triggerLabels) != 0 {
		for _, label := range pr.Labels {
			labelName := label.GetName()
			if _, exist := a.triggerLabels[labelName]; exist {
				return enqueue
			}
		}

		return dequeue
	}

	return undefined
}
