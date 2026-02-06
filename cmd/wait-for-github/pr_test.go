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
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// fakeGithubClientPRCheck implements the checkMergedAndOverallCI interface
type fakeGithubClientPRCheck struct {
	MergedCommit string
	Closed       bool
	MergedAt     int64

	isPRMergedError            error
	getPRHeadSHAError          error
	getCIStatusError           error
	rerunFailedWorkflowsError  error

	CIStatus           github.CIStatus
	RerunCount         int
	HasRunsInProgress  bool
	RerunCalledCount   int
}

func (fg *fakeGithubClientPRCheck) IsPRMergedOrClosed(ctx context.Context, owner, repo string, pr int) (string, bool, int64, error) {
	return fg.MergedCommit, fg.Closed, fg.MergedAt, fg.isPRMergedError
}

func (fg *fakeGithubClientPRCheck) GetPRHeadSHA(ctx context.Context, owner, repo string, pr int) (string, error) {
	return fg.MergedCommit, fg.getPRHeadSHAError
}

func (fg *fakeGithubClientPRCheck) GetCIStatus(ctx context.Context, owner, repo string, commitHash string, excludes []string) (github.CIStatus, error) {
	return fg.CIStatus, fg.getCIStatusError
}

func (fg *fakeGithubClientPRCheck) RerunFailedWorkflowsForCommit(ctx context.Context, owner, repo, commitHash string) (int, bool, error) {
	fg.RerunCalledCount++
	return fg.RerunCount, fg.HasRunsInProgress, fg.rerunFailedWorkflowsError
}

func TestPRCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fakeClient       fakeGithubClientPRCheck
		actionRetries    int
		expectedExitCode *int
	}{
		{
			name: "PR is merged",
			fakeClient: fakeGithubClientPRCheck{
				MergedCommit: "abc123",
				MergedAt:     1234567890,
			},
			expectedExitCode: &zero,
		},
		{
			name: "PR is closed",
			fakeClient: fakeGithubClientPRCheck{
				Closed: true,
			},
			expectedExitCode: &one,
		},
		{
			name: "PR is open",
			fakeClient: fakeGithubClientPRCheck{
				Closed: false,
			},
			expectedExitCode: &one,
		},
		{
			name: "Error from IsPRMergedOrClosed",
			fakeClient: fakeGithubClientPRCheck{
				isPRMergedError: fmt.Errorf("an error occurred"),
			},
		},
		{
			name: "Not merged, but CI failed",
			fakeClient: fakeGithubClientPRCheck{
				CIStatus: github.CIStatusFailed,
			},
			expectedExitCode: &one,
		},
		{
			name: "CI failed with action-retries, workflows rerun",
			fakeClient: fakeGithubClientPRCheck{
				CIStatus:   github.CIStatusFailed,
				RerunCount: 1,
			},
			actionRetries: 2,
			// No exit code - should continue waiting after rerun
		},
		{
			name: "CI failed with action-retries, no workflows to rerun",
			fakeClient: fakeGithubClientPRCheck{
				CIStatus:   github.CIStatusFailed,
				RerunCount: 0,
			},
			actionRetries:    2,
			expectedExitCode: &one,
		},
		{
			name: "CI failed with action-retries, rerun error continues waiting",
			fakeClient: fakeGithubClientPRCheck{
				CIStatus:                  github.CIStatusFailed,
				rerunFailedWorkflowsError: fmt.Errorf("rerun failed"),
			},
			actionRetries: 2,
			// No exit code - should continue waiting and retry later
		},
		{
			name: "Not merged, getting PR head SHA failed",
			fakeClient: fakeGithubClientPRCheck{
				getPRHeadSHAError: fmt.Errorf("an error occurred"),
			},
		},
		{
			name: "Not merged, getting CI status failed",
			fakeClient: fakeGithubClientPRCheck{
				getCIStatusError: fmt.Errorf("an error occurred"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakePRStatusChecker := &tt.fakeClient
			cfg := &config{
				recheckInterval: 1,
				logger:          testLogger,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1)
			cancel()

			prConfig := prConfig{
				owner:         "owner",
				repo:          "repo",
				pr:            1,
				actionRetries: tt.actionRetries,
			}

			err := checkPRMerged(ctx, fakePRStatusChecker, cfg, &prConfig)
			if tt.expectedExitCode != nil {
				var exitErr cli.ExitCoder
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, *tt.expectedExitCode, exitErr.ExitCode())
			} else if err != nil {
				require.NotNil(t, err)
			}
		})
	}
}

// TestPRCheckRetryWithInProgressWorkflow tests the race condition where GetCIStatus
// reports failure (a job has failed) but the workflow run hasn't concluded yet because
// other jobs are still running. RerunFailedWorkflowsForCommit returns 0 on the first
// call (no concluded runs to retry), and on the next poll the workflow has completed
// and can be retried.
func TestPRCheckRetryWithInProgressWorkflow(t *testing.T) {
	t.Parallel()

	fakeClient := &fakeGithubClientPRCheck{
		CIStatus:          github.CIStatusFailed,
		RerunCount:        0,    // Workflow run hasn't concluded yet, no runs match "failure" conclusion
		HasRunsInProgress: true, // Other jobs in the workflow are still running
	}

	pr := &prCheck{
		prConfig: prConfig{
			owner:         "owner",
			repo:          "repo",
			pr:            1,
			actionRetries: 2,
		},
		githubClient: fakeClient,
		logger:       testLogger,
	}

	// First check: CI reports failure (from a failed job), but the workflow run
	// is still in progress so RerunFailedWorkflowsForCommit finds nothing to retry.
	// Should continue waiting because runs are still in progress.
	err := pr.Check(context.Background())
	require.NoError(t, err, "should continue waiting when workflow runs are still in progress")
	require.Equal(t, 1, fakeClient.RerunCalledCount)
	require.Equal(t, 0, pr.retriesDone, "retriesDone should not increment when no workflows were rerun")

	// Simulate the workflow run completing — it now has conclusion "failure" and is retryable.
	fakeClient.RerunCount = 1
	fakeClient.HasRunsInProgress = false

	// Second check: CI still failed, workflow run now concluded and gets retried.
	err = pr.Check(context.Background())
	require.NoError(t, err, "should continue waiting after successful rerun")
	require.Equal(t, 2, fakeClient.RerunCalledCount)
	require.Equal(t, 1, pr.retriesDone, "retriesDone should increment after successful rerun")
}

type fakeFileWriter struct {
	filename string
	data     []byte
	perm     fs.FileMode

	err error
}

func (f *fakeFileWriter) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	f.filename = filename
	f.data = data
	f.perm = perm

	return f.err
}

func TestWriteCommitInfoFile(t *testing.T) {
	prConfig := prConfig{
		owner: "owner",
		repo:  "repo",
		pr:    1,

		commitInfoFile: "commit_info_file",
		writer:         &fakeFileWriter{},
	}

	prCheck := &prCheck{
		prConfig: prConfig,

		githubClient: &fakeGithubClientPRCheck{
			MergedCommit: "abc123",
			MergedAt:     1234567890,
		},
		logger: testLogger,
	}

	err := prCheck.Check(context.TODO())
	var cliExitErr cli.ExitCoder
	require.ErrorAs(t, err, &cliExitErr)
	require.Equal(t, 0, cliExitErr.ExitCode())

	ffw := prConfig.writer.(*fakeFileWriter)

	var gotCommitInfo commitInfo
	err = json.Unmarshal(ffw.data, &gotCommitInfo)
	require.NoError(t, err)

	require.Equal(t, "commit_info_file", ffw.filename)
	require.Equal(t, commitInfo{
		Owner:    "owner",
		Repo:     "repo",
		Commit:   "abc123",
		MergedAt: 1234567890,
	}, gotCommitInfo)
	require.Equal(t, fs.FileMode(0644), ffw.perm)
}

var erroringFileWriter *fakeFileWriter = &fakeFileWriter{
	err: fmt.Errorf("error"),
}

func TestWriteCommitInfoFileError(t *testing.T) {
	prConfig := prConfig{
		owner: "owner",
		repo:  "repo",
		pr:    1,

		commitInfoFile: "commit_info_file",
		writer:         erroringFileWriter,
	}

	prCheck := &prCheck{
		prConfig: prConfig,

		githubClient: &fakeGithubClientPRCheck{
			MergedCommit: "abc123",
			MergedAt:     1234567890,
		},
		logger: testLogger,
	}

	err := prCheck.Check(context.Background())
	require.Error(t, err)
}

func TestParsePRArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    prConfig
		wantErr error
	}{
		{
			name: "Valid pull request URL",
			args: []string{"https://github.com/owner/repo/pull/1"},
			want: prConfig{
				owner:  "owner",
				repo:   "repo",
				pr:     1,
				writer: osFileWriter{},
			},
		},
		{
			name: "Valid arguments owner, repo, pr",
			args: []string{"owner", "repo", "1"},
			want: prConfig{
				owner:  "owner",
				repo:   "repo",
				pr:     1,
				writer: osFileWriter{},
			},
		},
		{
			name:    "Invalid pull request URL",
			args:    []string{"https://invalid_url"},
			wantErr: ErrInvalidPRURL{},
		},
		{
			name:    "Invalid number of arguments",
			args:    []string{"owner", "repo"},
			wantErr: cli.Exit("invalid number of arguments", 1),
		},
		{
			name:    "Invalid PR number",
			args:    []string{"owner", "repo", "invalid"},
			wantErr: cli.Exit("invalid PR number", 1),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			rootCmd := &cli.Command{}
			prCmd := &cli.Command{
				Name: "pr",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					got, err := parsePRArguments(ctx, cmd, testLogger)
					if tt.wantErr != nil {
						require.ErrorAs(t, err, &tt.wantErr)
						require.Equal(t, err, tt.wantErr)
					}
					require.Equal(t, tt.want, got)
					return nil
				},
			}
			rootCmd.Commands = []*cli.Command{
				prCmd,
			}
			finalArgs := make([]string, 0, len(tt.args)+1)
			finalArgs = append(finalArgs, "root", "pr")
			finalArgs = append(finalArgs, tt.args...)
			_ = rootCmd.Run(ctx, finalArgs)
		})
	}
}
