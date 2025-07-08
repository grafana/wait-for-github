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
	"testing"
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// FakeCIStatusChecker implements the CheckCIStatus interface.
type FakeCIStatusChecker struct {
	status github.CIStatus
	err    error
}

func (c *FakeCIStatusChecker) GetCIStatus(ctx context.Context, owner, repo string, commitHash string, excludes []string) (github.CIStatus, error) {
	return c.status, c.err
}

func (c *FakeCIStatusChecker) GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (github.CIStatus, []string, error) {
	return c.status, checkNames, c.err
}

func (c *FakeCIStatusChecker) GetDetailedCIStatus(ctx context.Context, owner, repo string, commitHash string) ([]github.CICheckStatus, error) {
	return nil, c.err
}

func TestHandleCIStatus(t *testing.T) {
	tests := []struct {
		name             string
		status           github.CIStatus
		url              string
		expectedExitCode *int
		expectedError    string
	}{
		{
			name:             "passed",
			status:           github.CIStatusPassed,
			expectedExitCode: &zero,
			expectedError:    "CI successful",
		},
		{
			name:             "failed",
			status:           github.CIStatusFailed,
			url:              "https://example.com",
			expectedExitCode: &one,
			expectedError:    "CI failed. Please check CI on the following commit: https://example.com",
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			output := handleCIStatus(testLogger, tt.status, tt.url)

			if tt.expectedExitCode == nil {
				require.Nil(t, output)
			} else {
				require.NotNil(t, output)
				require.Equal(t, *tt.expectedExitCode, output.ExitCode())
				require.Equal(t, tt.expectedError, output.Error())
			}
		})
	}
}

func TestCheckCIStatus(t *testing.T) {
	tests := []struct {
		name             string
		checks           []string
		excludes         []string
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
			name:             "Failed checks excluded",
			checks:           []string{},
			excludes:         []string{"failingCheck"},
			status:           github.CIStatusPassed,
			err:              cli.Exit("CI successful", 0),
			expectedExitCode: &zero,
		},
		{
			name:             "Specific checks fail",
			checks:           []string{"check1", "check2"},
			status:           github.CIStatusFailed,
			err:              cli.Exit("CI failed", 1),
			expectedExitCode: &one,
		},
		{
			name:             "Specific checks fail, exclude ignored",
			checks:           []string{"check1", "check2"},
			excludes:         []string{"check1", "check2"},
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
			t.Parallel()

			fakeCIStatusChecker := &FakeCIStatusChecker{status: tt.status, err: tt.err}
			cfg := &config{
				recheckInterval: 1,
				logger:          testLogger,
			}
			ciConf := &ciConfig{
				excludes: tt.excludes,
				owner:    "owner",
				repo:     "repo",
				ref:      "ref",
				checks:   tt.checks,
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
			ctx := context.Background()
			rootCmd := &cli.Command{Name: "root"}
			ciCmd := &cli.Command{
				Name: "ci",
				Action: func(ctx context.Context, c *cli.Command) error {
					got, err := parseCIArguments(ctx, c, testLogger, "ci")
					if tt.wantErr != nil {
						require.ErrorAs(t, err, &tt.wantErr)
					}

					require.Equal(t, tt.want, got)
					return nil
				},
			}
			rootCmd.Commands = []*cli.Command{ciCmd}
			finalArgs := make([]string, 0, len(tt.args)+1)
			finalArgs = append(finalArgs, "root", "ci")
			finalArgs = append(finalArgs, tt.args...)
			_ = rootCmd.Run(ctx, finalArgs)
		})
	}
}

type UnknownCIStatusChecker struct {
	calls int
}

func (c *UnknownCIStatusChecker) GetCIStatus(ctx context.Context, owner, repo string, commitHash string, excludes []string) (github.CIStatus, error) {
	c.calls++
	if c.calls == 1 {
		return github.CIStatusUnknown, nil
	}

	return github.CIStatusPassed, nil
}

func (c *UnknownCIStatusChecker) GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (github.CIStatus, []string, error) {
	c.calls++
	if c.calls == 1 {
		return github.CIStatusUnknown, checkNames, nil
	}

	return github.CIStatusPassed, checkNames, nil
}

func (c *UnknownCIStatusChecker) GetDetailedCIStatus(ctx context.Context, owner, repo string, commitHash string) ([]github.CICheckStatus, error) {
	return nil, nil
}

func TestUnknownCIStatusRetries(t *testing.T) {
	t.Parallel()

	fakeCIStatusChecker := &UnknownCIStatusChecker{}
	cfg := &config{
		recheckInterval: 1,
		logger:          testLogger,
	}
	ciConf := &ciConfig{
		owner: "owner",
		repo:  "repo",
		ref:   "ref",
	}

	ctx := context.Background()

	err := checkCIStatus(ctx, fakeCIStatusChecker, cfg, ciConf)

	var exitErr cli.ExitCoder
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, zero, exitErr.ExitCode())
	require.Equal(t, 2, fakeCIStatusChecker.calls)

	fakeCIStatusChecker.calls = 0
	ciConf.checks = []string{"check1", "check2"}
	err = checkCIStatus(ctx, fakeCIStatusChecker, cfg, ciConf)

	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, zero, exitErr.ExitCode())
	require.Equal(t, 2, fakeCIStatusChecker.calls)
}

func TestUrlFor(t *testing.T) {
	t.Parallel()

	owner := "owner"
	repo := "repo"
	ref := "abc123"

	url := urlFor(owner, repo, ref)

	require.Equal(t, "https://github.com/owner/repo/commit/abc123", url)
}
