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

package utils

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grafana/wait-for-github/internal/github"
	"github.com/urfave/cli/v3"
)

type Check interface {
	Check(ctx context.Context) error
}

// TryRerunFailedWorkflows attempts to rerun failed workflows if retries are available.
// Returns whether to continue waiting, and the updated retriesDone count.
func TryRerunFailedWorkflows(ctx context.Context, client github.RerunFailedWorkflows, logger *slog.Logger, owner, repo, ref string, actionRetries, retriesDone int) (bool, int) {
	if actionRetries == 0 || retriesDone >= actionRetries {
		return false, retriesDone
	}

	logger.InfoContext(ctx, "CI failed, attempting to retry failed GitHub Actions",
		"retries_done", retriesDone, "retries_allowed", actionRetries)

	rerunCount, hasIncompleteRuns, err := client.RerunFailedWorkflowsForCommit(ctx, owner, repo, ref)
	if err != nil {
		logger.WarnContext(ctx, "failed to rerun workflows, will retry", "error", err)
		return true, retriesDone
	}

	if rerunCount > 0 {
		retriesDone++
		logger.InfoContext(ctx, "re-ran failed workflows, continuing to wait",
			"workflows_rerun", rerunCount, "retries_done", retriesDone)
		return true, retriesDone
	}

	if hasIncompleteRuns {
		// No concluded workflow runs to retry yet, but some are still running.
		// This happens when a job fails but other jobs in the same workflow
		// run are still in progress — the run won't have a "failure" conclusion
		// until all jobs complete. Keep waiting so we can retry on the next poll.
		logger.InfoContext(ctx, "CI failed but workflow runs still in progress, will keep waiting",
			"runs_in_progress", true, "failed_concluded_runs_count", rerunCount)
		return true, retriesDone
	}

	// No workflows were rerun and none are in progress, fail immediately
	logger.InfoContext(ctx, "CI failed with no GitHub Actions to retry, exiting")
	return false, retriesDone
}

func RunUntilCancelledOrTimeout(ctx context.Context, logger *slog.Logger, check Check, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)

	for {
		err := check.Check(ctx)
		if err != nil {
			return err
		}

		logger.InfoContext(ctx, "rechecking", "interval", interval)

		select {
		case <-ticker.C:
		case <-ctx.Done():
			logger.InfoContext(ctx, "timeout reached, exiting")
			return cli.Exit("Timeout reached", 1)
		case <-signalChan:
			logger.InfoContext(ctx, "Received SIGINT, exiting")
			return cli.Exit("Received SIGINT", 1)
		}
	}
}
