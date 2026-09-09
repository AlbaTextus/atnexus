package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/AlbaTextus/connect/internal/config"
	"github.com/AlbaTextus/connect/internal/email"
	"github.com/AlbaTextus/connect/internal/store"
)

func runMailDeliveries(ctx context.Context, st *store.Postgres, cfg config.SMTP, logger *slog.Logger) {
	if !cfg.Enabled() {
		return
	}
	sender := email.New(cfg)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		delivery, err := st.ClaimMailDelivery(ctx)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			logger.Error("mail queue unavailable")
			continue
		}
		sent := sender.SendMessage(ctx, delivery.Recipient, delivery.Title, delivery.Content) == nil
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err = st.FinishMailDelivery(finishCtx, delivery.ID, sent)
		cancel()
		if err != nil {
			logger.Error("mail delivery state could not be saved", "delivery_id", delivery.ID)
		}
	}
}
