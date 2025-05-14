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

	"github.com/fatih/color"
	"github.com/grafana/wait-for-github/internal/github"
	"github.com/stretchr/testify/require"
)

// mockTableWriter records what was written to the table
type mockTableWriter struct {
	headers []string
	rows    [][]string
}

func (m *mockTableWriter) Header(headers ...any) {
	for _, h := range headers {
		switch v := h.(type) {
		case []string:
			m.headers = append(m.headers, v...)
		default:
			panic(fmt.Sprintf("mockTableWriter.Header: unexpected header type %T", v))
		}
	}
}

func (m *mockTableWriter) Bulk(data any) error {
	switch v := data.(type) {
	case []string:
		m.rows = append(m.rows, v)
	case [][]string:
		m.rows = append(m.rows, v...)
	default:
		return fmt.Errorf("mockTableWriter.Bulk: unexpected data type %T", v)
	}

	return nil
}

func (m *mockTableWriter) Render() error {
	// Do nothing in tests
	return nil
}

var _ tableWriter = (*mockTableWriter)(nil)

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

			err := listChecks(t.Context(), cfg, table)

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
		wantHeaders []string
		wantRows    [][]string
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
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"test", "Check Run", "Passed"},
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
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"test", "Check Run", "Failed"},
			},
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
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"CI / build", "Check Run", "Passed"},
				{"deploy", "Status", "Unknown"},
				{"test", "Check Run", "Skipped"},
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
			wantHeaders: []string{"Name", "Type", "Status"},
			wantRows: [][]string{
				{"", "Check Run", "Pending"},
				{"", "Status", "Unknown"},
			},
		},
		{
			name:        "handles no checks",
			checks:      []github.CICheckStatus{},
			wantHeaders: []string{"Status"},
			wantRows: [][]string{
				{"No CI checks found"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

			err := listChecks(t.Context(), cfg, table)
			require.NoError(t, err)

			require.Equal(t, tt.wantHeaders, table.headers)
			require.Equal(t, len(tt.wantRows), len(table.rows))

			for i, wantRow := range tt.wantRows {
				require.Equal(t, wantRow, table.rows[i])
			}
		})
	}
}

// Tests with `color.NoColor` set to `false` to force colour output.
func TestListChecksColour(t *testing.T) {
	oldNoColor := color.NoColor
	color.NoColor = false

	t.Cleanup(func() {
		color.NoColor = oldNoColor
	})

	checks := []github.CICheckStatus{
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
	}

	table := &mockTableWriter{}
	cfg := &checkListConfig{
		ciConfig: ciConfig{
			owner: "owner",
			repo:  "repo",
			ref:   "ref",
		},
		githubClient: &FakeListCIStatusChecker{
			checks: checks,
		},
	}

	err := listChecks(t.Context(), cfg, table)
	require.NoError(t, err)

	require.Equal(t, len(checks), len(table.rows))

	expectedRows := [][]string{
		{"CI / \x1b[1mbuild\x1b[22m", "Check Run", "\x1b[32mPassed\x1b[0m"},
		{"\x1b[1mdeploy\x1b[22m", "Status", "\x1b[37mUnknown\x1b[0m"},
		{"\x1b[1mtest\x1b[22m", "Check Run", "\x1b[90mSkipped\x1b[0m"},
	}

	require.Equal(t, expectedRows, table.rows)
}
