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
	"time"

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

type GetPRHeadSHA interface {
	GetPRHeadSHA(ctx context.Context, owner, repo string, pr int) (string, error)
}

type CheckOverallCIStatus interface {
	GetCIStatus(ctx context.Context, owner, repo string, commitHash string) (CIStatus, error)
}

type CheckCIStatusForChecks interface {
	GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (CIStatus, []string, error)
}

type CheckCIStatus interface {
	CheckOverallCIStatus
	CheckCIStatusForChecks
}

type AuthInfo struct {
	InstallationID int64
	AppID          int64
	PrivateKey     []byte

	GithubToken string
}

type GHClient struct {
	client  *github.Client
	pendingRecheckTime time.Duration
}

func NewGithubClient(ctx context.Context, authInfo AuthInfo, pendingRecheckTime time.Duration) (GHClient, error) {
	// If a GitHub token is provided, use it to authenticate in preference to
	// App authentication
	if authInfo.GithubToken != "" {
		log.Debug("Using GitHub token for authentication")
		return AuthenticateWithToken(ctx, authInfo.GithubToken, pendingRecheckTime), nil
	}

	// Otherwise, use the App authentication flow
	log.Debug("Using GitHub App for authentication")
	return AuthenticateWithApp(ctx, authInfo.PrivateKey, authInfo.AppID, authInfo.InstallationID, pendingRecheckTime)
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
func AuthenticateWithToken(ctx context.Context, token string, pendingRecheckTime time.Duration) GHClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: cachingRetryableTransport()})
	httpClient := oauth2.NewClient(ctx, src)
	githubClient := github.NewClient(httpClient)

	return GHClient{client: githubClient, pendingRecheckTime: pendingRecheckTime}
}

// AuthenticateWithApp authenticates with a GitHub App
func AuthenticateWithApp(ctx context.Context, privateKey []byte, appID, installationID int64, pendingRecheckTime time.Duration) (GHClient, error) {
	itr, err := ghinstallation.New(cachingRetryableTransport(), appID, installationID, privateKey)
	if err != nil {
		return GHClient{}, fmt.Errorf("failed to create transport: %w", err)
	}

	githubClient := github.NewClient(&http.Client{Transport: itr})

	return GHClient{client: githubClient, pendingRecheckTime: pendingRecheckTime }, nil
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

func (c GHClient) GetPRHeadSHA(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", fmt.Errorf("failed to query GitHub for PR HEAD SHA: %w", err)
	}

	return pr.GetHead().GetSHA(), nil
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
	status, _, err := c.client.Repositories.GetCombinedStatus(ctx, owner, repoName, ref, nil)
	if err != nil {
		return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
	}

	switch status.GetState() {
	case "success":
		return CIStatusPassed, nil
	case "failure":
		return CIStatusFailed, nil
	case "pending":
		// From the GitHub API docs
		// (https://docs.github.com/en/rest/commits/statuses?apiVersion=2022-11-28#get-the-combined-status-for-a-specific-reference):
		//
		// > Additionally, a combined state is returned. The state is one of:
		// > ...
		// > pending *if there are no statuses* or a context is pending
		// > ...
		//
		// (Emphasis ours.) This means that if there are no statuses, the
		// combined status will be "pending". We need to check if there are
		// statuses to determine if the status is actually pending. If there
		// aren't any, we should consider this to be a success. But. A status
		// check could take a while to be created (think a webhook to a CI
		// system that takes a while to start a build). So we can wait a bit
		// and then check again.
		if len(status.Statuses) == 0 {
			log.Infof("No statuses found, waiting %s to see if one appears", c.pendingRecheckTime)
			time.Sleep(c.pendingRecheckTime)

			status, _, err = c.client.Repositories.GetCombinedStatus(ctx, owner, repoName, ref, nil)
			if err != nil {
				return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
			}

			state := status.GetState()

			// Something changed - this will cause us to be called again.
			if state != "pending" {
				return CIStatusUnknown, nil
			}

			// Ok, cool, now we believe it's not going to get a status, so let's say it passed.
			if len(status.Statuses) == 0 {
				log.Debug("No statuses found after waiting, assuming success")
				return CIStatusPassed, nil
			}

			// It's really pending
		}

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

	var checkRuns []*github.CheckRun
	for {
		runs, resp, err := c.client.Checks.ListCheckRunsForRef(ctx, owner, repoName, ref, opt)
		if err != nil {
			return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
		}
		checkRuns = append(checkRuns, runs.CheckRuns...)

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	for _, checkRun := range checkRuns {
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

	statuses := make([]*github.RepoStatus, 0)
	listOptions.Page = 0

	// didn't find the check run, so list statuses. we can't filter by status
	// name like we can for checks, so retrieve all results the first time
	for {
		s, resp, err := c.client.Repositories.ListStatuses(ctx, owner, repoName, ref, &listOptions)
		if err != nil {
			return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
		}

		statuses = append(statuses, s...)

		if resp.NextPage == 0 {
			break
		}

		listOptions.Page = resp.NextPage
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
