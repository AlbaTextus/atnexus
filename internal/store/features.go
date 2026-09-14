package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRegistrationDisabled  = errors.New("registration disabled")
	ErrExternalLoginDisabled = errors.New("external login disabled")
	ErrLoginProviderDisabled = errors.New("login provider disabled")
)

type SiteFeatures struct {
	ExternalLoginEnabled bool            `json:"external_login_enabled"`
	RegistrationEnabled  bool            `json:"registration_enabled"`
	TurnstileEnabled     bool            `json:"turnstile_enabled"`
	ProviderLoginEnabled map[string]bool `json:"provider_login_enabled"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (f SiteFeatures) LoginProviderEnabled(provider string) bool {
	enabled, configured := f.ProviderLoginEnabled[provider]
	return !configured || enabled
}

type SiteFeaturesStore interface {
	SiteFeatures(context.Context) (SiteFeatures, error)
	SetSiteFeature(context.Context, string, bool, uuid.UUID) (SiteFeatures, error)
	SetLoginProvider(context.Context, string, bool, uuid.UUID) (SiteFeatures, error)
}

const siteFeaturesColumns = `external_login_enabled,registration_enabled,turnstile_enabled,provider_login_enabled,updated_at`

func scanSiteFeatures(row interface{ Scan(...any) error }) (SiteFeatures, error) {
	var features SiteFeatures
	var providers []byte
	if err := row.Scan(&features.ExternalLoginEnabled, &features.RegistrationEnabled, &features.TurnstileEnabled, &providers, &features.UpdatedAt); err != nil {
		return features, err
	}
	if err := json.Unmarshal(providers, &features.ProviderLoginEnabled); err != nil {
		return features, err
	}
	return features, nil
}

func (p *Postgres) SiteFeatures(ctx context.Context) (SiteFeatures, error) {
	return scanSiteFeatures(p.pool.QueryRow(ctx, `SELECT `+siteFeaturesColumns+` FROM site_features WHERE id=1`))
}

func (p *Postgres) SetSiteFeature(ctx context.Context, feature string, enabled bool, actor uuid.UUID) (SiteFeatures, error) {
	query := ""
	switch feature {
	case "external_login_enabled":
		query = `UPDATE site_features SET external_login_enabled=$1,updated_by=$2,updated_at=NOW() WHERE id=1 RETURNING ` + siteFeaturesColumns
	case "registration_enabled":
		query = `UPDATE site_features SET registration_enabled=$1,updated_by=$2,updated_at=NOW() WHERE id=1 RETURNING ` + siteFeaturesColumns
	case "turnstile_enabled":
		query = `UPDATE site_features SET turnstile_enabled=$1,updated_by=$2,updated_at=NOW() WHERE id=1 RETURNING ` + siteFeaturesColumns
	default:
		return SiteFeatures{}, ErrNotFound
	}
	return scanSiteFeatures(p.pool.QueryRow(ctx, query, enabled, actor))
}

func (p *Postgres) SetLoginProvider(ctx context.Context, provider string, enabled bool, actor uuid.UUID) (SiteFeatures, error) {
	switch provider {
	case "github", "linuxdo", "custom":
	default:
		return SiteFeatures{}, ErrNotFound
	}
	return scanSiteFeatures(p.pool.QueryRow(ctx, `UPDATE site_features
	 SET provider_login_enabled=jsonb_set(provider_login_enabled,ARRAY[$1]::text[],to_jsonb($2::boolean),true),
	 updated_by=$3,updated_at=NOW() WHERE id=1 RETURNING `+siteFeaturesColumns, provider, enabled, actor))
}
