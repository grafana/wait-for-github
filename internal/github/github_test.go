// wait-for-github
// Copyright (C) 2023-2024, Grafana Labs

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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v79/github"
	"github.com/gregjones/httpcache"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/shurcooL/graphql"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

var testLogger = slog.New(slog.NewTextHandler(
	io.Discard,
	&slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

// TestNewGithubClientWithToken tests that NewGithubClient returns a client
// whose transport is correctly configured to use the provided token and uses a
// retrying transport which itself uses a caching transport.
func TestNewGithubClientWithToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Set up test data
	authInfo := AuthInfo{
		GithubToken: "my-token",
	}
	pendingRecheckTime := 1 * time.Second

	githubClient, err := NewGithubClient(ctx, testLogger, authInfo, pendingRecheckTime)
	require.NoError(t, err)

	if githubClient.client == nil {
		t.Fatal("Returned client has nil client field")
	}

	transport, ok := githubClient.client.Client().Transport.(*oauth2.Transport)
	require.Truef(t, ok, "Returned client transport is not an oauth2 transport (is %T)", githubClient.client.Client().Transport)

	token, err := transport.Source.Token()
	require.NoErrorf(t, err, "Returned client transport has no token: %s", err)
	require.Equalf(t, authInfo.GithubToken, token.AccessToken, "Returned client transport has incorrect token (is %s)", token.AccessToken)

	innerTransport, ok := transport.Base.(*retryablehttp.RoundTripper)
	require.Truef(t, ok, "Returned client transport is not a retryable transport (is %T)", transport.Base)

	require.IsTypef(t, &httpcache.Transport{}, innerTransport.Client.HTTPClient.Transport, "Returned client transport is not a caching transport (is %T)", innerTransport.Client.HTTPClient.Transport)
}

// TestNewGithubClientWithAppAuthentication tests that NewGithubClient returns a
// client whose transport is correctly configured to use the provided app
// authentication and uses a retrying transport which itself uses a caching
// transport.
func TestNewGithubClientWithAppAuthentication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Set up test data
	authInfo := AuthInfo{
		InstallationID: 123,
		AppID:          456,
		// generate this with: openssl genrsa 32 2>/dev/null | awk 1 ORS='\\n'
		PrivateKey: []byte("-----BEGIN RSA PRIVATE KEY-----\nMC0CAQACBQD7J5Q9AgMBAAECBB6C8NkCAwD+JwIDAPz7AgMA1xcCAkoZAgMAwE8=\n-----END RSA PRIVATE KEY-----"),
	}
	pendingRecheckTime := 1 * time.Second

	githubClient, err := NewGithubClient(ctx, testLogger, authInfo, pendingRecheckTime)
	require.NoError(t, err)

	if githubClient.client == nil {
		t.Fatal("Returned client has nil client field")
	}

	transport, ok := githubClient.client.Client().Transport.(*ghinstallation.Transport)
	require.Truef(t, ok, "Returned client transport is not a ghinstallation AppsTransport (is %T)", githubClient.client.Client().Transport)

	innerTransport, ok := transport.Client.(*http.Client)
	require.Truef(t, ok, "Returned client transport is not a http.Client (is %T)", transport.Client)

	nestedTransport, ok := innerTransport.Transport.(*retryablehttp.RoundTripper)
	require.Truef(t, ok, "Returned client transport is not a retryable transport (is %T)", nestedTransport)

	require.IsTypef(t, &httpcache.Transport{}, nestedTransport.Client.HTTPClient.Transport, "Returned client transport is not a caching transport (is %T)", nestedTransport.Client.HTTPClient.Transport)
}

// newClientFromMock returns a new REST & GraphQL GHClient whose transports are configured to use
// the provided mockClient.
func newClientFromMock(t *testing.T, mockClient *http.Client, graphQLURL string) *GHClient {
	t.Helper()

	// descend through the layers of transports to the bottom-most one, which is
	// the caching transport. replace its underlying transport with the mock one
	transport := cachingRetryableTransport(testLogger).(*retryablehttp.RoundTripper)
	cachingTransport := transport.Client.HTTPClient.Transport.(*httpcache.Transport)
	cachingTransport.Transport = mockClient.Transport

	// set a really short timeout so that the tests don't take forever
	transport.Client.RetryWaitMax = 1 * time.Millisecond

	httpClient := &http.Client{Transport: transport}

	return &GHClient{
		logger:             testLogger,
		client:             github.NewClient(httpClient),
		graphQLClient:      graphql.NewClient(graphQLURL, httpClient),
		pendingRecheckTime: 0,
	}
}

// TestResponsesAreRetried tests that responses are retried by the retrying
// transport which underlies the github clients we create.
func TestResponsesAreRetried(t *testing.T) {
	t.Parallel()

	retryCount := 0
	mockClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposPullsByOwnerByRepoByPullNumber,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				retryCount++
				mock.WriteError(
					w,
					http.StatusInternalServerError,
					"Internal Server Error",
				)
			}),
		),
	)

	ghclient := newClientFromMock(t, mockClient, "")
	_, _, err := ghclient.client.PullRequests.Get(context.Background(), "owner", "repo", 1)

	require.Error(t, err)
	require.Equal(t, 5, retryCount)
}

// Same as TestResponsesAreRetried but for GraphQL calls
func TestGraphQLRetry(t *testing.T) {
	retryCount := 0

	mockServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			retryCount++
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
	defer mockServer.Close()

	mockClient := &http.Client{Transport: http.DefaultTransport}
	ghClient := newClientFromMock(t, mockClient, mockServer.URL)

	_, err := ghClient.GetCIStatus(context.Background(), "owner", "repo", "ref", make([]string, 0))
	require.Error(t, err)
	require.Equal(t, 5, retryCount)
}

// TestResponsesAreCached tests that responses are cached by the caching
// transport which underlies the github clients we create.
func TestResponsesAreCached(t *testing.T) {
	t.Parallel()

	hits := 0
	lastStatusSent := 0

	epoch := time.Unix(0, 0)

	pr := &github.PullRequest{
		MergeCommitSHA: github.String("abc123"),
		MergedAt:       &github.Timestamp{Time: epoch},
		Merged:         github.Bool(true),
		State:          github.String("closed"),
	}
	now := time.Now().Format(http.TimeFormat)

	mockClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.GetReposPullsByOwnerByRepoByPullNumber,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++

				// check if we got an If-Modified-Since header with the value of now
				if ims := r.Header.Get("If-Modified-Since"); ims != "" {
					require.Equal(t, now, ims)
					lastStatusSent = http.StatusNotModified
					w.WriteHeader(lastStatusSent)
					return
				}

				// write cache-control headers to ensure that the response is cached
				w.Header().Set("Cache-Control", "max-age=1, must-revalidate")
				// and Last-Modified to ensure that the response is not considered stale
				w.Header().Set("Last-Modified", now)

				lastStatusSent = http.StatusOK
				w.WriteHeader(lastStatusSent)

				_, err := w.Write(mock.MustMarshal(pr))
				require.NoError(t, err)
			}),
		),
	)

	ghclient := newClientFromMock(t, mockClient, "")

	sha, closed, mergedTs, err := ghclient.IsPRMergedOrClosed(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	require.Equal(t, 1, hits)
	require.Equal(t, http.StatusOK, lastStatusSent)
	// check we get what we expect
	require.Equal(t, "abc123", sha)
	require.True(t, closed)
	require.Equal(t, epoch.Unix(), mergedTs)

	// this one should be cached, so the mockClient should not be hit
	sha2, closed2, mergedTs2, err := ghclient.IsPRMergedOrClosed(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	require.Equal(t, 1, hits)
	// check that we got the same values as before
	require.Equal(t, sha, sha2)
	require.Equal(t, closed, closed2)
	require.Equal(t, mergedTs, mergedTs2)

	// wait for the cache to expire
	time.Sleep(1 * time.Second)

	// the cache has expired, so the mockClient should be hit again, but this
	// time with an If-Modified-Since header which should cause the server to
	// return a 304 Not Modified response
	sha3, closed3, mergedTs3, err := ghclient.IsPRMergedOrClosed(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	require.Equal(t, 2, hits)
	require.Equal(t, http.StatusNotModified, lastStatusSent)
	// check that we got the same values as before
	require.Equal(t, sha, sha3)
	require.Equal(t, closed, closed3)
	require.Equal(t, mergedTs, mergedTs3)
}

func errorReturningHandler(t *testing.T, _ mock.EndpointPattern) mock.MockBackendOption {
	t.Helper()

	return mock.WithRequestMatchHandler(
		mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mock.WriteError(
				w,
				http.StatusInternalServerError,
				"Internal Server Error",
			)
		}),
	)
}

func newErrorReturningClient(t *testing.T) *GHClient {
	t.Helper()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))

	return &GHClient{
		logger:        testLogger,
		client:        github.NewClient(nil),
		graphQLClient: graphql.NewClient(mockServer.URL, http.DefaultClient),
	}
}

func TestIsPRMergedOrClosed_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposPullsByOwnerByRepoByPullNumber,
			github.PullRequest{
				Number:         github.Int(1),
				State:          github.String("closed"),
				Merged:         github.Bool(true),
				MergedAt:       &github.Timestamp{Time: time.Now()},
				MergeCommitSHA: github.String("abcdef12345"),
			},
		),
	)

	ghClient := newClientFromMock(t, mockedHTTPClient, "")
	sha, closed, mergedAt, err := ghClient.IsPRMergedOrClosed(ctx, "owner", "repo", 1)

	require.NoError(t, err)
	require.Equal(t, "abcdef12345", sha)
	require.True(t, closed)
	// As time is not frozen, we only check that mergedAt is not zero
	require.NotZero(t, mergedAt)
}

func TestIsPRMergedOrClosed_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ghClient := newErrorReturningClient(t)
	_, _, _, err := ghClient.IsPRMergedOrClosed(ctx, "owner", "repo", 1)
	require.Error(t, err)
}

func TestGetCIStatus(t *testing.T) {
	tests := []struct {
		name           string
		mockGraphQL    string
		expectedStatus CIStatus
		excludedChecks []string
	}{
		{
			name: "success with only checks",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "SUCCESS",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 0,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"conclusion": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusPassed,
		},
		{
			name: "success with only statuses",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "SUCCESS",
								"contexts": {
									"nodes": [
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusPassed,
		},
		{
			name: "success with checks and statuses",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "SUCCESS",
								"contexts": {
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"conclusion": "SUCCESS"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusPassed,
		},
		{
			name: "failed check",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusFailed,
		},
		{
			name: "failed status",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "SUCCESS"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "FAILURE"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusFailed,
		},
		{
			name: "pending with checks and statuses",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "PENDING",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "PENDING",
											"conclusion": "PENDING"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "PENDING"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusPending,
		},
		{
			name: "no data",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": null
					}
				}
			}
            `,
			expectedStatus: CIStatusPassed,
		},
		{
			name: "no CI",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": null
						}
					}
				}
			}
            `,
			expectedStatus: CIStatusPassed,
		},
		{
			name: "unknown status",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "INVALID_STATE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "test-check",
											"status": "INVALID",
											"conclusion": "invalid"
										},
										{
											"__typename": "StatusContext",
											"context": "test-status",
											"state": "unknown"
										}
									],
									"pageInfo": { "hasNextPage": false, "endCursor": null }
								}
							}
						}
					}
				}
			}
            `,
			excludedChecks: []string{"ignored-check"},
			expectedStatus: CIStatusUnknown,
		},
		{
			name: "excluded failed checks",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "CheckRun",
											"name": "test",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "workflow",
											"state": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			excludedChecks: []string{"build", "test", "workflow"},
			expectedStatus: CIStatusPassed,
		},
		{
			name: "multiple failed checks not excluded",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "CheckRun",
											"name": "test",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "SUCCESS"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			excludedChecks: []string{"build"},
			expectedStatus: CIStatusFailed,
		},
		{
			name: "failed status with excluded checks",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "FAILURE"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			excludedChecks: []string{"build"},
			expectedStatus: CIStatusFailed,
		},
		{
			name: "excluded status checks with failed check runs",
			mockGraphQL: `
            {
				"data": {
					"repository": {
						"object": {
							"statusCheckRollup": {
								"state": "FAILURE",
								"contexts": {
									"checkRunCount": 1,
									"statusContextCount": 1,
									"nodes": [
										{
											"__typename": "CheckRun",
											"name": "build",
											"status": "COMPLETED",
											"conclusion": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "deployment",
											"state": "FAILURE"
										},
										{
											"__typename": "StatusContext",
											"context": "worklow",
											"state": "FAILURE"
										}
									],
									"pageInfo": {
										"hasNextPage": false,
										"endCursor": null
									}
								}
							}
						}
					}
				}
			}
            `,
			excludedChecks: []string{"deployment", "workflow"},
			expectedStatus: CIStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockServer := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.URL.Path != "/graphql" {
						http.NotFound(w, r)
						return
					}

					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tt.mockGraphQL))
				}))
			defer mockServer.Close()

			graphQLClient := graphql.NewClient(mockServer.URL+"/graphql", http.DefaultClient)

			ghClient := GHClient{
				client:             nil,
				graphQLClient:      graphQLClient,
				pendingRecheckTime: 1 * time.Millisecond,
				logger:             testLogger,
			}

			require.NotNil(t, ghClient.graphQLClient, "graphQLClient should not be nil")

			status, err := ghClient.GetCIStatus(context.Background(), "owner", "repo", "abcdef12345", tt.excludedChecks)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestGetCIStatus_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ghClient := newErrorReturningClient(t)
	_, err := ghClient.GetCIStatus(ctx, "owner", "repo", "abcdef12345", make([]string, 0))
	require.Error(t, err)
}

func TestGetCIStatusForChecks(t *testing.T) {
	tests := []struct {
		name            string
		checksToLookFor []string
		mockCheckRuns   [][]github.CheckRun
		mockRepoStatus  [][]github.RepoStatus
		expectedStatus  CIStatus
		expectedAwait   []string
	}{
		{
			name:            "Single check - completed with success",
			checksToLookFor: []string{"check1"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Single check - completed with failure",
			checksToLookFor: []string{"check1"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("failure"),
					},
				},
			},
			expectedStatus: CIStatusFailed,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Single check - queued",
			checksToLookFor: []string{"check1"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:   github.String("check1"),
						Status: github.String("queued"),
					},
				},
			},
			expectedStatus: CIStatusPending,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Single check - in progress",
			checksToLookFor: []string{"check1"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:   github.String("check1"),
						Status: github.String("in_progress"),
					},
				},
			},
			expectedStatus: CIStatusPending,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Single check - skipped",
			checksToLookFor: []string{"check1"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Conclusion: github.String("skipped"),
						Status:     github.String("completed"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Single check - doesn't exist, status success",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("check1"),
						State:   github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Single check - doesn't exist, status success, multiple statuses returned",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("what"),
						State:   github.String("failed"),
					},
					{
						Context: github.String("check1"),
						State:   github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Single check - doesn't exist, status success, multiple statuses returned, multiple pages",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("what"),
						State:   github.String("failed"),
					},
				},
				{
					{
						Context: github.String("check1"),
						State:   github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Single check - doesn't exist, status failure",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("check1"),
						State:   github.String("failure"),
					},
				},
			},
			expectedStatus: CIStatusFailed,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Single check - doesn't exist, status pending",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("check1"),
						State:   github.String("pending"),
					},
				},
			},
			expectedStatus: CIStatusPending,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Single check - doesn't exist, status error",
			checksToLookFor: []string{"check1"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("check1"),
						State:   github.String("error"),
					},
				},
			},
			expectedStatus: CIStatusFailed,
			expectedAwait:  []string{"check1"},
		},
		{
			name:            "Multiple checks - all successful",
			checksToLookFor: []string{"check1", "check2"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
					{
						Name:       github.String("check2"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Multiple checks - all successful - multiple pages",
			checksToLookFor: []string{"check1", "check2"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
				{
					{
						Name:       github.String("check2"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
			},
		},
		{
			name:            "Multiple checks - one failed",
			checksToLookFor: []string{"check1", "check2"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
					{
						Name:       github.String("check2"),
						Status:     github.String("completed"),
						Conclusion: github.String("failure"),
					},
				},
			},
			expectedStatus: CIStatusFailed,
			expectedAwait:  []string{"check2"},
		},
		{
			name:            "Multiple checks - one failed - multiple pages",
			checksToLookFor: []string{"check1", "check2"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
				{
					{
						Name:       github.String("check2"),
						Status:     github.String("completed"),
						Conclusion: github.String("failure"),
					},
				},
			},
			expectedStatus: CIStatusFailed,
			expectedAwait:  []string{"check2"},
		},
		{
			name:            "Multiple checks - one pending",
			checksToLookFor: []string{"check1", "check2"},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check1"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
					{
						Name:   github.String("check2"),
						Status: github.String("queued"),
					},
				},
			},
			expectedStatus: CIStatusPending,
			expectedAwait:  []string{"check2"},
		},
		{
			name:            "Multiple checks - multiple statuses - passed - multiple pages",
			checksToLookFor: []string{"check2", "check4"},
			mockRepoStatus: [][]github.RepoStatus{
				{
					{
						Context: github.String("check1"),
						State:   github.String("success"),
					},
				},
				{
					{
						Context: github.String("check2"),
						State:   github.String("success"),
					},
				},
			},
			mockCheckRuns: [][]github.CheckRun{
				{
					{
						Name:       github.String("check3"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
				{
					{
						Name:       github.String("check4"),
						Status:     github.String("completed"),
						Conclusion: github.String("success"),
					},
				},
			},
			expectedStatus: CIStatusPassed,
			expectedAwait:  nil,
		},
		{
			name:            "Check not returned at all - unknown - converted to pending",
			checksToLookFor: []string{"check1"},
			expectedStatus:  CIStatusPending,
			expectedAwait:   []string{"check1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// We need to return a zero-length page rather than nil.
			if len(tt.mockCheckRuns) == 0 {
				tt.mockCheckRuns = [][]github.CheckRun{
					{},
				}
			}
			if tt.mockRepoStatus == nil {
				tt.mockRepoStatus = [][]github.RepoStatus{
					{},
				}
			}

			// Convert tt.mockRepoStatus to an []interface{} so that we can pass
			// it to WithRequestMatchPages.
			var interfaceStatuses []interface{}
			for _, statuses := range tt.mockRepoStatus {
				interfaceStatuses = append(interfaceStatuses, statuses)
			}

			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// parse the request URI to get the check name. only
						// return matching ones.
						uri, err := url.ParseRequestURI(r.RequestURI)
						require.NoError(t, err)
						checkName := uri.Query().Get("check_name")

						runs := make([][]*github.CheckRun, len(tt.mockCheckRuns))
						pages := make([][]byte, len(tt.mockCheckRuns))
						for i, page := range tt.mockCheckRuns {
							for _, run := range page {
								if checkName == "" || *run.Name == checkName {
									runs[i] = append(runs[i], &run)
								}
							}
							pages[i] = mock.MustMarshal(
								github.ListCheckRunsResults{
									Total:     github.Int(len(runs[i])),
									CheckRuns: runs[i],
								},
							)
						}

						handler := &mock.PaginatedResponseHandler{
							ResponsePages: pages,
						}

						handler.ServeHTTP(w, r)
					}),
				),
				mock.WithRequestMatchPages(
					mock.GetReposCommitsStatusesByOwnerByRepoByRef,
					interfaceStatuses...,
				),
			)

			ghClient := newClientFromMock(t, mockedHTTPClient, "")
			status, awaiting, err := ghClient.GetCIStatusForChecks(ctx, "owner", "repo", "abcdef12345", tt.checksToLookFor)

			require.NoError(t, err)
			require.Equalf(t, tt.expectedStatus, status, "expected status %s, got %s", tt.expectedStatus, status)
			require.ElementsMatch(t, tt.expectedAwait, awaiting)
		})
	}
}

func TestGetCIStatusForChecks_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ghClient := newErrorReturningClient(t)
	_, _, err := ghClient.GetCIStatusForChecks(ctx, "owner", "repo", "abcdef12345", []string{"check1"})
	require.Error(t, err)
}

// first call returns okay, second call returns error
func TestGetCIStatusForChecks_ErrorListStatuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposCommitsCheckRunsByOwnerByRepoByRef,
			github.ListCheckRunsResults{
				Total:     github.Int(0),
				CheckRuns: []*github.CheckRun{},
			},
		),
		errorReturningHandler(t, mock.GetReposCommitsStatusesByOwnerByRepoByRef),
	)

	ghClient := newClientFromMock(t, mockedHTTPClient, "")

	_, _, err := ghClient.GetCIStatusForChecks(ctx, "owner", "repo", "abcdef12345", []string{"check1"})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to query GitHub")
}

func TestGetPRHeadSHA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposPullsByOwnerByRepoByPullNumber,
			github.PullRequest{
				Head: &github.PullRequestBranch{
					SHA: github.String("abcdef12345"),
				},
			},
		),
	)

	ghClient := newClientFromMock(t, mockedHTTPClient, "")

	sha, err := ghClient.GetPRHeadSHA(ctx, "owner", "repo", 1)
	require.NoError(t, err)
	require.Equal(t, "abcdef12345", sha)
}

func TestGetPRHeadSHA_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ghClient := newErrorReturningClient(t)
	_, err := ghClient.GetPRHeadSHA(ctx, "owner", "repo", 1)
	require.Error(t, err)
}

func TestHandleResponseError(t *testing.T) {
	t.Parallel()

	logger := testLogger
	client := GHClient{logger: logger}

	resetTime := time.Now().Add(time.Hour)

	tests := []struct {
		name       string
		createResp func() *github.Response
		expectErr  error
	}{
		{
			name:       "nil response handled gracefully",
			createResp: func() *github.Response { return nil },
			expectErr:  nil, // No error expected
		},
		{
			name: "successful response returns no error",
			createResp: func() *github.Response {
				return &github.Response{
					Response: &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{}`)),
					},
				}
			},
			expectErr: nil,
		},
		{
			name: "rate limit error includes reset time",
			createResp: func() *github.Response {
				headers := http.Header{}
				headers.Set("X-RateLimit-Limit", "60")
				headers.Set("X-RateLimit-Remaining", "0")
				headers.Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

				resp := &github.Response{
					Response: &http.Response{
						StatusCode: http.StatusForbidden,
						Header:     headers,
						Body:       io.NopCloser(strings.NewReader(`{"message": "API rate limit exceeded"}`)),
					},
					Rate: github.Rate{
						Limit:     60,
						Remaining: 0,
						Reset:     github.Timestamp{Time: resetTime},
					},
				}
				return resp
			},
			expectErr: &GitHubRateLimitError{},
		},
		{
			name: "abuse rate limit error includes retry after",
			createResp: func() *github.Response {
				resp := &github.Response{
					Response: &http.Response{
						StatusCode: http.StatusForbidden,
						Status:     "403 Forbidden",
						Body: io.NopCloser(strings.NewReader(`{
							"message": "You have triggered an abuse detection mechanism.",
							"documentation_url": "https://developer.github.com/v3/#abuse-rate-limits"
						}`)),
						Header: http.Header{
							"Retry-After":      []string{"60"},
							"Content-Type":     []string{"application/json"},
							"X-GitHub-Request": []string{"ABC123"},
						},
					},
				}
				return resp
			},
			expectErr: &GitHubAbuseRateLimitError{},
		},
		{
			name: "accepted error returns proper error type",
			createResp: func() *github.Response {
				return &github.Response{
					Response: &http.Response{
						StatusCode: http.StatusAccepted,
						Body:       io.NopCloser(strings.NewReader(`{}`)),
					},
				}
			},
			expectErr: &GitHubAcceptedError{},
		},
		{
			name: "generic error includes status code",
			createResp: func() *github.Response {
				return &github.Response{
					Response: &http.Response{
						StatusCode: http.StatusNotFound,
						Status:     "404 Not Found",
						Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
					},
				}
			},
			expectErr: &GitHubAPIError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := tt.createResp()
			err := client.handleResponseError(resp, "TestOp", "owner", "repo")

			if tt.expectErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorAs(t, err, &tt.expectErr)
			}
		})
	}
}
