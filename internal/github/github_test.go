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
	"testing"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// TestNewGithubClientWithToken tests that NewGithubClient returns a client
// whose transport is correctly configured to use the provided token and uses a
// retrying transport which itself uses a caching transport.
func TestNewGithubClientWithToken(t *testing.T) {
	ctx := context.Background()

	// Set up test data
	authInfo := AuthInfo{
		GithubToken: "my-token",
	}

	client, err := NewGithubClient(ctx, authInfo)
	require.NoError(t, err)

	githubClient, ok := client.(*GHClient)
	require.Truef(t, ok, "Returned client is not a GHClient (is %T)", client)

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
	ctx := context.Background()

	// Set up test data
	authInfo := AuthInfo{
		InstallationID: 123,
		AppID:          456,
		// generate this with: openssl genrsa 32 2>/dev/null | awk 1 ORS='\\n'
		PrivateKey: []byte("-----BEGIN RSA PRIVATE KEY-----\nMC0CAQACBQD7J5Q9AgMBAAECBB6C8NkCAwD+JwIDAPz7AgMA1xcCAkoZAgMAwE8=\n-----END RSA PRIVATE KEY-----"),
	}

	client, err := NewGithubClient(ctx, authInfo)
	require.NoError(t, err)

	githubClient, ok := client.(*GHClient)
	require.Truef(t, ok, "Returned client is not a GHClient (is %T)", client)

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

// newClientFromMock returns a new GHClient whose transport is configured to use
// the provided mockClient.
func newClientFromMock(t *testing.T, mockClient *http.Client) *GHClient {
	t.Helper()

	// descend through the layers of transports to the bottom-most one, which is
	// the caching transport. replace its underlying transport with the mock one
	transport := cachingRetryableTransport().(*retryablehttp.RoundTripper)
	cachingTransport := transport.Client.HTTPClient.Transport.(*httpcache.Transport)
	cachingTransport.Transport = mockClient.Transport

	// set a really short timeout so that the tests don't take forever
	transport.Client.RetryWaitMax = 1 * time.Millisecond

	httpClient := &http.Client{Transport: transport}

	return &GHClient{client: github.NewClient(httpClient)}
}

// TestResponsesAreRetried tests that responses are retried by the retrying
// transport which underlies the github clients we create.
func TestResponsesAreRetried(t *testing.T) {
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

	ghclient := newClientFromMock(t, mockClient)
	_, _, err := ghclient.client.PullRequests.Get(context.Background(), "owner", "repo", 1)

	require.Error(t, err)
	require.Equal(t, 5, retryCount)
}

// TestResponsesAreCached tests that responses are cached by the caching
// transport which underlies the github clients we create.
func TestResponsesAreCached(t *testing.T) {
	hits := 0
	lastStatusSent := 0

	epoch := time.Unix(0, 0)

	pr := &github.PullRequest{
		MergeCommitSHA: github.String("abc123"),
		MergedAt:       &epoch,
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

	ghclient := newClientFromMock(t, mockClient)

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
