package goorderr

import (
	"fmt"
	"time"
)

type RetryableError struct {
	// Err is the wrapped original error
	Err error
	// After is the earlierst point in time that the opertion can be retried
	After time.Time
}

func NewRetryableError(originalErr error, retryAfter time.Time) *RetryableError {
	return &RetryableError{
		Err:   originalErr,
		After: retryAfter,
	}
}

func NewRetryableAnytimeError(originalErr error) *RetryableError {
	return &RetryableError{
		Err: originalErr,
	}
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

func (e *RetryableError) Error() string {
	if e.After.IsZero() {
		return fmt.Sprintf("retryable error: %s", e.Err)
	}

	return fmt.Sprintf("retryable error (after %s): %s", e.After, e.Err)
}
