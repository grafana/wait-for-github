package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"

	gh "github.com/grafana/wait-for-github/internal/github"
	"github.com/stretchr/testify/assert"
)

type TestCheck struct {
	fn func() error
}

func (t *TestCheck) Check(ctx context.Context) error {
	return t.fn()
}

var testLogger = slog.New(slog.NewTextHandler(
	io.Discard,
	&slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

// TestCancel tests that RunUntilCancelledOrTimeout returns an error when the
// context is cancelled.
func TestCancel(t *testing.T) {
	ctx := context.Background()
	timeoutCtx, cancel := context.WithCancel(ctx)
	cancel()

	check := &TestCheck{
		fn: func() error {
			return nil
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, 1*time.Second)

	assert.Error(t, err)
}

func TestCalledRepeatedly(t *testing.T) {
	ctx := context.Background()
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// sentinel error to check that the function is called repeatedly
	exitError := fmt.Errorf("exit error")

	n := 0
	check := &TestCheck{
		fn: func() error {
			n++
			if n < 10 {
				return nil
			}
			return exitError
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, 1*time.Millisecond)

	assert.Equal(t, 10, n)
	assert.Equal(t, exitError, err)
}

func TestInterrupt(t *testing.T) {
	ctx := context.Background()
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	check := &TestCheck{
		fn: func() error {
			p, err := os.FindProcess(os.Getpid())
			if err != nil {
				return err
			}
			return p.Signal(syscall.SIGINT)
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, 1*time.Millisecond)

	assert.EqualError(t, err, "Received SIGINT")
}

// TestPrimaryRateLimitRetries tests that RunUntilCancelledOrTimeout does not
// return the GitHubRateLimitError, and instead waits at least until ResetTime
// before retrying.
func TestPrimaryRateLimitRetries(t *testing.T) {
	ctx := t.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	interval := 50 * time.Millisecond
	resetDelay := 200 * time.Millisecond // longer than interval to verify the minimum is honoured
	var resetTime time.Time
	var secondCallAt time.Time
	sentinelErr := fmt.Errorf("sentinel: second call succeeded")

	calls := 0
	check := &TestCheck{
		fn: func() error {
			calls++
			if calls == 1 {
				resetTime = time.Now().Add(resetDelay)
				return &gh.GitHubRateLimitError{
					ResetTime: resetTime,
				}
			}
			secondCallAt = time.Now()
			return sentinelErr
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, interval)

	assert.Equal(t, sentinelErr, err, "expected sentinel error from second call, not the rate limit error")
	assert.Equal(t, 2, calls)
	assert.False(t, secondCallAt.Before(resetTime), "second call should not happen before ResetTime")
}

// TestAbuseRateLimitRespectsRetryAfter tests that RunUntilCancelledOrTimeout
// waits at least RetryAfter before retrying when RetryAfter exceeds the interval.
func TestAbuseRateLimitRespectsRetryAfter(t *testing.T) {
	ctx := t.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	interval := 50 * time.Millisecond
	retryAfter := 200 * time.Millisecond // longer than interval to verify the minimum is honoured
	var firstCallAt time.Time
	var secondCallAt time.Time
	sentinelErr := fmt.Errorf("sentinel: second call succeeded")

	calls := 0
	check := &TestCheck{
		fn: func() error {
			calls++
			if calls == 1 {
				firstCallAt = time.Now()
				return &gh.GitHubAbuseRateLimitError{
					RetryAfter: retryAfter,
				}
			}
			secondCallAt = time.Now()
			return sentinelErr
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, interval)

	assert.Equal(t, sentinelErr, err, "expected sentinel error from second call, not the abuse rate limit error")
	assert.Equal(t, 2, calls)
	assert.GreaterOrEqual(t, secondCallAt.Sub(firstCallAt), retryAfter, "second call should not happen before RetryAfter elapses")
}

// TestAbuseRateLimitFallsBackToInterval tests that RunUntilCancelledOrTimeout
// falls back to the recheck interval when RetryAfter is zero (both Retry-After
// and X-RateLimit-Reset headers were absent from the GitHub response).
func TestAbuseRateLimitFallsBackToInterval(t *testing.T) {
	ctx := t.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	interval := 50 * time.Millisecond
	var firstCallAt time.Time
	var secondCallAt time.Time
	sentinelErr := fmt.Errorf("sentinel: second call succeeded")

	calls := 0
	check := &TestCheck{
		fn: func() error {
			calls++
			if calls == 1 {
				firstCallAt = time.Now()
				return &gh.GitHubAbuseRateLimitError{
					RetryAfter: 0, // both headers absent
				}
			}
			secondCallAt = time.Now()
			return sentinelErr
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, interval)

	assert.Equal(t, sentinelErr, err, "expected sentinel error from second call, not the abuse rate limit error")
	assert.Equal(t, 2, calls)
	assert.GreaterOrEqual(t, secondCallAt.Sub(firstCallAt), interval, "second call should not happen before interval elapses")
}

// TestPrimaryRateLimitContextCancellation tests that if the context is already
// cancelled when a GitHubRateLimitError is encountered, the function exits with
// "Timeout reached" and does not retry.
func TestPrimaryRateLimitContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel right away

	interval := 50 * time.Millisecond
	calls := 0
	check := &TestCheck{
		fn: func() error {
			calls++
			return &gh.GitHubRateLimitError{
				ResetTime: time.Now().Add(2 * time.Second), // far enough away that ticker can't race ctx.Done
			}
		},
	}

	err := RunUntilCancelledOrTimeout(ctx, testLogger, check, interval)

	assert.EqualError(t, err, "Timeout reached")
	assert.Equal(t, 1, calls, "Check should be called once but not retried after cancellation")
}

// TestAbuseRateLimitContextCancellation tests that if the context is already
// cancelled when a GitHubAbuseRateLimitError is encountered, the function exits
// with "Timeout reached" and does not retry.
func TestAbuseRateLimitContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel right away

	interval := 50 * time.Millisecond
	calls := 0
	check := &TestCheck{
		fn: func() error {
			calls++
			return &gh.GitHubAbuseRateLimitError{
				RetryAfter: time.Second, // far enough away that ticker can't race ctx.Done
			}
		},
	}

	err := RunUntilCancelledOrTimeout(ctx, testLogger, check, interval)

	assert.EqualError(t, err, "Timeout reached")
	assert.Equal(t, 1, calls, "Check should be called once but not retried after cancellation")
}

func TestGlobalTimeout(t *testing.T) {
	ctx := t.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 0)
	defer cancel()

	check := &TestCheck{
		fn: func() error {
			return nil
		},
	}

	err := RunUntilCancelledOrTimeout(timeoutCtx, testLogger, check, 1*time.Millisecond)

	assert.EqualError(t, err, "Timeout reached")
}
