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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/utils"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type prConfig struct {
	owner string
	repo  string
	pr    int
}

var (
	// https://regex101.com/r/nexaWT/1
	pullRequestRegexp = regexp.MustCompile(`.*github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/pull/(?P<number>\d+)/?.*`)
	prConf            prConfig
)

func parsePRArguments(c *cli.Context) error {
	var owner, repo, number string

	switch {
	// If a single argument is provided, it is expected to be a pull request URL
	case c.NArg() == 1:
		url := c.Args().Get(0)
		match := pullRequestRegexp.FindStringSubmatch(url)
		if match == nil {
			return fmt.Errorf("invalid pull request URL: %s", url)
		}

		owner = match[1]
		repo = match[2]
		number = match[3]
	// If three arguments are provided, they are expected to be owner, repo, and PR number
	case c.NArg() == 3:
		owner = c.Args().Get(0)
		repo = c.Args().Get(1)
		number = c.Args().Get(2)
	// Any other number of arguments is an error
	default:
		// If the number of arguments is not as expected, show the usage message and error out
		// I think we should be able to do `cli.ShowCommandHelp(c, "pr")` here,
		// but it doesn't work, says "unknown command pr". So we go through the parent command.
		lineage := c.Lineage()
		parent := lineage[1]
		cli.ShowCommandHelpAndExit(parent, "pr", 1)

		// shouldn't get here, the previous line should exit
		return nil
	}

	prConf.owner = owner
	prConf.repo = repo

	n, err := strconv.Atoi(number)
	if err != nil {
		return fmt.Errorf("PR must be a number, got '%s'", c.Args().Get(2))
	}
	prConf.pr = n

	log.Infof("Waiting for PR %s/%s#%d to be merged/closed", owner, repo, n)

	return nil
}

type commitInfo struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
}

func checkPRMerged(c *cli.Context) error {
	ctx := context.Background()

	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.globalTimeout)
	defer cancel()

	githubClient, err := github.NewGithubClient(ctx, cfg.AuthInfo)
	if err != nil {
		return err
	}

	commitInfoFile := c.String("commit-info-file")

	checkPRMergedOrClosed := func() error {
		mergedCommit, closed, err := githubClient.IsPRMergedOrClosed(ctx, prConf.owner, prConf.repo, prConf.pr)
		if err != nil {
			return err
		}

		if mergedCommit != "" {
			log.Info("PR is merged, exiting")
			if commitInfoFile != "" {
				commit := []commitInfo{
					{
						Owner:  prConf.owner,
						Repo:   prConf.repo,
						Commit: mergedCommit,
					},
				}

				jsonCommit, err := json.MarshalIndent(commit, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal commit info to json: %w", err)
				}

				log.Debugf("Writing commit info to file %s", commitInfoFile)
				if err := ioutil.WriteFile(commitInfoFile, jsonCommit, 0644); err != nil {
					return fmt.Errorf("failed to write commit info to file: %w", err)
				}
			}
			return cli.Exit("PR is merged", 0)
		}

		if closed {
			log.Info("PR is closed, exiting")
			return cli.Exit("PR is closed without being merged", 1)
		}
		log.Infof("PR is not closed yet, rechecking in %s", cfg.recheckInterval)
		return nil
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, checkPRMergedOrClosed, cfg.recheckInterval)
}

var prCommand = &cli.Command{
	Name:      "pr",
	Usage:     "Wait for a PR to be merged",
	ArgsUsage: "<https://github.com/OWNER/REPO/pulls/PR|owner> [<repo> <pr>]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "commit-info-file",
			Usage: "Path to a file which the commit info will be written. " +
				"The file will be overwritten if it already exists.",
		},
	},
	Before: parsePRArguments,
	Action: checkPRMerged,
}
