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

	isPRMergedError   error
	getPRHeadSHAError error
	getCIStatusError  error

	CIStatus github.CIStatus
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

func TestPRCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fakeClient       fakeGithubClientPRCheck
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
				owner: "owner",
				repo:  "repo",
				pr:    1,
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
