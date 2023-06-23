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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	log "github.com/sirupsen/logrus"
)

type Check interface {
	Check(ctx context.Context, recheckInterval time.Duration) error
}

func RunUntilCancelledOrTimeout(ctx context.Context, check Check, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)

	for {
		err := check.Check(ctx, interval)
		if err != nil {
			return err
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			log.Info("Timeout reached, exiting")
			return cli.Exit("Timeout reached", 1)
		case <-signalChan:
			log.Info("Received SIGINT, exiting")
			return cli.Exit("Received SIGINT", 1)
		}
	}
}
