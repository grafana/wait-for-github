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

package github

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v48/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
)

// owner -> repo -> prNumber -> pr
type pullRequests map[string]map[string]map[int]github.PullRequest

// mockPRClient returns a client which will respond with information about the
// PRs given
func mockPRClient(prs pullRequests) GithubClient {
	mockClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposPullsByOwnerByRepoByPullNumber,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// e.g. "/repos/grafana/foo/pulls/1"
				path := strings.TrimPrefix(r.URL.Path, "/repos/")
				parts := strings.Split(path, "/")
				owner := parts[0]
				repo := parts[1]
				prNumber, err := strconv.Atoi(parts[3])

				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				pr, ok := prs[owner][repo][prNumber]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(mock.MustMarshal(pr))
			}),
		),
	)

	client := github.NewClient(mockClient)

	return &GHClient{
		client: client,
	}
}

/*
func mockClientHangForever() GithubClient {
	mockClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.
		)
	client := github.NewClient(mockClient)

	return &GHClient{
		client: client,
	}
}
*/

func TestPRMerged(t *testing.T) {
	ctx := context.Background()

	type testCase struct {
		name                   string
		tc                     pullRequests
		expectedError          bool
		expectedMergeCommit    string
		expectedMergedOrClosed bool
	}

	testCases := []testCase{
		{
			name: "PR merged",
			tc: pullRequests{
				"grafana": {
					"foo": {
						1: {
							MergeCommitSHA: github.String("abc123"),
							Merged:         github.Bool(true),
							State:          github.String("closed"),
						},
					},
				},
			},
			expectedError:          false,
			expectedMergeCommit:    "abc123",
			expectedMergedOrClosed: true,
		},
		{
			name: "PR closed",
			tc: pullRequests{
				"grafana": {
					"foo": {
						1: {
							Merged: github.Bool(false),
							State:  github.String("closed"),
						},
					},
				},
			},
			expectedError:          false,
			expectedMergedOrClosed: true,
		},
		{
			name: "PR open",
			tc: pullRequests{
				"grafana": {
					"foo": {
						1: {
							Merged: github.Bool(false),
							State:  github.String("open"),
						},
					},
				},
			},
			expectedError:          false,
			expectedMergedOrClosed: false,
		},
		{
			name: "PR not found",
			tc: pullRequests{
				"grafana": {
					"foo": {},
				},
			},
			expectedError:          true,
			expectedMergedOrClosed: false,
		},
		{
			name:                   "error",
			tc:                     pullRequests{},
			expectedError:          true,
			expectedMergedOrClosed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := mockPRClient(tc.tc)

			sha, prMerged, err := mockClient.IsPRMergedOrClosed(ctx, "grafana", "foo", 1)
			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.expectedMergeCommit != "" {
					require.Equal(t, sha, tc.expectedMergeCommit)
				}
				require.Equal(t, prMerged, tc.expectedMergedOrClosed)
			}
		})
	}
}
