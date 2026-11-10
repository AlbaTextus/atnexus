package athub

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestInvalidConfigurationFailsBeforeConnecting(t *testing.T) {
	for _, tc := range []struct {
		name, dsn, want string
	}{
		{"missing database", "", "ATHUB_DATABASE_URL is required"},
		{"blank database", "  ", "ATHUB_DATABASE_URL is required"},
		{"invalid connection", "://", "invalid ATHUB_DATABASE_URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			module, err := New(context.Background(), Config{DatabaseURL: tc.dsn, WrapHandler: func(h http.Handler) http.Handler { return h }}, nil)
			if module != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("module=%v, error=%v; want %q", module, err, tc.want)
			}
		})
	}
}

func TestHostAuthenticationRequired(t *testing.T) {
	if err := (Config{DatabaseURL: "postgresql://localhost/athub"}).Validate(); err == nil {
		t.Fatal("module must not initialize without host authentication")
	}
}
