// wait-for-github
// Copyright (C) 2022, Grafana Labs

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
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v48/github"
	"github.com/gregjones/httpcache"
	"golang.org/x/oauth2"

	log "github.com/sirupsen/logrus"
)

type GithubClient interface {
	IsPRMergedOrClosed(ctx context.Context, owner, repo string, pr int) (string, bool, error)
	GetCIStatus(ctx context.Context, owner, repo string, commitHash string) (CIStatus, error)
}

type AuthInfo struct {
	InstallationID int64
	AppID          int64
	PrivateKey     []byte

	GithubToken string
}

type GHClient struct {
	client *github.Client
}

func NewGithubClient(ctx context.Context, authInfo AuthInfo) (GithubClient, error) {
	// If a GitHub token is provided, use it to authenticate in preference to
	// App authentication
	if authInfo.GithubToken != "" {
		log.Debug("Using GitHub token for authentication")
		return AuthenticateWithToken(ctx, authInfo.GithubToken), nil
	}

	// Otherwise, use the App authentication flow
	log.Debug("Using GitHub App for authentication")
	return AuthenticateWithApp(ctx, authInfo.PrivateKey, authInfo.AppID, authInfo.InstallationID)
}

// AuthenticateWithToken authenticates with a GitHub token
func AuthenticateWithToken(ctx context.Context, token string) GithubClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(ctx, src)
	httpClient.Transport = httpcache.NewMemoryCacheTransport()
	githubClient := github.NewClient(httpClient)

	return &GHClient{client: githubClient}
}

// AuthenticateWithApp authenticates with a GitHub App
func AuthenticateWithApp(ctx context.Context, privateKey []byte, appID, installationID int64) (GithubClient, error) {
	itr, err := ghinstallation.New(httpcache.NewMemoryCacheTransport(), appID, installationID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	githubClient := github.NewClient(&http.Client{Transport: itr})

	return &GHClient{client: githubClient}, nil
}

func (c *GHClient) IsPRMergedOrClosed(ctx context.Context, owner, repo string, prNumber int) (string, bool, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", false, fmt.Errorf("failed to query GitHub: %w", err)
	}

	sha := ""

	if pr.GetMerged() {
		sha = pr.GetMergeCommitSHA()
	}

	return sha, pr.GetState() == "closed", nil
}

type CIStatus uint

const (
	CIStatusPassed CIStatus = iota
	CIStatusFailed
	CIStatusPending
	CIStatusUnknown
)

func (c *GHClient) GetCIStatus(ctx context.Context, owner, repoName string, ref string) (CIStatus, error) {
	opt := &github.ListOptions{
		PerPage: 100,
	}

	status, _, err := c.client.Repositories.GetCombinedStatus(ctx, owner, repoName, ref, opt)
	if err != nil {
		return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
	}

	switch status.GetState() {
	case "success":
		return CIStatusPassed, nil
	case "failure":
		return CIStatusFailed, nil
	case "pending":
		return CIStatusPending, nil
	}

	return CIStatusUnknown, nil
}
