// wait-for-github
// Copyright (C) 2022-2023, Grafana Labs

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
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/oauth2"

	log "github.com/sirupsen/logrus"
)

type CheckPRMerged interface {
	IsPRMergedOrClosed(ctx context.Context, owner, repo string, pr int) (string, bool, int64, error)
}

type CheckCIStatus interface {
	GetCIStatus(ctx context.Context, owner, repo string, commitHash string) (CIStatus, error)
	GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (CIStatus, []string, error)
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

func NewGithubClient(ctx context.Context, authInfo AuthInfo) (GHClient, error) {
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

func cachingRetryableTransport() http.RoundTripper {
	retryableClient := retryablehttp.NewClient()
	httpCache := httpcache.NewMemoryCacheTransport()
	retryableClient.HTTPClient.Transport = httpCache

	return &retryablehttp.RoundTripper{
		Client: retryableClient,
	}
}

// AuthenticateWithToken authenticates with a GitHub token
func AuthenticateWithToken(ctx context.Context, token string) GHClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: cachingRetryableTransport()})
	httpClient := oauth2.NewClient(ctx, src)
	githubClient := github.NewClient(httpClient)

	return GHClient{client: githubClient}
}

// AuthenticateWithApp authenticates with a GitHub App
func AuthenticateWithApp(ctx context.Context, privateKey []byte, appID, installationID int64) (GHClient, error) {
	itr, err := ghinstallation.New(cachingRetryableTransport(), appID, installationID, privateKey)
	if err != nil {
		return GHClient{}, fmt.Errorf("failed to create transport: %w", err)
	}

	githubClient := github.NewClient(&http.Client{Transport: itr})

	return GHClient{client: githubClient}, nil
}

func (c GHClient) IsPRMergedOrClosed(ctx context.Context, owner, repo string, prNumber int) (string, bool, int64, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", false, -1, fmt.Errorf("failed to query GitHub: %w", err)
	}

	var (
		sha      string
		mergedAt int64
	)

	if pr.GetMerged() {
		sha = pr.GetMergeCommitSHA()
		mergedAt = pr.GetMergedAt().Unix()
	}

	return sha, pr.GetState() == "closed", mergedAt, nil
}

type CIStatus uint

const (
	CIStatusPassed CIStatus = iota
	CIStatusFailed
	CIStatusPending
	CIStatusUnknown
)

func (c CIStatus) String() string {
	switch c {
	case CIStatusPassed:
		return "passed"
	case CIStatusFailed:
		return "failed"
	case CIStatusPending:
		return "pending"
	case CIStatusUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func (c GHClient) GetCIStatus(ctx context.Context, owner, repoName string, ref string) (CIStatus, error) {
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

func (c GHClient) getOneStatus(ctx context.Context, owner, repoName, ref, check string) (CIStatus, error) {
	listOptions := github.ListOptions{
		PerPage: 100,
	}

	opt := &github.ListCheckRunsOptions{
		CheckName:   github.String(check),
		ListOptions: listOptions,
		Filter:      github.String("latest"),
	}

	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, owner, repoName, ref, opt)
	if err != nil {
		return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
	}

	statuses := make([]*github.RepoStatus, 0)

	for _, checkRun := range checkRuns.CheckRuns {
		switch checkRun.GetStatus() {
		case "completed":
			switch checkRun.GetConclusion() {
			case "success":
				return CIStatusPassed, nil
			case "skipped":
				return CIStatusPassed, nil
			default:
				return CIStatusFailed, nil
			}
		case "queued":
			return CIStatusPending, nil
		case "in_progress":
			return CIStatusPending, nil
		}
	}

	// didn't find the check run, so list statuses. we can't filter by status
	// name like we can for checks, so retrieve all results the first time
	if len(statuses) == 0 {
		statuses, _, err = c.client.Repositories.ListStatuses(ctx, owner, repoName, ref, &listOptions)
		if err != nil {
			return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
		}
	}

	// get the statuses for the commit
	for _, status := range statuses {
		if status.GetContext() != check {
			continue
		}

		switch status.GetState() {
		case "success":
			return CIStatusPassed, nil
		case "failure":
			return CIStatusFailed, nil
		case "pending":
			return CIStatusPending, nil
		case "error":
			return CIStatusFailed, nil
		}
	}

	return CIStatusUnknown, nil
}

// GetCIStatusForCheck returns the CI status for a specific commit. It looks at
// both 'checks' and 'statuses'.
func (c GHClient) GetCIStatusForChecks(ctx context.Context, owner, repoName string, ref string, checkNames []string) (CIStatus, []string, error) {
	allFinished := true
	awaitedChecks := make(map[string]bool, len(checkNames))
	var status CIStatus

	for _, checkName := range checkNames {
		status, err := c.getOneStatus(ctx, owner, repoName, ref, checkName)
		if err != nil {
			return CIStatusUnknown, nil, fmt.Errorf("failed to get CI status for check %s: %w", checkName, err)
		}

		if status == CIStatusFailed {
			return status, []string{checkName}, nil
		}

		if status != CIStatusPassed {
			awaitedChecks[checkName] = false
			allFinished = false
		}
	}

	if allFinished {
		return status, nil, nil
	}

	stillWaitingFor := []string{}
	for check, finished := range awaitedChecks {
		if !finished {
			stillWaitingFor = append(stillWaitingFor, check)
		}
	}

	return CIStatusPending, stillWaitingFor, nil
}
