package github

import (
	"fmt"
	"time"
)

type GitHubRateLimitError struct {
	Operation string
	Owner     string
	Repo      string
	ResetTime time.Time
	Remaining int
	Err       error
}

func (e *GitHubRateLimitError) Error() string {
	return fmt.Sprintf("GitHub rate limit exceeded for %s operation on %s/%s, resets at %v",
		e.Operation, e.Owner, e.Repo, e.ResetTime)
}

func (e *GitHubRateLimitError) Unwrap() error {
	return e.Err
}

type GitHubAbuseRateLimitError struct {
	Operation  string
	Owner      string
	Repo       string
	RetryAfter time.Duration
	Err        error
}

func (e *GitHubAbuseRateLimitError) Error() string {
	return fmt.Sprintf("GitHub abuse detection triggered for %s operation on %s/%s, retry after %v",
		e.Operation, e.Owner, e.Repo, e.RetryAfter)
}

func (e *GitHubAbuseRateLimitError) Unwrap() error {
	return e.Err
}

type GitHubAcceptedError struct {
	Operation string
	Owner     string
	Repo      string
	Err       error
}

func (e *GitHubAcceptedError) Error() string {
	return fmt.Sprintf("GitHub is processing the %s operation on %s/%s",
		e.Operation, e.Owner, e.Repo)
}

func (e *GitHubAcceptedError) Unwrap() error {
	return e.Err
}

type GitHubAPIError struct {
	Operation string
	Owner     string
	Repo      string
	Status    string
	Err       error
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub API error for %s operation on %s/%s (%s)",
		e.Operation, e.Owner, e.Repo, e.Status)
}

func (e *GitHubAPIError) Unwrap() error {
	return e.Err
}
