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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"log/slog"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/utils"
	"github.com/urfave/cli/v3"
)

// for testing. We could use `io.Writer`, but then we have to handle opening and
// closing the file.
type fileWriter interface {
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type osFileWriter struct{}

func (f osFileWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

type prConfig struct {
	owner string
	repo  string
	pr    int

	commitInfoFile string
	excludes       []string
	writer         fileWriter
}

var (
	// https://regex101.com/r/nexaWT/1
	pullRequestRegexp = regexp.MustCompile(`.*github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/pull/(?P<number>\d+)/?.*`)
)

type ErrInvalidPRURL struct {
	url string
}

func (e ErrInvalidPRURL) Error() string {
	return fmt.Sprintf("invalid pull request URL: %s", e.url)
}

func extractNumberFromPrURL(url string) (owner, repo, number string) {
	match := pullRequestRegexp.FindStringSubmatch(url)
	if match == nil {
		return owner, repo, number
	}

	owner = match[1]
	repo = match[2]
	number = match[3]
	return owner, repo, number
}

func parsePRArguments(ctx context.Context, cmd *cli.Command, logger *slog.Logger) (prConfig, error) {
	var owner, repo, number string

	switch {
	// If a single argument is provided, it is expected to be a pull request URL
	case cmd.NArg() == 1:
		url := cmd.Args().Get(0)
		owner, repo, number = extractNumberFromPrURL(url)

		if len(number) == 0 {
			return prConfig{}, ErrInvalidPRURL{url}
		}
	// If three arguments are provided, they are expected to be owner, repo, and PR number
	case cmd.NArg() == 3:
		owner = cmd.Args().Get(0)
		repo = cmd.Args().Get(1)
		number = cmd.Args().Get(2)
	// Any other number of arguments is an error
	default:
		// If the number of arguments is not as expected, show the usage message and error out
		// I think we should be able to do `cli.ShowCommandHelp(c, "pr")` here,
		// but it doesn't work, says "unknown command pr". So we go through the parent command.
		lineage := cmd.Lineage()
		parent := lineage[1]
		err := cli.ShowCommandHelp(ctx, parent, "pr")
		if err != nil {
			return prConfig{}, err
		}

		return prConfig{}, cli.Exit("invalid number of arguments", 1)
	}

	n, err := strconv.Atoi(number)
	if err != nil {
		return prConfig{}, fmt.Errorf("PR must be a number, got '%s'", cmd.Args().Get(2))
	}
	logger.InfoContext(ctx, "waiting for PR to be merged/closed", "owner", owner, "repo", repo, "pr", n)

	return prConfig{
		owner:          owner,
		repo:           repo,
		pr:             n,
		commitInfoFile: cmd.String("commit-info-file"),
		excludes:       cmd.StringSlice("exclude"),
		writer:         osFileWriter{},
	}, nil
}

type commitInfo struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	MergedAt int64  `json:"mergedAt"`
}

type checkMergedAndOverallCI interface {
	github.CheckPRMerged
	github.GetPRHeadSHA
	github.CheckOverallCIStatus
}

type prCheck struct {
	prConfig
	githubClient checkMergedAndOverallCI
	logger       *slog.Logger
}

func (pr prCheck) Check(ctx context.Context) error {
	mergedCommit, closed, mergedAt, err := pr.githubClient.IsPRMergedOrClosed(ctx, pr.owner, pr.repo, pr.pr)
	if err != nil {
		return err
	}

	if mergedCommit != "" {
		pr.logger.InfoContext(ctx, "PR is merged, exiting")
		if pr.commitInfoFile != "" {
			commit := commitInfo{
				Owner:    pr.owner,
				Repo:     pr.repo,
				Commit:   mergedCommit,
				MergedAt: mergedAt,
			}

			jsonCommit, err := json.MarshalIndent(commit, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal commit info to json: %w", err)
			}

			pr.logger.DebugContext(ctx, "writing commit info to file", "file", pr.commitInfoFile)
			if err := pr.writer.WriteFile(pr.commitInfoFile, jsonCommit, 0644); err != nil {
				return fmt.Errorf("failed to write commit info to file: %w", err)
			}
		}
		return cli.Exit("PR is merged", 0)
	}

	if closed {
		return cli.Exit("PR is closed", 1)
	}

	// not merged, not closed, let's see what the CI status is. If that's bad,
	// we can exit early.
	sha, err := pr.githubClient.GetPRHeadSHA(ctx, pr.owner, pr.repo, pr.pr)
	if err != nil {
		return err
	}

	status, err := pr.githubClient.GetCIStatus(ctx, pr.owner, pr.repo, sha, pr.excludes)
	if err != nil {
		return err
	}

	if status == github.CIStatusFailed {
		pr.logger.InfoContext(ctx, "CI failed, exiting")
		return cli.Exit("CI failed", 1)
	}

	pr.logger.InfoContext(ctx, "PR is not closed yet")
	return nil
}

func checkPRMerged(timeoutCtx context.Context, githubClient checkMergedAndOverallCI, cfg *config, prConf *prConfig) error {
	checkPRMergedOrClosed := prCheck{
		githubClient: githubClient,
		prConfig:     *prConf,
		logger:       cfg.logger,
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, cfg.logger, checkPRMergedOrClosed, cfg.recheckInterval)
}

func prCommand(cfg *config) *cli.Command {
	var prConf prConfig

	return &cli.Command{
		Name:      "pr",
		Usage:     "Wait for a PR to be merged",
		ArgsUsage: "<https://github.com/OWNER/REPO/pulls/PR|owner> [<repo> <pr>]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "commit-info-file",
				Usage: "Path to a file which the commit info will be written. " +
					"The file will be overwritten if it already exists.",
			},
			&cli.StringSliceFlag{
				Name: "exclude",
				Aliases: []string{
					"x",
				},
				Usage: "Exclude the status of a specific CI check from failing the wait. " +
					"By default, a failed status check will exit the pr wait command.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_CI_EXCLUDE"),
				),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			var err error
			prConf, err = parsePRArguments(ctx, cmd, cfg.logger)

			return ctx, err
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			githubClient, err := github.NewGithubClient(ctx, cfg.logger, cfg.AuthInfo, cfg.pendingRecheckTime)
			if err != nil {
				return err
			}
			return checkPRMerged(ctx, githubClient, cfg, &prConf)
		},
	}
}
