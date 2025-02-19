package logging

import (
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/term"

	"github.com/lmittmann/tint"
	"github.com/willabides/actionslog"
	"github.com/willabides/actionslog/human"
)

type Level slog.Level

func (l *Level) UnmarshalFlag(value string) error {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(value)); err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	*l = Level(lvl)
	return nil
}

func (l *Level) Set(value string) error {
	return l.UnmarshalFlag(value)
}

func (l Level) String() string {
	return slog.Level(l).String()
}

func SetupLogger(defaultLevel slog.Level) {
	var lv slog.LevelVar
	lv.Set(defaultLevel)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &lv,
	}))
	defer func() { slog.SetDefault(logger) }()

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		logger = slog.New(&actionslog.Wrapper{
			Handler: (&human.Handler{
				AddSource:   true,
				ExcludeTime: true,
				Level:       &lv,
			}).WithOutput,
			Output: os.Stderr,
		})
		if os.Getenv("RUNNER_DEBUG") == "1" {
			lv.Set(slog.LevelDebug)
		}
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		logger = slog.New(tint.NewHandler(os.Stderr, &tint.Options{
			AddSource: true,
			Level:     &lv,
		}))
	}
}
