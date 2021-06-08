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

// Sync synchronized the states of the autoupdate queues with the current
// pull-request states.
// Pull-Request information are queried from github.
// If it is the first sync , all open Pull-Requests are processed. If it's
// false, all pull-requests are fetched.
// If a PR meets a condition to be enqueued for auto-updates it is enqueued.
// If it meets a condition for not bein autoupdated, it is dequeued.
func (a *Autoupdater) Sync(ctx context.Context) error {
	for repo := range a.repositories {
		err := a.sync(ctx, repo.Owner, repo.RepositoryName)
		if err != nil {
			return fmt.Errorf("syncing %s failed: %w", repo, err)
		}
	}

	return nil
}

type action int

const (
	none action = iota
	enqueue
	dequeue
)

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
	a.lock.Lock()
	if !a.firstSyncDone {
		stateFilter = "open"
		a.firstSyncDone = true // TODO: only set if sync was successful?
	} else {
		stateFilter = "all"
	}
	a.lock.Unlock()

	// TODO: could we query less Pull-Requests by ignoring PRs that are
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
					logFieldEventIgnored,
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
					"dequeing pull-request failed",
					logFieldEventIgnored,
					zap.Error(err),
				)
				break
			}

			stats.Dequeued++
			logger.Info(
				"queue was out of sync, pr dequeued",
				logfields.Event("queue_out_of_sync"),
			)

		case none:
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

func (a *Autoupdater) evaluateAction(pr *github.PullRequest) action {
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

	return none
}
