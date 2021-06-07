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

// Retryer executes a function repeatedly until it was successful or cancel
// condition happened.
type Retryer struct {
	logger          *zap.Logger
	maxRetryTimeout time.Duration //2 hours
	shutdownChan    chan struct{}
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
func (r *Retryer) Run(ctx context.Context, fn func(context.Context) error, logF []zap.Field) error {
	var tryCnt uint

	startTime := time.Now()
	endTime := startTime.Add(r.maxRetryTimeout)

	retryTimeout := time.NewTimer(r.maxRetryTimeout)
	defer retryTimeout.Stop()

	retryTimer := time.NewTimer(0)
	defer retryTimeout.Stop()

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 5 * time.Second

	for {
		tryCnt++
		logger := r.logger.With(zap.Uint("try_count", tryCnt))

		select {
		case <-ctx.Done():
			logger.Info(
				"action execution cancelled",
				logfields.Event("action_execution_cancelled"),
				logFieldActionResult("cancelled"),
			)

			return ctx.Err()

		case <-retryTimer.C:
			logger.Debug(
				"running action",
				logfields.Event("action_running"),
				zap.Uint("try_count", tryCnt),
				zap.Duration("age", bo.GetElapsedTime()),
				zap.Duration("retry_timeout", r.maxRetryTimeout),
			)

			err := fn(ctx)
			if err != nil {
				var retryError *goorderr.RetryableError

				logger = logger.With(zap.Error(err))

				if errors.Is(err, context.Canceled) {
					logger.Error(
						"action cancelled",
						logfields.Event("action_cancelled"),
						logFieldActionResult("cancelled"),
					)

					return err
				}

				if errors.As(err, &retryError) {
					logger = logger.With(
						zap.Duration("age", bo.GetElapsedTime()),
						zap.Duration("retry_timeout", r.maxRetryTimeout),
					)

					if retryError.After.After(endTime) {
						logger.Error(
							"action failed, next possible retry time is after timeout expiration",
							logfields.Event("action_failed"),
							zap.Time("earliest_allowed_retry", retryError.After),
						)

						return err
					}

					var retryIn time.Duration

					if retryError.After.IsZero() {
						retryIn = bo.NextBackOff()
					} else {
						retryIn = time.Until(retryError.After)
					}

					retryTimer.Reset(retryIn)
					logger.Error(
						"action failed, retry scheduled",
						logfields.Event("action_retry_scheduled"),
						zap.Duration("retry_in", retryIn),
					)

					continue
				}

				logger.Error(
					"action failed, not retryable",
					logfields.Event("action_failed"),
					logFieldActionResult("failure"),
				)

				return err
			}

			logger.Info(
				"action executed successfully",
				logfields.Event("action_executed_successfully"),
				logFieldActionResult("success"),
			)

			return nil

		case <-retryTimeout.C:
			logger.Warn(
				"giving up retrying action execution, retry timeout expired",
				logfields.Event("action_retry_timeout"),
				logFieldActionResult("cancelled"),
				zap.Duration("age", bo.GetElapsedTime()),
				zap.Duration("retry_timeout", r.maxRetryTimeout),
			)

			return errors.New("retry timeout expired")

		case <-r.shutdownChan:
			logger.Info(
				"evloop terminating, action not executed",
				logfields.Event("action_execution_cancelled_evloop_terminated"),
				logFieldActionResult("cancelled"),
			)

			return nil
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
