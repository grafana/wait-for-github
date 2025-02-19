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

func TestGlobalTimeout(t *testing.T) {
	ctx := context.Background()
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
