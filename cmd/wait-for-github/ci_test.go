// wait-for-github
// Copyright (C) 2023, Grafana Labs

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
	"flag"
	"testing"
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// FakeCIStatusChecker implements the CheckCIStatus interface.
type FakeCIStatusChecker struct {
	status github.CIStatus
	err    error
}

func (c *FakeCIStatusChecker) GetCIStatus(ctx context.Context, owner, repo string, commitHash string) (github.CIStatus, error) {
	return c.status, c.err
}

func (c *FakeCIStatusChecker) GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (github.CIStatus, []string, error) {
	return c.status, checkNames, c.err
}

func TestHandleCIStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		status           github.CIStatus
		expectedExitCode *int
	}{
		{
			name:             "passed",
			status:           github.CIStatusPassed,
			expectedExitCode: &zero,
		},
		{
			name:             "failed",
			status:           github.CIStatusFailed,
			expectedExitCode: &one,
		},
		{
			name:             "pending",
			status:           github.CIStatusPending,
			expectedExitCode: nil,
		},
		{
			name:             "unknown",
			status:           github.CIStatusUnknown,
			expectedExitCode: nil,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			output := handleCIStatus(tt.status, 1, "")
			if tt.expectedExitCode == nil {
				require.Nil(t, output)
			} else {
				require.NotNil(t, output)
				require.Equal(t, *tt.expectedExitCode, output.ExitCode())
			}
		})
	}
}
func TestCheckCIStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		checks           []string
		status           github.CIStatus
		err              error
		recheckInterval  time.Duration
		expectedExitCode *int
	}{
		{
			name:             "All checks pass",
			checks:           []string{},
			status:           github.CIStatusPassed,
			err:              cli.Exit("CI successful", 0),
			recheckInterval:  time.Second * 2,
			expectedExitCode: &zero,
		},
		{
			name:             "Specific checks pass",
			checks:           []string{"check1", "check2"},
			status:           github.CIStatusPassed,
			err:              cli.Exit("CI successful", 0),
			expectedExitCode: &zero,
		},
		{
			name:             "All checks fail",
			checks:           []string{},
			status:           github.CIStatusFailed,
			err:              cli.Exit("CI failed", 1),
			expectedExitCode: &one,
		},
		{
			name:             "Specific checks fail",
			checks:           []string{"check1", "check2"},
			status:           github.CIStatusFailed,
			err:              cli.Exit("CI failed", 1),
			expectedExitCode: &one,
		},
		{
			name:             "All checks pending",
			checks:           []string{},
			status:           github.CIStatusPending,
			err:              nil,
			recheckInterval:  1,
			expectedExitCode: &one,
		},
		{
			name:             "Specific checks pending",
			checks:           []string{"check1", "check2"},
			status:           github.CIStatusPending,
			err:              nil,
			recheckInterval:  1,
			expectedExitCode: &one,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeCIStatusChecker := &FakeCIStatusChecker{status: tt.status, err: tt.err}
			cfg := &config{
				recheckInterval: 1,
			}
			ciConf := &ciConfig{
				owner:  "owner",
				repo:   "repo",
				ref:    "ref",
				checks: tt.checks,
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := checkCIStatus(ctx, fakeCIStatusChecker, cfg, ciConf)

			var exitErr cli.ExitCoder
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, *tt.expectedExitCode, exitErr.ExitCode())
		})
	}
}

func TestParseCIArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    ciConfig
		wantErr error
	}{
		{
			name: "Valid commit URL",
			args: []string{"https://github.com/owner/repo/commit/abc123"},
			want: ciConfig{
				owner: "owner",
				repo:  "repo",
				ref:   "abc123",
			},
		},
		{
			name: "Valid PR URL",
			args: []string{"https://github.com/owner/repo/pull/1234"},
			want: ciConfig{
				owner: "owner",
				repo:  "repo",
				ref:   "refs/pull/1234/head",
			},
		},
		{
			name: "Valid arguments owner, repo, ref",
			args: []string{"owner", "repo", "abc123"},
			want: ciConfig{
				owner: "owner",
				repo:  "repo",
				ref:   "abc123",
			},
		},
		{
			name:    "Invalid commit URL",
			args:    []string{"https://invalid_url"},
			wantErr: ErrInvalidURL{},
		},
		{
			name:    "Invalid number of arguments",
			args:    []string{"owner", "repo"},
			wantErr: cli.Exit("invalid number of arguments", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
			err := flagSet.Parse(tt.args)
			require.NoError(t, err)
			parentCliContext := cli.NewContext(nil, nil, nil)
			parentCliContext.App = cli.NewApp()
			cliContext := cli.NewContext(nil, flagSet, parentCliContext)

			got, err := parseCIArguments(cliContext)
			if tt.wantErr != nil {
				require.ErrorAs(t, err, &tt.wantErr)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestUrlFor(t *testing.T) {
	t.Parallel()

	owner := "owner"
	repo := "repo"
	ref := "abc123"

	url := urlFor(owner, repo, ref)

	require.Equal(t, "https://github.com/owner/repo/commit/abc123", url)
}
