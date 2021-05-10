package goordinator

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/action"
	"github.com/simplesurance/goordinator/internal/action/httprequest"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider"
)

const DefEventChannelBufferSize = 512
const DefRetryTimeout = 2 * time.Hour

const loggerName = "event-loop"

// EvLoop receives events and triggers matching actions.
// Actions are executed asynchronously in go-routines and are retried until
// DefRetryTimeout expired.
type EvLoop struct {
	ch     chan *provider.Event
	logger *zap.Logger
	rules  []*Rule

	actionWg      sync.WaitGroup
	shutdownChan  chan struct{}
	actionDeferFn func()
}

// WithActionRoutineDeferFunc sets a function to be run when an go-routine that
// executes an action returns.
// It can be used to set a panic handler.
// Because it does not pass any arguments, there is probably no other good
// usecase for it. :-)
func WithActionRoutineDeferFunc(fn func()) func(*EvLoop) {
	return func(e *EvLoop) {
		e.actionDeferFn = fn
	}
}

func NewEventLoop(rules []*Rule, opts ...func(*EvLoop)) *EvLoop {
	evl := EvLoop{
		ch:           make(chan *provider.Event, DefEventChannelBufferSize),
		rules:        rules,
		shutdownChan: make(chan struct{}, 1),
	}

	for _, opt := range opts {
		opt(&evl)
	}

	if evl.logger == nil {
		evl.logger = zap.L().Named(loggerName)
	}

	return &evl
}

// C returns the event channel.
// Events sent to this channel will be processed.
// The channel is closed when Stop() is called.
func (e *EvLoop) C() chan<- *provider.Event {
	return e.ch
}

func (e *EvLoop) Start() {
	ctx := context.Background()
	e.logger.Info("ready to process events", logfields.Event("eventloop_started"))

	for ev := range e.ch {
		logger := e.logger.With(ev.LogFields()...)

		logger.Debug("event received", logfields.Event("event_received"))

		for _, rule := range e.rules {
			logger := logger.With(zap.String("rule_name", rule.name))

			match, err := rule.Match(ctx, ev)
			if err != nil {
				logger.Error(
					"matching rule failed",
					logfields.Event("rule_matching_failed"),
					zap.Error(err),
				)
				continue
			}

			logger.Debug(
				"evaluated result of matching event with rule",
				logfields.Event("rule_match_result_evaluted"),
				zap.String("match_result", match.String()),
			)

			switch match {
			case Match:
				break
			case EventSourceMismatch, RuleMismatch:
				continue
			case MatchResultUndefined:
				logger.Error(
					"match returned invalid result",
					logfields.Event("rule_match_invalid_result"),
					zap.String("match_result", match.String()),
				)
			default:
				logger.Panic(
					"match returned undefined MatchResult enum value",
					zap.Int("match_result_int", int(match)),
				)
			}

			actions, err := rule.TemplateActions(ctx, ev)
			if err != nil {
				logger.Error(
					"templating action definition failed, rule is skipped",
					logfields.Event("rule_action_templating_failed"),
					zap.Error(err),
				)
				continue
			}

			for _, action := range actions {
				e.scheduleAction(ctx, ev, action)
			}
		}
	}

	e.logger.Info(
		"event loop terminated, event channel was closed",
		logfields.Event("eventloop_termianted"),
	)
}

func logFieldActionResult(val string) zap.Field {
	return zap.String("action_result", val)
}

func (e *EvLoop) scheduleAction(ctx context.Context, event *provider.Event, action action.Runner) {
	e.actionWg.Add(1)

	go func() {
		var retryCount uint

		if e.actionDeferFn != nil {
			defer e.actionDeferFn()
		}

		defer e.actionWg.Done()

		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 5 * time.Second
		bo.MaxElapsedTime = DefRetryTimeout

		ticker := backoff.NewTicker(bo)
		defer ticker.Stop()

		logger := e.logger.With(append(
			event.LogFields(),
			action.LogFields()...,
		)...)

		for {
			select {
			case _, channelOpen := <-ticker.C:
				if !channelOpen {
					logger.Warn(
						"giving up retrying action execution, retry timeout expired",
						logfields.Event("action_retry_timeout"),
						logFieldActionResult("cancelled"),
						zap.Uint("retry_count", retryCount),
						zap.Duration("age", bo.GetElapsedTime()),
						zap.Duration("max_age", bo.MaxElapsedTime),
					)

					return
				}

				logger := logger.With(zap.String("action", action.String()))

				logger.Debug(
					"running action",
					logfields.Event("action_running"),
					zap.Uint("retry_count", retryCount),
					zap.Duration("age", bo.GetElapsedTime()),
					zap.Duration("max_age", bo.MaxElapsedTime),
				)

				err := action.Run(ctx)
				retryCount++
				if err != nil {
					var httpErr *httprequest.ErrorHTTPRequest

					if errors.As(err, &httpErr) {
						logger.Info(
							"action failed",
							logfields.Event("action_failed"),
							logFieldActionResult("failure"),
							zap.Int("http_response_code", httpErr.Status),
							zap.ByteString("http_response_body", httpErr.Body),
						)
						continue
					}

					logger.Error(
						"action failed",
						logfields.Event("action_failed"),
						logFieldActionResult("failure"),
						zap.Error(err),
					)
					continue
				}

				logger.Info(
					"action executed",
					logfields.Event("action_executed_successfully"),
					logFieldActionResult("success"),
				)

				return

			case <-ctx.Done():
				logger.Info(
					"action execution cancelled",
					logfields.Event("action_execution_cancelled"),
					logFieldActionResult("cancelled"),
				)
				return

			case <-e.shutdownChan:
				logger.Info("evloop terminating, action was not executed",
					logfields.Event("action_execution_cancelled_evloop_terminated"),
					logFieldActionResult("cancelled"),
				)
			}

		}
	}()
}

// Stop stops the event loop, all waits until all scheduled go-routines
// terminated.
// The event channel (Evloop.C()) will be closed.
func (e *EvLoop) Stop() {
	e.logger.Debug("event loop terminating", logfields.Event("eventloop_terminating"))

	close(e.ch)
	close(e.shutdownChan)
	e.actionWg.Wait()

	e.logger.Info("event loop terminated", logfields.Event("eventloop_terminated"))
}
