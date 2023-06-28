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
	"flag"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// FakeGithubClientPRCheck implements CheckPRMerged
type FakeGithubClientPRCheck struct {
	MergedCommit string
	Closed       bool
	MergedAt     int64
	Error        error
}

func (fg *FakeGithubClientPRCheck) IsPRMergedOrClosed(ctx context.Context, owner, repo string, pr int) (string, bool, int64, error) {
	return fg.MergedCommit, fg.Closed, fg.MergedAt, fg.Error
}

func TestPRCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fakeClient       FakeGithubClientPRCheck
		err              error
		expectedExitCode *int
	}{
		{
			name: "PR is merged",
			fakeClient: FakeGithubClientPRCheck{
				MergedCommit: "abc123",
				MergedAt:     1234567890,
			},
			expectedExitCode: &zero,
		},
		{
			name: "PR is closed",
			fakeClient: FakeGithubClientPRCheck{
				Closed: true,
			},
			expectedExitCode: &one,
		},
		{
			name: "PR is open",
			fakeClient: FakeGithubClientPRCheck{
				MergedCommit: "",
				Closed:       false,
			},
			expectedExitCode: &one,
		},
		{
			name: "Error from IsPRMergedOrClosed",
			fakeClient: FakeGithubClientPRCheck{
				Error: fmt.Errorf("an error occurred"),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakePRStatusChecker := &tt.fakeClient
			cfg := &config{
				recheckInterval: 1,
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
				require.ErrorAs(t, err, &tt.err)
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

		githubClient: &FakeGithubClientPRCheck{
			MergedCommit: "abc123",
			MergedAt:     1234567890,
		},
	}

	err := prCheck.Check(context.TODO(), 1)
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

		githubClient: &FakeGithubClientPRCheck{
			MergedCommit: "abc123",
			MergedAt:     1234567890,
		},
	}

	err := prCheck.Check(context.Background(), 1)
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
			flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
			err := flagSet.Parse(tt.args)
			require.NoError(t, err)
			parentCliContext := cli.NewContext(nil, nil, nil)
			parentCliContext.App = cli.NewApp()
			cliContext := cli.NewContext(nil, flagSet, parentCliContext)

			got, err := parsePRArguments(cliContext)
			if tt.wantErr != nil {
				require.ErrorAs(t, err, &tt.wantErr)
				require.Equal(t, err, tt.wantErr)
			}
			require.Equal(t, tt.want, got)
		})
	}
}
