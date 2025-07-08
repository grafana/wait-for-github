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
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/grafana/wait-for-github/internal/ansi"
	"github.com/grafana/wait-for-github/internal/github"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/urfave/cli/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

type checkListConfig struct {
	ciConfig
	githubClient github.GetDetailedCIStatus
}

// tableWriter is a wrapper around the tablewriter library, provided so that
// tests can mock the table writing process
type tableWriter interface {
	Header(headers ...any)
	Bulk(data any) error
	Render() error
}

type realTableWriter = tablewriter.Table

var _ tableWriter = (*realTableWriter)(nil)

func newTableWriter(w io.Writer) (tableWriter, error) {
	colorConfig := renderer.ColorizedConfig{
		Header: renderer.Tint{
			FG: renderer.Colors{color.FgBlack, color.Bold},
		},
		Border:    renderer.Tint{FG: renderer.Colors{color.FgWhite}},
		Separator: renderer.Tint{FG: renderer.Colors{color.FgWhite}},

		Column: renderer.Tint{BG: renderer.Colors{color.Reset}},
	}

	table := tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewColorized(
			colorConfig,
		)),
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Formatting: tw.CellFormatting{
					AutoWrap: tw.WrapNone,
				},
			},
		}),
	)

	return table, nil
}

func listChecks(ctx context.Context, cfg *checkListConfig, table tableWriter) error {
	checks, err := cfg.githubClient.GetDetailedCIStatus(ctx, cfg.owner, cfg.repo, cfg.ref)
	if err != nil {
		return err
	}

	if len(checks) == 0 {
		// For empty results, still use the table but with a single message row
		table.Header([]string{"Status"})

		if err := table.Bulk([]string{"No CI checks found"}); err != nil {
			return fmt.Errorf("failed to write table: %w", err)
		}

		if err := table.Render(); err != nil {
			return fmt.Errorf("failed to render table: %w", err)
		}

		return nil
	}

	table.Header([]string{"Name", "Type", "Status"})

	var data [][]string
	for _, check := range checks {
		caser := ansi.NewANSITransformer(cases.Title(language.English))
		checkOutcomeString, _, _ := transform.String(caser, check.Outcome().String())

		data = append(data, []string{
			check.String(),
			check.Type(),
			checkOutcomeString,
		})
	}

	if err := table.Bulk(data); err != nil {
		return fmt.Errorf("failed to write table: %w", err)
	}

	return table.Render()
}

func ciListCommand(cfg *config) *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List all CI checks and their status",
		ArgsUsage: "<https://github.com/OWNER/REPO/commit|pull/HASH|PRNumber|owner> [<repo> <ref>]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ciConf, err := parseCIArguments(ctx, cmd, cfg.logger, "list")
			if err != nil {
				return err
			}

			githubClient, err := github.NewGithubClient(ctx, cfg.logger, cfg.AuthInfo, cfg.pendingRecheckTime)
			if err != nil {
				return err
			}

			w := os.Stdout
			table, err := newTableWriter(w)
			if err != nil {
				return err
			}

			return listChecks(ctx, &checkListConfig{
				ciConfig:     ciConf,
				githubClient: githubClient,
			}, table)
		},
	}
}
