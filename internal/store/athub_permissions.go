package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Existing-novel uploads use upserts. They also need permission to modify the
// submitted chapter/volume types, so upload cannot bypass a disabled edit rule.
func ATHubUploadEdits(path string, payload json.RawMessage) []ATHubEndpoint {
	if path != "/api/v2/upload" {
		return nil
	}
	var input struct {
		NovelID  *int32            `json:"novelId"`
		Chapters []json.RawMessage `json:"chapters"`
		Volumes  []json.RawMessage `json:"volumes"`
	}
	if json.Unmarshal(payload, &input) != nil || input.NovelID == nil {
		return nil
	}
	var endpoints []ATHubEndpoint
	if len(input.Chapters) > 0 {
		endpoints = append(endpoints, ATHubEndpoint{"PUT", "/api/v2/content", "edit"})
	}
	if len(input.Volumes) > 0 {
		endpoints = append(endpoints, ATHubEndpoint{"PUT", "/api/v2/volumes", "edit"})
	}
	return endpoints
}

var ErrATHubPermissionDenied = errors.New("ATHub operation forbidden")

func (p *Postgres) RejectATHubAdmission(ctx context.Context, id uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `UPDATE athub_api_calls SET authenticated=false WHERE id=$1`, id)
	return err
}

type ATHubEndpoint struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

func ATHubEndpoints() []ATHubEndpoint {
	var endpoints []ATHubEndpoint
	for _, path := range []string{"/api/v2/platforms", "/api/v2/authors", "/api/v2/tags", "/api/v2/novelinfo", "/api/v2/content", "/api/v2/toc", "/api/v2/volumes", "/api/v3/history", "/api/v3/contributors"} {
		endpoints = append(endpoints, ATHubEndpoint{"GET", path, "read"})
	}
	for _, path := range []string{"/api/v2/platforms", "/api/v2/authors", "/api/v2/tags", "/api/v2/novelinfo", "/api/v2/upload"} {
		endpoints = append(endpoints, ATHubEndpoint{"POST", path, "upload"})
	}
	for _, path := range []string{"/api/v2/authors", "/api/v2/tags", "/api/v2/novelinfo", "/api/v2/content", "/api/v2/volumes"} {
		endpoints = append(endpoints, ATHubEndpoint{"PUT", path, "edit"})
	}
	for _, path := range []string{"/api/v2/tags", "/api/v2/novelinfo", "/api/v2/content", "/api/v2/volumes"} {
		endpoints = append(endpoints, ATHubEndpoint{"DELETE", path, "delete"})
	}
	return endpoints
}

func ATHubOperation(method, path string) string {
	if path == "/api/v3/reviews" {
		return "review"
	}
	for _, endpoint := range ATHubEndpoints() {
		if endpoint.Method == method && endpoint.Path == path {
			return endpoint.Operation
		}
	}
	return ""
}

type ATHubAccessRule struct {
	Operation string `json:"operation"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Allowed   bool   `json:"allowed"`
	// Accepted for compatibility with older clients; ignored and never persisted.
	TotalCallLimit *int64 `json:"total_call_limit,omitempty"`
}
type ATHubRolePolicy struct {
	Role           string            `json:"role"`
	ReviewRequired bool              `json:"review_required"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Rules          []ATHubAccessRule `json:"rules"`
}
type ATHubPolicyStore interface {
	ATHubPolicies(context.Context) ([]ATHubRolePolicy, error)
	ATHubPolicy(context.Context, string) (ATHubRolePolicy, error)
	SetATHubPolicy(context.Context, uuid.UUID, ATHubRolePolicy) (ATHubRolePolicy, error)
}

func validATHubPolicy(policy ATHubRolePolicy) bool {
	if RoleLevel(policy.Role) == 0 || len(policy.Rules) > 100 {
		return false
	}
	seen, base := map[string]bool{}, map[string]bool{}
	for _, rule := range policy.Rules {
		if rule.Operation != "read" && rule.Operation != "upload" && rule.Operation != "edit" && rule.Operation != "delete" && rule.Operation != "review" {
			return false
		}
		if rule.Method == "" && rule.Path == "" {
			base[rule.Operation] = true
		} else if ATHubOperation(rule.Method, rule.Path) != rule.Operation || rule.Operation == "review" {
			return false
		}
		key := rule.Operation + " " + rule.Method + " " + rule.Path
		if seen[key] {
			return false
		}
		seen[key] = true
		// Administrative powers remain tied to account level.
		if rule.Allowed && ((rule.Operation == "review" && RoleLevel(policy.Role) < 3) || (rule.Operation == "delete" && RoleLevel(policy.Role) < 4)) {
			return false
		}
	}
	return len(base) == 5
}

func policyWith(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, role string, lock bool) (ATHubRolePolicy, error) {
	policy := ATHubRolePolicy{Role: role, Rules: []ATHubAccessRule{}}
	suffix := ""
	if lock {
		suffix = " FOR SHARE"
	}
	err := q.QueryRow(ctx, `SELECT review_required,updated_at FROM athub_role_policies WHERE role=$1`+suffix, role).Scan(&policy.ReviewRequired, &policy.UpdatedAt)
	if err != nil {
		return policy, err
	}
	rows, err := q.Query(ctx, `SELECT operation,method,path,allowed FROM athub_access_rules WHERE role=$1 ORDER BY operation,method,path`, role)
	if err != nil {
		return policy, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule ATHubAccessRule
		if err := rows.Scan(&rule.Operation, &rule.Method, &rule.Path, &rule.Allowed); err != nil {
			return policy, err
		}
		policy.Rules = append(policy.Rules, rule)
	}
	return policy, rows.Err()
}
func (p ATHubRolePolicy) Rule(operation, method, path string) ATHubAccessRule {
	rule := ATHubAccessRule{Operation: operation}
	for _, candidate := range p.Rules {
		if candidate.Operation == operation && candidate.Method == "" {
			rule = candidate
		}
	}
	for _, candidate := range p.Rules {
		if candidate.Operation == operation && candidate.Method == method && candidate.Path == path {
			return candidate
		}
	}
	return rule
}
func (p *Postgres) ATHubPolicy(ctx context.Context, role string) (ATHubRolePolicy, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ATHubRolePolicy{}, err
	}
	defer tx.Rollback(ctx)
	policy, err := policyWith(ctx, tx, role, false)
	if err != nil {
		return policy, err
	}
	return policy, tx.Commit(ctx)
}
func (p *Postgres) ATHubPolicies(ctx context.Context) ([]ATHubRolePolicy, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	policies := []ATHubRolePolicy{}
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		policy, err := policyWith(ctx, tx, role, false)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, tx.Commit(ctx)
}
func (p *Postgres) SetATHubPolicy(ctx context.Context, actor uuid.UUID, policy ATHubRolePolicy) (ATHubRolePolicy, error) {
	// Permissions only control access and moderation. Key totals are configured separately.
	for i := range policy.Rules {
		policy.Rules[i].TotalCallLimit = nil
	}
	if !validATHubPolicy(policy) || policy.UpdatedAt.IsZero() {
		return policy, ErrInvalidATHubKeyLimit
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return policy, err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT is_active AND role='SUPER_ADMIN' FROM users WHERE id=$1 FOR SHARE`, actor).Scan(&allowed); err != nil {
		return policy, err
	}
	if !allowed {
		return policy, ErrUserForbidden
	}
	var revision time.Time
	if err = tx.QueryRow(ctx, `SELECT updated_at FROM athub_role_policies WHERE role=$1 FOR UPDATE`, policy.Role).Scan(&revision); err != nil {
		return policy, err
	}
	if !revision.Equal(policy.UpdatedAt) {
		return policy, ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM athub_access_rules WHERE role=$1`, policy.Role); err != nil {
		return policy, err
	}
	for _, rule := range policy.Rules {
		if _, err = tx.Exec(ctx, `INSERT INTO athub_access_rules(role,operation,method,path,allowed) VALUES($1,$2,$3,$4,$5)`, policy.Role, rule.Operation, rule.Method, rule.Path, rule.Allowed); err != nil {
			return policy, err
		}
	}
	if err = tx.QueryRow(ctx, `UPDATE athub_role_policies SET review_required=$2,updated_at=clock_timestamp() WHERE role=$1 RETURNING updated_at`, policy.Role, policy.ReviewRequired).Scan(&policy.UpdatedAt); err != nil {
		return policy, err
	}
	return policy, tx.Commit(ctx)
}

// Run under the user's admission lock and the policy's shared lock.
func checkATHubRule(ctx context.Context, tx pgx.Tx, call ATHubCall, role string) error {
	operation := ATHubOperation(call.Method, call.Path)
	if operation == "" {
		return nil
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return err
	}
	rule := policy.Rule(operation, call.Method, call.Path)
	if !rule.Allowed {
		return ErrATHubPermissionDenied
	}
	return nil
}
