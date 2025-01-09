// wait-for-github
// Copyright (C) 2022-2024, Grafana Labs

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
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/shurcooL/graphql"
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
	client             *github.Client
	graphQLClient      *graphql.Client
	pendingRecheckTime time.Duration
}

type PageInfo struct {
	EndCursor   *string
	HasNextPage bool
}

type CheckRun struct {
	Name       string
	Conclusion string
}

type StatusContext struct {
	Context string
	State   string
}

type RollupContextNode struct {
	Typename      string        `graphql:"__typename"`
	CheckRun      CheckRun      `graphql:"... on CheckRun"`
	StatusContext StatusContext `graphql:"... on StatusContext"`
}

type RollupContexts struct {
	Nodes    []RollupContextNode
	PageInfo PageInfo
}

type StatusCheckRollup struct {
	State    string
	Contexts RollupContexts `graphql:"contexts(first: 100, after: $cursor)"`
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

	restClient := github.NewClient(httpClient)
	graphQLClient := graphql.NewClient("https://api.github.com/graphql", httpClient)

	return GHClient{
		client:             restClient,
		graphQLClient:      graphQLClient,
		pendingRecheckTime: pendingRecheckTime,
	}
}

// AuthenticateWithApp authenticates with a GitHub App
func AuthenticateWithApp(ctx context.Context, privateKey []byte, appID, installationID int64, pendingRecheckTime time.Duration) (GHClient, error) {
	itr, err := ghinstallation.New(cachingRetryableTransport(), appID, installationID, privateKey)
	if err != nil {
		return GHClient{}, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: itr}

	restClient := github.NewClient(httpClient)
	graphQLClient := graphql.NewClient("https://api.github.com/graphql", httpClient)

	return GHClient{
		client:             restClient,
		graphQLClient:      graphQLClient,
		pendingRecheckTime: pendingRecheckTime,
	}, nil
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

func (c GHClient) GetCIStatus(ctx context.Context, owner, repoName, ref string) (CIStatus, error) {
	var query struct {
		Repository struct {
			Object struct {
				Commit struct {
					StatusCheckRollup *StatusCheckRollup `graphql:"statusCheckRollup"`
				} `graphql:"... on Commit"`
			} `graphql:"object(expression: $ref)"`
		} `graphql:"repository(owner: $owner, name: $repository)"`
	}

	vars := map[string]interface{}{
		"owner":      graphql.String(owner),
		"repository": graphql.String(repoName),
		"ref":        graphql.String(ref),
		"cursor":     (*graphql.String)(nil),
	}

	hasChecks := false
	hasStatuses := false
	retried := false

	for {
		err := c.graphQLClient.Query(ctx, &query, vars)
		if err != nil {
			return CIStatusUnknown, fmt.Errorf("failed to query GitHub: %w", err)
		}

		rollup := query.Repository.Object.Commit.StatusCheckRollup
		if rollup == nil {
			return CIStatusPassed, nil
		}

		isSuccess := strings.ToLower(rollup.State) == "success"
		isFailure := strings.ToLower(rollup.State) == "failure"
		isPending := strings.ToLower(rollup.State) == "pending"

		// return early if all checks and statuses are successful. no need to evaluate individual nodes in the response.
		if isSuccess {
			return CIStatusPassed, nil
		}

		hasChecks = false
		hasStatuses = false
		failedNodes := []string{}

		for _, node := range rollup.Contexts.Nodes {
			isCheckFailure := strings.ToLower(node.CheckRun.Conclusion) == "failure"
			isStatusFailure := strings.ToLower(node.StatusContext.State) == "failure"
			hasStatusContext := node.StatusContext.Context != "" && node.StatusContext.State != ""

			switch node.Typename {
			case "CheckRun":
				hasChecks = true
				if isCheckFailure {
					failedNodes = append(failedNodes, fmt.Sprintf("CheckRun '%s' failed", node.CheckRun.Name))
				}
			case "StatusContext":
				if hasStatusContext {
					hasStatuses = true
				}
				if isStatusFailure {
					failedNodes = append(failedNodes, fmt.Sprintf("StatusContext '%s' failed", node.StatusContext.Context))
				}
			}
		}

		if rollup.Contexts.PageInfo.HasNextPage {
			vars["cursor"] = graphql.String(*rollup.Contexts.PageInfo.EndCursor)
			continue
		}

		if (!hasChecks || !hasStatuses) && !retried {
			log.Infof("Did not find any checks and/or statuses. Retrying in %s to see if any appear", c.pendingRecheckTime)
			time.Sleep(c.pendingRecheckTime)
			retried = true
			vars["cursor"] = (*graphql.String)(nil)
			continue
		}

		if isPending {
			return CIStatusPending, nil
		}

		if isFailure {
			log.Debug("Failed CI checks:")
			for _, failure := range failedNodes {
				log.Debug(failure)
			}
			return CIStatusFailed, nil
		}

		break
	}

	if !hasChecks && !hasStatuses {
		log.Debug("No checks or statuses found after retry, assuming success")
		return CIStatusPassed, nil
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
