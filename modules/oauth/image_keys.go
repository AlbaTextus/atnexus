package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImageKeys owns external image credentials in the OAuth account database.
type ImageKeys struct {
	pool     *pgxpool.Pool
	sessions auth.Verifier
}
type ImageKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Owner       string     `json:"owner"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Disabled    bool       `json:"disabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	AutoDelete  bool       `json:"autoDelete"`
}

// NewImageKeysWithPool uses the caller's account-database pool and never closes it.
func NewImageKeysWithPool(ctx context.Context, p *pgxpool.Pool, sessions auth.Verifier) (*ImageKeys, error) {
	rows, err := p.Query(ctx, `SELECT id,token_hash,permissions,disabled FROM image_api_keys LIMIT 0`)
	if err != nil {
		return nil, errors.New("OAuth image key schema not ready; run npm run db:deploy")
	}
	rows.Close()
	return &ImageKeys{pool: p, sessions: sessions}, nil
}
func (s *ImageKeys) AuthorizeImageKey(ctx context.Context, raw, permission string) (string, error) {
	var id string
	var scopes []string
	err := s.pool.QueryRow(ctx, `SELECT id,permissions FROM image_api_keys WHERE token_hash=$1 AND NOT disabled AND (expires_at IS NULL OR expires_at>now())`, security.HashToken(raw)).Scan(&id, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrUnauthenticated
	}
	if err != nil {
		return "", errors.New("OAuth image key authority unavailable")
	}
	if !slices.Contains(scopes, permission) {
		return "", auth.ErrForbidden
	}
	return id, nil
}

func validImagePermissions(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > 4 {
		return false
	}
	for i, p := range scopes {
		if !slices.Contains([]string{"upload", "list", "delete", "manage"}, p) || slices.Contains(scopes[:i], p) {
			return false
		}
	}
	return true
}

func (s *ImageKeys) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(code int, reason string) { reply(code, map[string]string{"error": reason}) }
	if s.sessions == nil {
		fail(503, "authentication_unavailable")
		return
	}
	identity, err := s.sessions.VerifySession(r)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			fail(401, "login_required")
		} else {
			fail(503, "authentication_unavailable")
		}
		return
	}
	if identity.Role != "ADMIN" && identity.Role != "SUPER_ADMIN" {
		fail(403, "admin_required")
		return
	}
	if r.Method != http.MethodGet && (r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
		fail(403, "invalid_origin")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.pool.Query(r.Context(), `SELECT id,name,owner,permissions,expires_at,disabled,created_at,updated_at,auto_delete FROM image_api_keys ORDER BY created_at DESC,id`)
		if err != nil {
			fail(503, "key_store_unavailable")
			return
		}
		defer rows.Close()
		items := []ImageKey{}
		for rows.Next() {
			var k ImageKey
			if err = rows.Scan(&k.ID, &k.Name, &k.Owner, &k.Permissions, &k.ExpiresAt, &k.Disabled, &k.CreatedAt, &k.UpdatedAt, &k.AutoDelete); err != nil {
				fail(503, "key_store_unavailable")
				return
			}
			items = append(items, k)
		}
		if rows.Err() != nil {
			fail(503, "key_store_unavailable")
			return
		}
		reply(200, map[string]any{"tokens": items})
		return
	}
	if r.Method == http.MethodDelete {
		id := r.URL.Query().Get("id")
		if id == "" {
			fail(400, "id_required")
			return
		}
		tag, err := s.pool.Exec(r.Context(), `UPDATE image_api_keys SET disabled=true,updated_at=now() WHERE id=$1`, id)
		if err != nil {
			fail(503, "key_store_unavailable")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(404, "key_not_found")
			return
		}
		reply(200, map[string]bool{"success": true})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		fail(405, "method_not_allowed")
		return
	}
	var input struct {
		Name        string     `json:"name"`
		Owner       string     `json:"owner"`
		TokenID     string     `json:"tokenId"`
		Permissions []string   `json:"permissions"`
		ExpiresAt   *time.Time `json:"expiresAt"`
		Disabled    bool       `json:"disabled"`
		AutoDelete  bool       `json:"autoDelete"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&input) != nil || !validImagePermissions(input.Permissions) || input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		fail(400, "invalid_key_request")
		return
	}
	if r.Method == http.MethodPut {
		if input.TokenID == "" {
			fail(400, "id_required")
			return
		}
		tag, err := s.pool.Exec(r.Context(), `UPDATE image_api_keys SET permissions=$2,expires_at=$3,disabled=$4,auto_delete=$5,updated_at=now() WHERE id=$1`, input.TokenID, input.Permissions, input.ExpiresAt, input.Disabled, input.AutoDelete)
		if err != nil {
			fail(503, "key_store_unavailable")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(404, "key_not_found")
			return
		}
		reply(200, map[string]bool{"success": true})
		return
	}
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 200 || len(input.Owner) > 320 {
		fail(400, "invalid_key_request")
		return
	}
	raw, err := security.RandomToken(32)
	if err != nil {
		fail(503, "key_generation_unavailable")
		return
	}
	raw = "imgbed_" + raw
	id := uuid.NewString()
	owner := input.Owner
	if owner == "" {
		owner = identity.UserID.String()
	}
	_, err = s.pool.Exec(r.Context(), `INSERT INTO image_api_keys(id,name,owner,token_hash,permissions,expires_at,auto_delete) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, input.Name, owner, security.HashToken(raw), input.Permissions, input.ExpiresAt, input.AutoDelete)
	if err != nil {
		fail(503, "key_store_unavailable")
		return
	}
	reply(201, map[string]any{"id": id, "name": input.Name, "owner": owner, "token": raw, "permissions": input.Permissions, "expiresAt": input.ExpiresAt})
}
