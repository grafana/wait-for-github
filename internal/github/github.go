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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/fatih/color"
	"github.com/google/go-github/v79/github"
	"github.com/grafana/wait-for-github/internal/logging"
	"github.com/gregjones/httpcache"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

var (
	bold      = color.New(color.Bold).SprintFunc()
	fgGreen   = color.New(color.FgGreen).SprintFunc()
	fgHiBlack = color.New(color.FgHiBlack).SprintFunc()
	fgRed     = color.New(color.FgRed).SprintFunc()
	fgWhite   = color.New(color.FgWhite).SprintFunc()
	fgYellow  = color.New(color.FgYellow).SprintFunc()
)

type CheckPRMerged interface {
	IsPRMergedOrClosed(ctx context.Context, owner, repo string, pr int) (string, bool, int64, error)
}

type GetPRHeadSHA interface {
	GetPRHeadSHA(ctx context.Context, owner, repo string, pr int) (string, error)
}

type CheckOverallCIStatus interface {
	GetCIStatus(ctx context.Context, owner, repo string, commitHash string, excludes []string) (CIStatus, error)
}

type CheckCIStatusForChecks interface {
	GetCIStatusForChecks(ctx context.Context, owner, repo string, commitHash string, checkNames []string) (CIStatus, []string, error)
}

type GetDetailedCIStatus interface {
	GetDetailedCIStatus(ctx context.Context, owner, repo string, commitHash string) ([]CICheckStatus, error)
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
	logger             *slog.Logger
	client             *github.Client
	graphQLClient      *graphql.Client
	pendingRecheckTime time.Duration
}

type CIStatus uint

const (
	CIStatusPassed CIStatus = iota
	CIStatusFailed
	CIStatusPending
	CIStatusUnknown
	CIStatusSkipped
)

func (c CIStatus) String() string {
	switch c {
	case CIStatusPassed:
		return fgGreen("passed")
	case CIStatusFailed:
		return fgRed("failed")
	case CIStatusPending:
		return fgYellow("pending")
	case CIStatusSkipped:
		return fgHiBlack("skipped")
	case CIStatusUnknown:
		fallthrough
	default:
		return fgWhite("unknown")
	}
}

type CICheckStatus interface {
	fmt.Stringer
	Outcome() CIStatus
	Type() string
}

type PageInfo struct {
	EndCursor   *string
	HasNextPage bool
}

type WorkflowInfo struct {
	Name string
}

type AppInfo struct {
	Name string
}

type CheckSuiteInfo struct {
	App         AppInfo
	WorkflowRun struct {
		Workflow WorkflowInfo
	}
}

type CheckRun struct {
	Name       string
	Status     string
	Conclusion string
	CheckSuite CheckSuiteInfo
}

func (c CheckRun) String() string {
	boldName := bold(c.Name)
	if c.CheckSuite.WorkflowRun.Workflow.Name == "" {
		return boldName
	}

	return fmt.Sprintf("%s / %s", c.CheckSuite.WorkflowRun.Workflow.Name, boldName)
}

func (c CheckRun) Outcome() CIStatus {
	switch strings.ToLower(c.Status) {
	case "completed":
		switch strings.ToLower(c.Conclusion) {
		case "success":
			return CIStatusPassed
		case "startup_failure":
			return CIStatusFailed
		case "failure":
			return CIStatusFailed
		case "skipped":
			return CIStatusSkipped
		default:
			return CIStatusUnknown
		}
	default:
		return CIStatusPending
	}
}

func (c CheckRun) Type() string {
	if c.CheckSuite.App.Name == "GitHub Actions" {
		return "Action"
	}

	return "Check Run"
}

type StatusContext struct {
	Context string
	State   string
}

func (s StatusContext) String() string {
	return bold(s.Context)
}

func (s StatusContext) Outcome() CIStatus {
	switch strings.ToLower(s.State) {
	case "success":
		return CIStatusPassed
	case "failure":
		return CIStatusFailed
	case "error":
		return CIStatusFailed
	default:
		return CIStatusUnknown
	}
}

func (s StatusContext) Type() string {
	return "Status"
}

type RollupContextNode struct {
	Typename      string        `graphql:"__typename"`
	CheckRun      CheckRun      `graphql:"... on CheckRun"`
	StatusContext StatusContext `graphql:"... on StatusContext"`
}

type RollupContexts struct {
	CheckRunCount      int
	StatusContextCount int
	Nodes              []RollupContextNode
	PageInfo           PageInfo
}

type StatusCheckRollup struct {
	State    string
	Contexts RollupContexts `graphql:"contexts(first: 100, after: $cursor)"`
}

// handleResponseError processes GitHub API response errors
func (c GHClient) handleResponseError(resp *github.Response, operation, owner, repo string) error {
	if resp == nil || resp.Response == nil {
		return nil
	}

	apiErr := github.CheckResponse(resp.Response)
	if apiErr != nil {
		switch e := apiErr.(type) {
		case *github.RateLimitError:
			resetTime := e.Rate.Reset.Time
			return &GitHubRateLimitError{
				Operation: operation,
				Owner:     owner,
				Repo:      repo,
				ResetTime: resetTime,
				Remaining: e.Rate.Remaining,
				Err:       apiErr,
			}

		case *github.AbuseRateLimitError:
			var retryAfter time.Duration
			if e.RetryAfter != nil {
				retryAfter = *e.RetryAfter
			}
			return &GitHubAbuseRateLimitError{
				Operation:  operation,
				Owner:      owner,
				Repo:       repo,
				RetryAfter: retryAfter,
				Err:        apiErr,
			}

		case *github.AcceptedError:
			return &GitHubAcceptedError{
				Operation: operation,
				Owner:     owner,
				Repo:      repo,
				Err:       apiErr,
			}

		default:
			return &GitHubAPIError{
				Operation: operation,
				Owner:     owner,
				Repo:      repo,
				Status:    resp.Response.Status,
				Err:       apiErr,
			}
		}
	}

	return nil
}

func cachingRetryableTransport(logger *slog.Logger) http.RoundTripper {
	retryableClient := retryablehttp.NewClient()
	retryableClient.Logger = logger

	httpCache := httpcache.NewMemoryCacheTransport()
	retryableClient.HTTPClient.Transport = httpCache

	return &retryablehttp.RoundTripper{
		Client: retryableClient,
	}
}

func NewGithubClient(ctx context.Context, logger *slog.Logger, authInfo AuthInfo, pendingRecheckTime time.Duration) (GHClient, error) {
	// If a GitHub token is provided, use it to authenticate in preference to App authentication.
	if authInfo.GithubToken != "" {
		logger.InfoContext(ctx, "using github token for authentication")
		return AuthenticateWithToken(ctx, logger, authInfo.GithubToken, pendingRecheckTime), nil
	}

	logger.InfoContext(ctx, "using github app for authentication")
	return AuthenticateWithApp(ctx, logger, authInfo.PrivateKey, authInfo.AppID, authInfo.InstallationID, pendingRecheckTime)
}

// AuthenticateWithToken authenticates with a GitHub token.
func AuthenticateWithToken(ctx context.Context, logger *slog.Logger, token string, pendingRecheckTime time.Duration) GHClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: cachingRetryableTransport(logger)})
	httpClient := oauth2.NewClient(ctx, src)

	restClient := github.NewClient(httpClient)
	graphQLClient := graphql.NewClient("https://api.github.com/graphql", httpClient)

	return GHClient{
		logger:             logger,
		client:             restClient,
		graphQLClient:      graphQLClient,
		pendingRecheckTime: pendingRecheckTime,
	}
}

// AuthenticateWithApp authenticates with a GitHub App
func AuthenticateWithApp(ctx context.Context, logger *slog.Logger, privateKey []byte, appID, installationID int64, pendingRecheckTime time.Duration) (GHClient, error) {
	itr, err := ghinstallation.New(cachingRetryableTransport(logger), appID, installationID, privateKey)
	if err != nil {
		return GHClient{}, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: itr}

	restClient := github.NewClient(httpClient)
	graphQLClient := graphql.NewClient("https://api.github.com/graphql", httpClient)

	return GHClient{
		logger:             logger,
		client:             restClient,
		graphQLClient:      graphQLClient,
		pendingRecheckTime: pendingRecheckTime,
	}, nil
}

func (c GHClient) IsPRMergedOrClosed(ctx context.Context, owner, repo string, prNumber int) (string, bool, int64, error) {
	pr, resp, err := c.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", false, -1, fmt.Errorf("failed to query GitHub: %w", err)
	}

	respErr := c.handleResponseError(resp, "GetPullRequest", owner, repo)
	if respErr != nil {
		return "", false, -1, respErr
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
	pr, resp, err := c.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", fmt.Errorf("failed to query GitHub for PR HEAD SHA: %w", err)
	}

	respErr := c.handleResponseError(resp, "GetPullRequest", owner, repo)
	if respErr != nil {
		return "", respErr
	}

	return pr.GetHead().GetSHA(), nil
}

func (c GHClient) getStatusCheckRollup(ctx context.Context, owner, repoName, ref string) (*StatusCheckRollup, []RollupContextNode, error) {
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

	var allNodes []RollupContextNode
	retried := false

	var rollup *StatusCheckRollup

	for {
		if err := c.graphQLClient.Query(ctx, &query, vars); err != nil {
			return nil, nil, fmt.Errorf("failed to query GitHub: %w", err)
		}

		if rollup = query.Repository.Object.Commit.StatusCheckRollup; rollup == nil {
			return nil, allNodes, nil
		}

		contexts := rollup.Contexts
		pageInfo := contexts.PageInfo

		allNodes = append(allNodes, contexts.Nodes...)

		if !pageInfo.HasNextPage {
			if (!hasChecksOrStatuses(rollup)) && !retried {
				c.logger.InfoContext(ctx, "did not find any checks and/or statuses. retrying", "retry_in", c.pendingRecheckTime)
				time.Sleep(c.pendingRecheckTime)
				retried = true
				vars["cursor"] = (*graphql.String)(nil)
				allNodes = nil
				continue
			}

			break
		}

		vars["cursor"] = graphql.String(*pageInfo.EndCursor)
	}

	return rollup, allNodes, nil
}

func hasChecksOrStatuses(rollup *StatusCheckRollup) bool {
	return rollup.Contexts.CheckRunCount > 0 || rollup.Contexts.StatusContextCount > 0
}

func (c GHClient) GetCIStatus(ctx context.Context, owner, repoName, ref string, excludes []string) (CIStatus, error) {
	logger := c.logger.With(logging.OwnerAttr(owner), logging.RepoAttr(repoName), logging.RefAttr(ref))
	rollup, nodes, err := c.getStatusCheckRollup(ctx, owner, repoName, ref)
	if err != nil {
		return CIStatusUnknown, err
	}

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

	if !hasChecksOrStatuses(rollup) {
		logger.DebugContext(ctx, "no checks or statuses found after retry, assuming success")
		return CIStatusPassed, nil
	}

	if isPending {
		return CIStatusPending, nil
	}

	if isFailure {
		reportFailure := false
		if len(excludes) == 0 {
			reportFailure = true
			c.logger.DebugContext(ctx, "failed CI checks")
		}

		for _, node := range nodes {
			isCheckFailure := strings.ToLower(node.CheckRun.Conclusion) == "failure"
			isStatusFailure := strings.ToLower(node.StatusContext.State) == "failure"

			checkRunName := node.CheckRun.Name
			statusContextName := node.StatusContext.Context

			switch node.Typename {
			case "CheckRun":
				if isCheckFailure {
					checkLogger := logger.With(logging.CheckRunAttr(checkRunName))
					if !slices.Contains(excludes, checkRunName) {
						reportFailure = true
						checkLogger.DebugContext(ctx, "checkrun failed")
					} else {
						checkLogger.DebugContext(ctx, "checkrun failed but excluded")
					}
				}
			case "StatusContext":
				if isStatusFailure {
					statusLogger := logger.With(logging.StatusContextAttr(statusContextName))
					if !slices.Contains(excludes, statusContextName) {
						reportFailure = true
						statusLogger.DebugContext(ctx, "status context failed")
					} else {
						statusLogger.DebugContext(ctx, "status context failed but excluded")
					}
				}
			}
		}
		if reportFailure {
			return CIStatusFailed, nil
		}
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

		respErr := c.handleResponseError(resp, "ListCheckRunsForRef", owner, repoName)
		if respErr != nil {
			var acceptedErr *GitHubAcceptedError
			if errors.As(respErr, &acceptedErr) {
				return CIStatusPending, nil
			}
			return CIStatusUnknown, respErr
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

		respErr := c.handleResponseError(resp, "ListStatuses", owner, repoName)
		if respErr != nil {
			var acceptedErr *GitHubAcceptedError
			if errors.As(respErr, &acceptedErr) {
				return CIStatusPending, nil
			}
			return CIStatusUnknown, respErr
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

func (c GHClient) GetDetailedCIStatus(ctx context.Context, owner, repoName, ref string) ([]CICheckStatus, error) {
	_, nodes, err := c.getStatusCheckRollup(ctx, owner, repoName, ref)
	if err != nil {
		return nil, err
	}

	var allChecks []CICheckStatus
	for _, node := range nodes {
		switch node.Typename {
		case "CheckRun":
			allChecks = append(allChecks, node.CheckRun)
		case "StatusContext":
			if node.StatusContext.Context != "" && node.StatusContext.State != "" {
				allChecks = append(allChecks, node.StatusContext)
			}
		}
	}

	slices.SortFunc(allChecks, func(a, b CICheckStatus) int {
		return strings.Compare(a.String(), b.String())
	})

	return allChecks, nil
}
