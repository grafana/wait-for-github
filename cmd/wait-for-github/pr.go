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
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/grafana/wait-for-github/internal/utils"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
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

func parsePRArguments(c *cli.Context) (prConfig, error) {
	var owner, repo, number string

	switch {
	// If a single argument is provided, it is expected to be a pull request URL
	case c.NArg() == 1:
		url := c.Args().Get(0)
		match := pullRequestRegexp.FindStringSubmatch(url)
		if match == nil {
			return prConfig{}, ErrInvalidPRURL{url}
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
		err := cli.ShowCommandHelp(parent, "pr")
		if err != nil {
			return prConfig{}, err
		}

		return prConfig{}, cli.Exit("invalid number of arguments", 1)
	}

	n, err := strconv.Atoi(number)
	if err != nil {
		return prConfig{}, fmt.Errorf("PR must be a number, got '%s'", c.Args().Get(2))
	}
	log.Infof("Waiting for PR %s/%s#%d to be merged/closed", owner, repo, n)

	return prConfig{
		owner:          owner,
		repo:           repo,
		pr:             n,
		commitInfoFile: c.String("commit-info-file"),
		writer: 	   osFileWriter{},
	}, nil
}

type commitInfo struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	MergedAt int64  `json:"mergedAt"`
}

type prCheck struct {
	prConfig

	githubClient github.CheckPRMerged
}

func (pr prCheck) Check(ctx context.Context, recheckInterval time.Duration) error {
	mergedCommit, closed, mergedAt, err := pr.githubClient.IsPRMergedOrClosed(ctx, pr.owner, pr.repo, pr.pr)
	if err != nil {
		return err
	}

	if mergedCommit != "" {
		log.Info("PR is merged, exiting")
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

			log.Debugf("Writing commit info to file %s", pr.commitInfoFile)
			if err := pr.writer.WriteFile(pr.commitInfoFile, jsonCommit, 0644); err != nil {
				return fmt.Errorf("failed to write commit info to file: %w", err)
			}
		}
		return cli.Exit("PR is merged", 0)
	}

	if closed {
		return cli.Exit("PR is closed", 1)
	}

	log.Infof("PR is not closed yet, rechecking in %s", recheckInterval)
	return nil
}

func checkPRMerged(timeoutCtx context.Context, githubClient github.CheckPRMerged, cfg *config, prConf *prConfig) error {
	checkPRMergedOrClosed := prCheck{
		githubClient: githubClient,
		prConfig:     *prConf,
	}

	return utils.RunUntilCancelledOrTimeout(timeoutCtx, checkPRMergedOrClosed, cfg.recheckInterval)
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
		},
		Before: func(c *cli.Context) error {
			var err error
			prConf, err = parsePRArguments(c)

			return err
		},
		Action: func(c *cli.Context) error {
			githubClient, err := github.NewGithubClient(c.Context, cfg.AuthInfo)
			if err != nil {
				return err
			}
			return checkPRMerged(c.Context, githubClient, cfg, &prConf)
		},
	}
}
