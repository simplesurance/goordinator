package goordinator

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/goorderr"
)

func TestRetryerDefaultTimeout(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	r := NewRetryer()
	t.Cleanup(r.Stop)

	r.defTimeout = time.Second

	var err error
	assert.Eventually(
		t,
		func() bool {
			err = r.Run(context.Background(), func(context.Context) error {
				return goorderr.NewRetryableAnytimeError(errors.New("err"))
			}, nil)

			t.Logf("err: %s\n", err)
			return true
		},
		r.defTimeout+time.Second,
		200*time.Millisecond,
	)

	assert.ErrorIsf(t, err, context.DeadlineExceeded, "err: %+v", err)
}

func TestRetryAfterInThePast(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	r := NewRetryer()
	r.backoffInitialInterval = 100 * time.Millisecond
	t.Cleanup(r.Stop)

	ctx, cancelFunc := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFunc()

	var retryTimes []time.Time

	err := r.Run(ctx, func(context.Context) error {
		retryTimes = append(retryTimes, time.Now())
		return goorderr.NewRetryableError(errors.New("err"), time.Now().Add(-time.Second))
	}, nil)

	assert.ErrorIs(t, err, context.DeadlineExceeded)

	require.GreaterOrEqual(t, len(retryTimes), 2)

	for i := 1; i < len(retryTimes); i++ {
		d := retryTimes[i].Sub(retryTimes[i-1])
		require.GreaterOrEqualf(t, d, minInterval(r),
			"time between retry %d and %d is %s, expected >=%s",
			retryTimes[i-1], retryTimes[i], d, minInterval(r),
		)
	}
}

func TestBackoffInterval(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	r := NewRetryer()
	r.backoffInitialInterval = 500 * time.Millisecond
	t.Cleanup(r.Stop)

	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()

	var retryTimes []time.Time

	err := r.Run(ctx, func(context.Context) error {
		retryTimes = append(retryTimes, time.Now())
		return goorderr.NewRetryableAnytimeError(errors.New("err"))
	}, nil)

	assert.ErrorIs(t, err, context.DeadlineExceeded)

	require.GreaterOrEqual(t, len(retryTimes), 2)
	for i := 1; i < len(retryTimes); i++ {
		d := retryTimes[i].Sub(retryTimes[i-1])
		require.GreaterOrEqualf(t, d, minInterval(r),
			"time between retry %d and %d is %s, expected >=%s",
			i-1, i, retryTimes[i-1], retryTimes[i], d, minInterval(r),
		)
	}
}

func minInterval(retryer *Retryer) int64 {
	return int64(math.Floor(float64(retryer.backoffInitialInterval) * (1 - retryer.backoffRandomizationFactor)))
}

func TestBackOffIntervalReachesMaxElapsedTime(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	r := NewRetryer()
	r.backoffInitialInterval = 500 * time.Millisecond
	r.backoffMaxElapsedTime = time.Millisecond
	t.Cleanup(r.Stop)

	ctx, cancelFunc := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFunc()

	var retryTimes []time.Time

	err := r.Run(ctx, func(context.Context) error {
		retryTimes = append(retryTimes, time.Now())
		return goorderr.NewRetryableAnytimeError(errors.New("err"))
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bug: backoff timer returned stop signal")

	require.GreaterOrEqual(t, len(retryTimes), 2)
	t.Logf("retries: %d", len(retryTimes))

	for i := 1; i < len(retryTimes); i++ {
		d := retryTimes[i].Sub(retryTimes[i-1])
		require.GreaterOrEqualf(t, d, minInterval(r),
			"time between retry %d and %d is %s, expected >=%s",
			i-1, i, retryTimes[i-1], retryTimes[i], d, minInterval(r),
		)
	}
}

func TestRetryerDoesNotAccumulateLogFields(t *testing.T) {
	var logBuf bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logBuf),
		zapcore.DebugLevel,
	)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))

	r := NewRetryer()
	r.backoffInitialInterval = 10 * time.Millisecond
	t.Cleanup(r.Stop)

	var attempts int
	err := r.Run(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return goorderr.NewRetryableAnytimeError(errors.New("temporary failure"))
		}
		return nil
	}, []zap.Field{zap.String("action", "httprequest")})

	require.NoError(t, err)
	require.Equal(t, 3, attempts)

	lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	require.NotEmpty(t, lines)

	for i, line := range lines {
		if !strings.Contains(line, `"try_count":`) {
			continue
		}
		assert.Equalf(t, 1, strings.Count(line, `"try_count":`), "line %d has duplicate try_count: %s", i, line)
		assert.Equalf(t, 1, strings.Count(line, `"age":`), "line %d has duplicate age: %s", i, line)
		assert.LessOrEqualf(t, strings.Count(line, `"error":`), 1, "line %d has duplicate error: %s", i, line)
	}
}
