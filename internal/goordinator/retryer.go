package goordinator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/cenkalti/backoff"

	"github.com/simplesurance/goordinator/internal/goorderr"
	"github.com/simplesurance/goordinator/internal/logfields"
)

// DefTimeout is used as timeout for runs when the passed context has no deadline set.
const DefTimeout = 24 * time.Hour

// Retryer executes a function repeatedly until it was successful or it's
// context was cancelled.
type Retryer struct {
	logger       *zap.Logger
	shutdownChan chan struct{}
}

func NewRetryer() *Retryer {
	return &Retryer{
		logger:       zap.L().Named("retryer"),
		shutdownChan: make(chan struct{}),
	}
}

// Run executes fn until it was successful, it returned an error that
// does not wrap goorderr.RetryableError or the execution was aborted via the
// context.
// If the context has no deadline set, it will be set to DefTimeout.
func (r *Retryer) Run(ctx context.Context, fn func(context.Context) error, logF []zap.Field) error {
	var tryCnt uint

	if _, set := ctx.Deadline(); !set {
		var cancelFunc context.CancelFunc

		r.logger.Debug("context has no deadline set, using default timeout", zap.Duration("timeout", DefTimeout))
		ctx, cancelFunc = context.WithTimeout(ctx, DefTimeout)
		defer cancelFunc()
	}

	retryTimer := time.NewTimer(0)
	defer retryTimer.Stop()

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 5 * time.Second

	logger := r.logger.With(logF...)

	for {
		tryCnt++

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-retryTimer.C:
			logger = logger.With(zap.Uint("try_count", tryCnt))

			logger.Debug(
				"running action",
				logfields.Event("action_running"),
				zap.Duration("age", bo.GetElapsedTime()),
			)

			err := fn(ctx)
			if err != nil {
				var retryError *goorderr.RetryableError

				if errors.As(err, &retryError) {
					var retryIn time.Duration

					logger = logger.With(
						zap.Duration("age", bo.GetElapsedTime()),
						zap.Error(err),
					)

					if untilAfter := time.Until(retryError.After); untilAfter > 0 {
						retryIn = untilAfter
					} else {
						retryIn = bo.NextBackOff()
					}

					timerWasActive := retryTimer.Reset(retryIn)
					if timerWasActive {
						logger.DPanic("timer was active when reset was called")
					}

					logger.Info(
						"action failed, retry scheduled",
						logfields.Event("action_retry_scheduled"),
						zap.Duration("retry_in", retryIn),
					)

					continue
				}

				return err
			}

			return nil

		case <-r.shutdownChan:
			return errors.New("event loop terminated, action not executed successful")
		}
	}
}

// Stop notifies all Run() methods to terminate.
// It does not wait for their termination.
func (r *Retryer) Stop() {
	r.logger.Debug("retryer terminating", logfields.Event("retryer_terminating"))

	select {
	case <-r.shutdownChan:
		return // already closed
	default:
		close(r.shutdownChan)
	}
}
