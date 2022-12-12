// wait-for-github
// Copyright (C) 2022, Grafana Labs

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

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/utils"
	"github.com/urfave/cli/v2"

	log "github.com/sirupsen/logrus"
)

type ciConfig struct {
	owner string
	repo  string
	ref   string
}

var (
	// https://regex101.com/r/dqMmDP/1
	commitRegexp = regexp.MustCompile(`.*github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/commit/(?P<commit>[abcdef\d]+)/?.*`)
	ciConf       ciConfig
)

func parseCIArguments(c *cli.Context) error {
	var owner, repo, ref string

	switch {
	// If a single argument is provided, it is expected to be a commit URL
	case c.NArg() == 1:
		url := c.Args().Get(0)
		match := commitRegexp.FindStringSubmatch(url)
		if match == nil {
			return fmt.Errorf("invalid pull request URL: %s", url)
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
		return nil
	}

	ciConf.owner = owner
	ciConf.repo = repo
	ciConf.ref = ref

	log.Debugf("Will wait for CI on %s/%s@%s", owner, repo, ref)

	return nil
}

func checkCIStatus(c *cli.Context) error {
	ctx := context.Background()

	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.globalTimeout)
	defer cancel()

	githubClient, err := github.NewGithubClient(ctx, cfg.AuthInfo)
	if err != nil {
		return err
	}

	log.Infof("Checking CI status on %s/%s@%s", ciConf.owner, ciConf.repo, ciConf.ref)

	checkCI := func() error {
		status, err := githubClient.GetCIStatus(ctx, ciConf.owner, ciConf.repo, ciConf.ref)
		if err != nil {
			return err
		}

		switch status {
		case github.CIStatusUnknown:
			log.Infof("CI status is unknown, rechecking in %s", cfg.recheckInterval)
		case github.CIStatusPending:
		case github.CIStatusPassed:
			return cli.Exit("CI successful", 0)
		case github.CIStatusFailed:
			return cli.Exit("CI failed", 1)
		}

		log.Infof("CI is not finished yet, rechecking in %s", cfg.recheckInterval)
		return nil
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, checkCI, cfg.recheckInterval)
}

var ciCommand = &cli.Command{
	Name:      "ci",
	Usage:     "Wait for CI to be finished",
	ArgsUsage: "<https://github.com/OWNER/REPO/commit/HASH|owner> [<repo> <ref>]",
	Before:    parseCIArguments,
	Action:    checkCIStatus,
}
