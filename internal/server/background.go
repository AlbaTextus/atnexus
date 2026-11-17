package server

import (
	"context"
	"log/slog"
	"time"
)

// Every runs task immediately and then once per interval until ctx ends. Each
// run gets its own deadline; a failure is logged and retried on the next tick.
func Every(ctx context.Context, interval, timeout time.Duration, logger *slog.Logger, name string, task func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		err := task(runCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Error("background task failed", "task", name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
