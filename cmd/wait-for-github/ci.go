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
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/utils"
	"github.com/urfave/cli/v2"

	log "github.com/sirupsen/logrus"
)

type ciConfig struct {
	owner string
	repo  string
	ref   string

	// options
	checks []string
}

var (
	// https://regex101.com/r/dqMmDP/1
	commitRegexp = regexp.MustCompile(`.*github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/commit/(?P<commit>[abcdef\d]+)/?.*`)
)

func parseCIArguments(c *cli.Context) (ciConfig, error) {
	var owner, repo, ref string

	switch {
	// If a single argument is provided, it is expected to be a commit URL
	case c.NArg() == 1:
		url := c.Args().Get(0)
		match := commitRegexp.FindStringSubmatch(url)
		if match == nil {
			return ciConfig{}, fmt.Errorf("invalid commit URL: %s", url)
		}

		owner = match[1]
		repo = match[2]
		ref = match[3]
	// If three arguments are provided, they are expected to be owner, repo, and ref
	case c.NArg() == 3:
		owner = c.Args().Get(0)
		repo = c.Args().Get(1)
		ref = c.Args().Get(2)
	// Any other number of arguments is an error
	default:
		// If the number of arguments is not as expected, show the usage message and error out
		// I think we should be able to do `cli.ShowCommandHelp(c, "ci")` here,
		// but it doesn't work, says "unknown command ci". So we go through the parent command.
		lineage := c.Lineage()
		parent := lineage[1]
		cli.ShowCommandHelpAndExit(parent, "ci", 1)

		// shouldn't get here, the previous line should exit
		return ciConfig{}, nil
	}

	return ciConfig{
		owner:  owner,
		repo:   repo,
		ref:    ref,
		checks: c.StringSlice("check"),
	}, nil
}

func handleCIStatus(status github.CIStatus, recheckInterval time.Duration) cli.ExitCoder {
	switch status {
	case github.CIStatusUnknown:
		log.Infof("CI status is unknown, rechecking in %s", recheckInterval)
	case github.CIStatusPending:
	case github.CIStatusPassed:
		return cli.Exit("CI successful", 0)
	case github.CIStatusFailed:
		return cli.Exit("CI failed", 1)
	}

	log.Infof("CI is not finished yet, rechecking in %s", recheckInterval)
	return nil
}

type checkAllCI struct {
	githubClient github.CheckCIStatus
	owner        string
	repo         string
	ref          string
}

func (ci checkAllCI) Check(ctx context.Context, recheckInterval time.Duration) error {
	status, err := ci.githubClient.GetCIStatus(ctx, ci.owner, ci.repo, ci.ref)
	if err != nil {
		return err
	}

	return handleCIStatus(status, recheckInterval)
}

type checkSpecificCI struct {
	// same field as checkAllCI, and also a list of checks to wait for
	checkAllCI
	checks []string
}

func (ci checkSpecificCI) Check(ctx context.Context, recheckInterval time.Duration) error {
	var status github.CIStatus

	status, interestingChecks, err := ci.githubClient.GetCIStatusForChecks(ctx, ci.owner, ci.repo, ci.ref, ci.checks)
	if err != nil {
		return err
	}

	if status == github.CIStatusFailed {
		log.Infof("CI check %s failed, not waiting for other checks", strings.Join(interestingChecks, ", "))
	}

	// we didn't find any failed checks, and not all checks are finished, so
	// we need to recheck
	if status == github.CIStatusPending {
		log.Infof("CI checks are not finished yet (still waiting for %s), rechecking in %s", strings.Join(interestingChecks, ", "), recheckInterval)
	}

	return handleCIStatus(status, recheckInterval)
}

func checkCIStatus(timeoutCtx context.Context, cfg *config, ciConf *ciConfig) error {
	githubClient, err := github.NewGithubClient(timeoutCtx, cfg.AuthInfo)
	if err != nil {
		return err
	}

	log.Infof("Checking CI status on %s/%s@%s", ciConf.owner, ciConf.repo, ciConf.ref)

	all := checkAllCI{
		githubClient: githubClient,
		owner:        ciConf.owner,
		repo:         ciConf.repo,
		ref:          ciConf.ref,
	}

	specific := checkSpecificCI{
		checkAllCI: all,
		checks:     ciConf.checks,
	}

	if len(ciConf.checks) > 0 {
		log.Infof("Checking CI status for checks: %s", strings.Join(ciConf.checks, ", "))
		return utils.RunUntilCancelledOrTimeout(timeoutCtx, specific, cfg.recheckInterval)
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, all, cfg.recheckInterval)
}

func ciCommand(cfg *config) *cli.Command {
	var ciConf ciConfig

	return &cli.Command{
		Name:      "ci",
		Usage:     "Wait for CI to be finished",
		ArgsUsage: "<https://github.com/OWNER/REPO/commit/HASH|owner> [<repo> <ref>]",
		Before: func(c *cli.Context) error {
			var err error
			ciConf, err = parseCIArguments(c)

			return err
		},
		Action: func(c *cli.Context) error { return checkCIStatus(c.Context, cfg, &ciConf) },
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name: "check",
				Aliases: []string{
					"c",
				},
				Usage: "Check the status of a specific CI check. " +
					"By default, the status of all checks is checked.",
				EnvVars: []string{
					"GITHUB_CI_CHECKS",
				},
			},
		},
	}
}
