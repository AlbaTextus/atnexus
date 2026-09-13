package store

import (
	"context"
	"time"
)

// LoginDevice is advisory browser metadata, never an authentication factor.
type LoginDevice struct {
	IP          string    `json:"ip"`
	Fingerprint string    `json:"fingerprint"`
	UserAgent   string    `json:"user_agent"`
	Screen      string    `json:"screen,omitempty"`
	Timezone    string    `json:"timezone,omitempty"`
	LoggedInAt  time.Time `json:"logged_in_at"`
}

type loginDeviceKey struct{}

func WithLoginDevice(ctx context.Context, device LoginDevice) context.Context {
	return context.WithValue(ctx, loginDeviceKey{}, device)
}
