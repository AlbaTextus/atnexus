package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/AlbaTextus/connect/internal/store"
)

func runWalletSettlements(ctx context.Context, st *store.Postgres, logger *slog.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		for {
			count, err := st.SettleDueWallets(batchCtx, 100)
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("wallet settlement failed", "error", err)
				}
				break
			}
			if count < 100 {
				break
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
