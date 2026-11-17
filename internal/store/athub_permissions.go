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
	// A rejected call returns its admission to the key's quota counter.
	_, err := p.pool.Exec(ctx, `WITH rejected AS (
			UPDATE athub_api_calls SET authenticated=false WHERE id=$1 AND authenticated RETURNING key_id)
		UPDATE athub_api_keys k SET authenticated_calls=k.authenticated_calls-1 FROM rejected WHERE k.id=rejected.key_id`, id)
	return err
}

type ATHubEndpoint struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

// athubRoutes is the single description of the content API surface. Permission
// rules, audit labels, creator submissions and review queueing all derive from
// it, so a new endpoint is declared here once.
var athubRoutes = []struct {
	ATHubEndpoint
	// submission: the creator platform may call it on behalf of a signed-in user.
	submission bool
	// reviewQuery: query parameters accepted when the call is queued for review.
	reviewQuery []string
}{
	{ATHubEndpoint{"GET", "/api/v2/platforms", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/authors", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/tags", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/novelinfo", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/content", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/toc", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/volumes", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/illustrations", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v2/illustration-layout", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v3/history", "read"}, false, nil},
	{ATHubEndpoint{"GET", "/api/v3/contributors", "read"}, false, nil},

	{ATHubEndpoint{"POST", "/api/v2/platforms", "upload"}, false, nil},
	{ATHubEndpoint{"POST", "/api/v2/authors", "upload"}, false, nil},
	{ATHubEndpoint{"POST", "/api/v2/tags", "upload"}, false, nil},
	{ATHubEndpoint{"POST", "/api/v2/novelinfo", "upload"}, false, nil},
	{ATHubEndpoint{"POST", "/api/v2/upload", "upload"}, true, nil},
	{ATHubEndpoint{"POST", "/api/v2/illustrations", "upload"}, true, []string{"novelId"}},

	{ATHubEndpoint{"PUT", "/api/v2/authors", "edit"}, false, []string{"id"}},
	{ATHubEndpoint{"PUT", "/api/v2/tags", "edit"}, false, []string{"id"}},
	{ATHubEndpoint{"PUT", "/api/v2/novelinfo", "edit"}, true, []string{"id"}},
	{ATHubEndpoint{"PUT", "/api/v2/content", "edit"}, false, nil},
	{ATHubEndpoint{"PUT", "/api/v2/volumes", "edit"}, false, nil},
	{ATHubEndpoint{"PUT", "/api/v2/illustration-layout", "edit"}, true, []string{"novelId"}},

	{ATHubEndpoint{"DELETE", "/api/v2/tags", "delete"}, false, []string{"id"}},
	{ATHubEndpoint{"DELETE", "/api/v2/novelinfo", "delete"}, false, []string{"id"}},
	{ATHubEndpoint{"DELETE", "/api/v2/content", "delete"}, false, []string{"novelId", "order"}},
	{ATHubEndpoint{"DELETE", "/api/v2/volumes", "delete"}, false, []string{"novelId", "order"}},
	{ATHubEndpoint{"DELETE", "/api/v2/illustrations", "delete"}, true, []string{"novelId", "id"}},
}

// Built once: every ATHub request resolves its route.
var athubRouteIndex, athubPaths = func() (map[[2]string]int, map[string]bool) {
	index, paths := make(map[[2]string]int, len(athubRoutes)), make(map[string]bool)
	for i, route := range athubRoutes {
		index[[2]string{route.Method, route.Path}] = i
		paths[route.Path] = true
	}
	return index, paths
}()

func ATHubEndpoints() []ATHubEndpoint {
	endpoints := make([]ATHubEndpoint, len(athubRoutes))
	for i, route := range athubRoutes {
		endpoints[i] = route.ATHubEndpoint
	}
	return endpoints
}

func ATHubOperation(method, path string) string {
	if path == "/api/v3/reviews" {
		return "review"
	}
	if i, ok := athubRouteIndex[[2]string{method, path}]; ok {
		return athubRoutes[i].Operation
	}
	return ""
}

// ATHubKnownPath reports whether a path belongs to the content API.
func ATHubKnownPath(path string) bool { return athubPaths[path] }

// ATHubSubmissionEndpoint reports whether the creator platform may forward
// this call with a submitter's identity.
func ATHubSubmissionEndpoint(method, path string) bool {
	i, ok := athubRouteIndex[[2]string{method, path}]
	return ok && athubRoutes[i].submission
}

// ATHubReviewQuery lists the query parameters a queued call may carry.
func ATHubReviewQuery(method, path string) []string {
	if i, ok := athubRouteIndex[[2]string{method, path}]; ok {
		return athubRoutes[i].reviewQuery
	}
	return nil
}

// ATHubMutation reports whether an operation changes content and can be reviewed.
func ATHubMutation(operation string) bool {
	return operation == "upload" || operation == "edit" || operation == "delete"
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
	// Wait out calls admitted under the old rules; later ones queue behind this.
	// It follows the policy row lock so this transaction never waits for a row
	// while calls queue behind it.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(`+athubRoleLock(`$1`)+`)`, policy.Role); err != nil {
		return policy, err
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
