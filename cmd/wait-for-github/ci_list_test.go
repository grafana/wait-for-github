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
	"testing"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/olekukonko/tablewriter"
	"github.com/stretchr/testify/require"
)

// mockTableWriter records what was written to the table
type mockTableWriter struct {
	headers []string
	rows    []struct {
		row    []string
		colors []tablewriter.Colors
	}
}

func (m *mockTableWriter) SetHeader(headers []string) {
	m.headers = headers
}

func (m *mockTableWriter) Rich(row []string, colors []tablewriter.Colors) {
	m.rows = append(m.rows, struct {
		row    []string
		colors []tablewriter.Colors
	}{row, colors})
}

func (m *mockTableWriter) Render() {
	// Do nothing in tests
}

// FakeListCIStatusChecker implements the CheckCIStatus interface for testing.
type FakeListCIStatusChecker struct {
	checks []github.CICheckStatus
	err    error
}

func (c *FakeListCIStatusChecker) GetDetailedCIStatus(ctx context.Context, owner, repo string, commitHash string) ([]github.CICheckStatus, error) {
	return c.checks, c.err
}

func TestListChecks(t *testing.T) {
	tests := []struct {
		name       string
		checks     []github.CICheckStatus
		err        error
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "successfully lists checks",
			checks: []github.CICheckStatus{
				github.CheckRun{
					Name:       "test",
					Status:     "COMPLETED",
					Conclusion: "SUCCESS",
				},
				github.StatusContext{
					Context: "deploy",
					State:   "PENDING",
				},
			},
			wantErr: false,
		},
		{
			name:    "handles no checks",
			checks:  []github.CICheckStatus{},
			wantErr: false,
		},
		{
			name:       "handles error",
			err:        fmt.Errorf("test error"),
			wantErr:    true,
			wantErrMsg: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := &mockTableWriter{}
			cfg := &checkListConfig{
				ciConfig: ciConfig{
					owner: "owner",
					repo:  "repo",
					ref:   "ref",
				},
				githubClient: &FakeListCIStatusChecker{
					checks: tt.checks,
					err:    tt.err,
				},
			}

			err := listChecks(context.TODO(), cfg, table, true)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestListChecks_TableRendering(t *testing.T) {
	tests := []struct {
		name        string
		checks      []github.CICheckStatus
		isTTY       bool
		wantHeaders []string
		wantRows    [][]string
		wantColors  [][]tablewriter.Colors
	}{
		{
			name: "renders colors in TTY",
			checks: []github.CICheckStatus{
				github.CheckRun{
					Name:       "test",
					Status:     "COMPLETED",
					Conclusion: "SUCCESS",
				},
			},
			isTTY:       true,
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"test", "Check Run", "Passed"},
			},
			wantColors: [][]tablewriter.Colors{
				{
					{},
					{},
					{tablewriter.FgGreenColor},
				},
			},
		},
		{
			name: "no colors in non-TTY",
			checks: []github.CICheckStatus{
				github.CheckRun{
					Name:       "test",
					Status:     "COMPLETED",
					Conclusion: "FAILURE",
				},
			},
			isTTY:       false,
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"test", "Check Run", "Failed"},
			},
			wantColors: [][]tablewriter.Colors{nil},
		},
		{
			name: "multiple checks with different statuses",
			checks: []github.CICheckStatus{
				github.CheckRun{
					Name:       "build",
					Status:     "COMPLETED",
					Conclusion: "SUCCESS",
					CheckSuite: github.CheckSuiteInfo{
						App: github.AppInfo{
							Name: "",
						},
						WorkflowRun: struct {
							Workflow github.WorkflowInfo
						}{
							Workflow: github.WorkflowInfo{
								Name: "CI",
							},
						},
					},
				},
				github.StatusContext{
					Context: "deploy",
					State:   "PENDING",
				},
				github.CheckRun{
					Name:       "test",
					Status:     "COMPLETED",
					Conclusion: "SKIPPED",
				},
			},
			isTTY:       true,
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"CI / build", "Check Run", "Passed"},
				{"deploy", "Status", "Unknown"},
				{"test", "Check Run", "Skipped"},
			},
			wantColors: [][]tablewriter.Colors{
				{{}, {}, {tablewriter.FgGreenColor}},
				{{}, {}, {tablewriter.FgWhiteColor}},
				{{}, {}, {tablewriter.FgHiBlackColor}},
			},
		},
		{
			name: "handles empty check names and states",
			checks: []github.CICheckStatus{
				github.CheckRun{
					Name:       "",
					Status:     "",
					Conclusion: "",
				},
				github.StatusContext{
					Context: "",
					State:   "",
				},
			},
			isTTY:       true,
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"", "Check Run", "Pending"},
				{"", "Status", "Unknown"},
			},
			wantColors: [][]tablewriter.Colors{
				{{}, {}, {tablewriter.FgYellowColor}},
				{{}, {}, {tablewriter.FgWhiteColor}},
			},
		},
		{
			name:        "handles no checks",
			checks:      []github.CICheckStatus{},
			isTTY:       true,
			wantHeaders: []string{"Status"},
			wantRows: [][]string{
				{"No CI checks found"},
			},
			wantColors: [][]tablewriter.Colors{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := &mockTableWriter{}
			cfg := &checkListConfig{
				ciConfig: ciConfig{
					owner: "owner",
					repo:  "repo",
					ref:   "ref",
				},
				githubClient: &FakeListCIStatusChecker{
					checks: tt.checks,
				},
			}

			err := listChecks(context.TODO(), cfg, table, tt.isTTY)
			require.NoError(t, err)

			require.Equal(t, tt.wantHeaders, table.headers)
			require.Equal(t, len(tt.wantRows), len(table.rows))

			for i, wantRow := range tt.wantRows {
				require.Equal(t, wantRow, table.rows[i].row)
				if tt.isTTY && len(tt.wantColors) > i && tt.wantColors[i] != nil {
					require.Equal(t, tt.wantColors[i], table.rows[i].colors)
				} else {
					require.Nil(t, table.rows[i].colors)
				}
			}
		})
	}
}
