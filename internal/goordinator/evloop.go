package goordinator

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/action"
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
	actionDeferFn func()
	retryer       *Retryer
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
		ch:      make(chan *provider.Event, DefEventChannelBufferSize),
		rules:   rules,
		retryer: NewRetryer(),
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
		if e.actionDeferFn != nil {
			defer e.actionDeferFn()
		}

		defer e.actionWg.Done()

		_ = e.retryer.Run(
			ctx,
			action.Run,
			append(event.LogFields(), action.LogFields()...),
		)
	}()
}

// Stop stops the event loop, all waits until all scheduled go-routines
// terminated.
// The event channel (Evloop.C()) will be closed.
func (e *EvLoop) Stop() {
	e.logger.Debug("event loop terminating", logfields.Event("eventloop_terminating"))
	close(e.ch)

	e.retryer.Stop()

	e.logger.Debug(
		"waiting for scheduled actions to terminate",
		logfields.Event("eventloop_terminating"),
	)
	e.actionWg.Wait()

	e.logger.Info("event loop terminated", logfields.Event("eventloop_terminated"))
}
