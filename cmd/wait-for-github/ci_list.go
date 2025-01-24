// wait-for-github
// Copyright (C) 2025, Grafana Labs

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
	"io"
	"os"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/mattn/go-isatty"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type checkListConfig struct {
	ciConfig
	githubClient github.GetDetailedCIStatus
}

var caser = cases.Title(language.English)

func statusColor(check github.CICheckStatus) tablewriter.Colors {
	switch check.Outcome() {
	case github.CIStatusPassed:
		return tablewriter.Colors{tablewriter.FgGreenColor}
	case github.CIStatusFailed:
		return tablewriter.Colors{tablewriter.FgRedColor}
	case github.CIStatusPending:
		return tablewriter.Colors{tablewriter.FgYellowColor}
	case github.CIStatusSkipped:
		return tablewriter.Colors{tablewriter.FgHiBlackColor}
	default:
		return tablewriter.Colors{tablewriter.FgWhiteColor}
	}
}

// tableWriter is a wrapper around the tablewriter library, provided so that
// tests can mock the table writing process
type tableWriter interface {
	SetHeader(headers []string)
	Rich(row []string, colors []tablewriter.Colors)
	Render()
}

func newTableWriter(w io.Writer) (tableWriter, error) {
	tw := tablewriter.NewWriter(w)
	tw.SetAutoWrapText(false)

	if err := tw.SetUnicodeHV(tablewriter.Double, tablewriter.Regular); err != nil {
		return nil, err
	}

	return tw, nil
}

func listChecks(ctx context.Context, cfg *checkListConfig, table tableWriter, useColors bool) error {
	checks, err := cfg.githubClient.GetDetailedCIStatus(ctx, cfg.owner, cfg.repo, cfg.ref)
	if err != nil {
		return err
	}

	if len(checks) == 0 {
		// For empty results, still use the table but with a single message row
		table.SetHeader([]string{"Status"})
		table.Rich([]string{"No CI checks found"}, nil)
		table.Render()

		return nil
	}

	table.SetHeader([]string{"Name", "Type", "Status"})

	for _, check := range checks {
		var colors []tablewriter.Colors
		if useColors {
			colors = []tablewriter.Colors{
				{},
				{},
				statusColor(check),
			}
		}

		table.Rich([]string{
			check.String(),
			check.Type(),
			caser.String(check.Outcome().String()),
		}, colors)
	}

	table.Render()
	return nil
}

func ciListCommand(cfg *config) *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List all CI checks and their status",
		ArgsUsage: "<https://github.com/OWNER/REPO/commit|pull/HASH|PRNumber|owner> [<repo> <ref>]",
		Action: func(c *cli.Context) error {
			ciConf, err := parseCIArguments(c, "list")
			if err != nil {
				return err
			}

			githubClient, err := github.NewGithubClient(c.Context, cfg.AuthInfo, cfg.pendingRecheckTime)
			if err != nil {
				return err
			}

			w := os.Stdout
			table, err := newTableWriter(w)
			if err != nil {
				return err
			}

			useColor := isatty.IsTerminal(w.Fd()) && os.Getenv("NO_COLOR") == ""

			return listChecks(c.Context, &checkListConfig{
				ciConfig:     ciConf,
				githubClient: githubClient,
			}, table, useColor)
		},
	}
}
