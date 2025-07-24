// wait-for-github
// Copyright (C) 2022-2023, Grafana Labs

// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option) any
// later version.

// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
// FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License for more
// details.

// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/logging"
	"github.com/urfave/cli/v3"
)

type cmdFunc func(cfg *config) *cli.Command

var defaultLogLevel logging.Level = logging.Level(slog.LevelInfo)

func root() *cli.Command {
	var cfg config

	// give a timeout context to all commands
	var commands []*cli.Command
	for _, cf := range []cmdFunc{ciCommand, prCommand} {
		cmd := cf(&cfg)
		action := cmd.Action
		cmd.Action = func(c context.Context, cmd *cli.Command) error {
			timeoutCtx, cancel := context.WithTimeout(c, cfg.globalTimeout)
			defer cancel()

			return action(timeoutCtx, cmd)
		}
		commands = append(commands, cmd)
	}

	return &cli.Command{
		Name:  "wait-for-github",
		Usage: "Wait for things to happen on GitHub",
		Flags: []cli.Flag{
			&cli.GenericFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				Usage:   fmt.Sprintf("Set the log level. Valid levels are: %s.", strings.Join(validLogLevels(), ", ")),
				Value:   &defaultLogLevel,
				Action: func(ctx context.Context, cmd *cli.Command, value cli.Value) error {
					level := value.(*logging.Level)
					cfg.logger = logging.SetupLogger(slog.Level(*level))

					return nil
				},
			},
			&cli.StringFlag{
				Name:    "github-app-private-key-path",
				Aliases: []string{"p"},
				Usage:   "Path to the GitHub App private key",
			},
			&cli.StringFlag{
				Name:  "github-app-private-key",
				Usage: "Contents of the GitHub App private key",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_APP_PRIVATE_KEY"),
				),
			},
			&cli.Int64Flag{
				Name:  "github-app-id",
				Usage: "GitHub App ID",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_APP_ID"),
				),
			},
			&cli.Int64Flag{
				Name:  "github-app-installation-id",
				Usage: "GitHub App installation ID",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_APP_INSTALLATION_ID"),
				),
			},
			&cli.StringFlag{
				Name: "github-token",
				Usage: "GitHub token. If not provided, the app will try to use the " +
					"GitHub App authentication mechanism.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_TOKEN"),
				),
			},
			&cli.DurationFlag{
				Name:  "recheck-interval",
				Usage: "Interval after which to recheck GitHub.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("RECHECK_INTERVAL"),
				),
				Value: time.Duration(30 * time.Second),
			},
			&cli.DurationFlag{
				Name:  "pending-recheck-time",
				Usage: "Time after which to recheck the pending status on GitHub.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("PENDING_RECHECK_TIME"),
				),
				Value: 5 * time.Second,
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Timeout after which to stop checking GitHub.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("TIMEOUT"),
				),
				Value: time.Duration(7 * 24 * time.Hour),
			},
		},
		Commands: commands,
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			err := initialiseConfig(ctx, cmd, &cfg)
			return ctx, err
		},
	}
}

func validLogLevels() []string {
	return []string{"debug", "info", "warn", "error"}
}

func initialiseConfig(ctx context.Context, cmd *cli.Command, cfg *config) error {
	cfg.logger = logging.SetupLogger(slog.Level(defaultLogLevel))

	cfg.logger.DebugContext(ctx, "debug logging enabled")

	cfg.recheckInterval = cmd.Duration("recheck-interval")
	cfg.pendingRecheckTime = cmd.Duration("pending-recheck-time")
	cfg.globalTimeout = cmd.Duration("timeout")

	token := cmd.String("github-token")
	if token != "" {
		cfg.logger.DebugContext(ctx, "will use github token for authentication")
		cfg.logger.DebugContext(ctx, "using token starting with", "token", token[:10])
		cfg.AuthInfo.GithubToken = token

		return nil
	}

	privateKey := []byte(cmd.String("github-app-private-key"))

	file := cmd.String("github-app-private-key-path")
	if file != "" {
		var err error
		privateKey, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read private key file: %w", err)
		}
	}

	appId := cmd.Int64("github-app-id")
	installationID := cmd.Int64("github-app-installation-id")

	if len(privateKey) == 0 || appId == 0 || installationID == 0 {
		cfg.logger.ErrorContext(ctx, "must provide either a GitHub token or a GitHub app private key, app ID and installation ID")
		cli.ShowAppHelpAndExit(cmd, 1)
	}

	cfg.AuthInfo = github.AuthInfo{
		InstallationID: installationID,
		AppID:          appId,
		PrivateKey:     privateKey,
	}

	return nil
}
