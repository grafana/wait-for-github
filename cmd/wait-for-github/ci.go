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
	"regexp"
	"strings"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/logging"
	"github.com/grafana/wait-for-github/internal/utils"
	"github.com/urfave/cli/v3"

	"log/slog"
)

type ciConfig struct {
	owner string
	repo  string
	ref   string

	// options
	checks   []string
	excludes []string
}

var (
	// https://regex101.com/r/dqMmDP/1
	commitRegexp = regexp.MustCompile(`.*github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/commit/(?P<commit>[abcdef\d]+)/?.*`)
)

type ErrInvalidURL struct {
	url string
}

func (e ErrInvalidURL) Error() string {
	return fmt.Sprintf("invalid URL to either PR or commit: %s", e.url)
}

func extractRefFromCommitURL(url string) (owner, repo, ref string) {
	match := commitRegexp.FindStringSubmatch(url)
	if match == nil {
		return
	}

	owner = match[1]
	repo = match[2]
	ref = match[3]
	return
}

func extractRefFromPrURL(url string) (owner, repo, ref string) {
	owner, repo, number := extractNumberFromPrURL(url)
	if number == "" {
		return owner, repo, ref
	}
	// Construct Ref with PR number
	ref = fmt.Sprintf("refs/pull/%s/head", number)
	return owner, repo, ref
}

func parseCIArguments(ctx context.Context, cmd *cli.Command, logger *slog.Logger, command string) (ciConfig, error) {
	var owner, repo, ref string

	logger.DebugContext(ctx, "parseCIArguments", "args", cmd.Args(), "nArgs", cmd.NArg())

	args := cmd.Args()

	switch nArgs := cmd.NArg(); nArgs {
	// If a single argument is provided, it is expected to be either a commit URL or PR URL
	case 1:
		url := args.Get(0)
		// Try for a PR URL
		owner, repo, ref = extractRefFromPrURL(url)
		if len(ref) == 0 {
			// Try for a commit URL
			owner, repo, ref = extractRefFromCommitURL(url)
		}

		// Neither URL parsed
		if len(ref) == 0 {
			return ciConfig{}, ErrInvalidURL{url}
		}

	// If three arguments are provided, they are expected to be owner, repo, and ref
	case 3:
		owner = args.Get(0)
		repo = args.Get(1)
		ref = args.Get(2)
	// Any other number of arguments is an error
	default:
		// If the number of arguments is not as expected, show the usage message and error out
		// I think we should be able to do `cli.ShowCommandHelp(c, "ci")` here,
		// but it doesn't work, says "unknown command ci". So we go through the parent command.
		lineage := cmd.Lineage()
		parent := lineage[1]
		err := cli.ShowCommandHelp(ctx, parent, command)
		if err != nil {
			return ciConfig{}, err
		}

		return ciConfig{}, cli.Exit("invalid number of arguments", 1)
	}

	return ciConfig{
		owner:    owner,
		repo:     repo,
		ref:      ref,
		checks:   cmd.StringSlice("check"),
		excludes: cmd.StringSlice("exclude"),
	}, nil
}

func handleCIStatus(logger *slog.Logger, status github.CIStatus, url string) cli.ExitCoder {
	switch status {
	case github.CIStatusUnknown:
		logger.Info("CI status is unknown")
	case github.CIStatusPending:
	case github.CIStatusPassed:
		return cli.Exit("CI successful", 0)
	case github.CIStatusFailed:
		return cli.Exit(fmt.Sprintf("CI failed. Please check CI on the following commit: %s", url), 1)
	}

	logger.Info("CI is not finished yet")
	return nil
}

type checkAllCI struct {
	githubClient github.CheckCIStatus
	owner        string
	repo         string
	ref          string
	excludes     []string
	logger       *slog.Logger
}

func (ci checkAllCI) Check(ctx context.Context) error {
	status, err := ci.githubClient.GetCIStatus(ctx, ci.owner, ci.repo, ci.ref, ci.excludes)
	if err != nil {
		return err
	}

	return handleCIStatus(ci.logger, status, urlFor(ci.owner, ci.repo, ci.ref))
}

type checkSpecificCI struct {
	// same field as checkAllCI, and also a list of checks to wait for
	checkAllCI
	checks []string
}

func (ci checkSpecificCI) Check(ctx context.Context) error {
	var status github.CIStatus

	status, interestingChecks, err := ci.githubClient.GetCIStatusForChecks(ctx, ci.owner, ci.repo, ci.ref, ci.checks)
	if err != nil {
		return err
	}

	if status == github.CIStatusFailed {
		ci.logger.InfoContext(ctx, "CI check failed, not waiting for other checks", "failed_checks", strings.Join(interestingChecks, ", "))
	}

	// we didn't find any failed checks, and not all checks are finished, so
	// we need to recheck
	if status == github.CIStatusPending {
		ci.logger.InfoContext(ctx, "CI checks are not finished yet", "waiting_for", strings.Join(interestingChecks, ", "))
	}

	return handleCIStatus(ci.logger, status, urlFor(ci.owner, ci.repo, ci.ref))
}

func checkCIStatus(timeoutCtx context.Context, githubClient github.CheckCIStatus, cfg *config, ciConf *ciConfig) error {
	logger := cfg.logger.With(logging.OwnerAttr(ciConf.owner), logging.RepoAttr(ciConf.repo), logging.RefAttr(ciConf.ref))
	logger.InfoContext(timeoutCtx, "checking CI status")

	all := checkAllCI{
		githubClient: githubClient,
		owner:        ciConf.owner,
		repo:         ciConf.repo,
		ref:          ciConf.ref,
		excludes:     ciConf.excludes,
		logger:       cfg.logger,
	}

	specific := checkSpecificCI{
		checkAllCI: all,
		checks:     ciConf.checks,
	}

	if len(ciConf.checks) > 0 {
		logger.InfoContext(timeoutCtx, "checking CI status for checks", "checks", strings.Join(ciConf.checks, ", "))
		return utils.RunUntilCancelledOrTimeout(timeoutCtx, cfg.logger, specific, cfg.recheckInterval)
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, cfg.logger, all, cfg.recheckInterval)
}

func ciCommand(cfg *config) *cli.Command {
	return &cli.Command{
		Name:      "ci",
		Usage:     "Wait for CI to be finished",
		ArgsUsage: "<https://github.com/OWNER/REPO/commit|pull/HASH|PRNumber|owner> [<repo> <ref>]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ciConf, err := parseCIArguments(ctx, cmd, cfg.logger, "ci")
			if err != nil {
				return err
			}

			githubClient, err := github.NewGithubClient(ctx, cfg.logger, cfg.AuthInfo, cfg.pendingRecheckTime)
			if err != nil {
				return err
			}

			return checkCIStatus(ctx, githubClient, cfg, &ciConf)
		},
		Commands: []*cli.Command{
			ciListCommand(cfg),
		},
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name: "check",
				Aliases: []string{
					"c",
				},
				Usage: "Check the status of a specific CI check. " +
					"By default, the status of all checks is checked.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_CI_CHECKS"),
				),
			},
			&cli.StringSliceFlag{
				Name: "exclude",
				Aliases: []string{
					"x",
				},
				Usage: "Exclude the status of a specific CI check. " +
					"Argument ignored if checks are specified individually. " +
					"By default, the status of all checks is checked.",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("GITHUB_CI_EXCLUDE"),
				),
			},
		},
	}
}

func urlFor(owner, repo, ref string) string {
	return fmt.Sprintf("https://github.com/%s/%s/commit/%s", owner, repo, ref)
}
