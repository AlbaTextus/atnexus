// Package migration imports an immutable SQLite snapshot into the independent
// image database. The SQLite database and object files are only ever read.
package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

type Options struct {
	SQLitePath, LocalRoot, EncryptionKey string
	Target                               *pgxpool.Pool
	Keys                                 *oauth.ImageKeys
}
type Result struct {
	Counts          map[string]int `json:"counts"`
	Digest          string         `json:"digest"`
	Keys            int            `json:"keys"`
	AlreadyImported bool           `json:"alreadyImported"`
}
type row struct {
	table, key string
	data       []byte
	fields     map[string]any
}

func digest(data []byte) string                     { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func stringValue(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func decode(raw string, v any) error {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	return d.Decode(v)
}
func domain(r row) string { return "image-legacy:" + r.table + ":" + r.key }
func validKey(key string) bool {
	if key == "" || strings.ContainsAny(key, "\\\x00:") || strings.HasPrefix(key, "/") || strings.HasPrefix(key, "_multipart/") {
		return false
	}
	for _, p := range strings.Split(key, "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}

func snapshot(ctx context.Context, opts Options) ([]row, map[string]int, error) {
	absolute, err := filepath.Abs(opts.SQLitePath)
	if err != nil {
		return nil, nil, err
	}
	p := filepath.ToSlash(absolute)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, nil, errors.New("could not open legacy SQLite database")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	tables := map[string]string{"files": "id", "settings": "key", "index_operations": "id", "index_metadata": "key", "other_data": "key"}
	names, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	ordered := []string{}
	for names.Next() {
		var name string
		if err = names.Scan(&name); err != nil {
			names.Close()
			return nil, nil, err
		}
		if tables[name] == "" {
			names.Close()
			return nil, nil, errors.New("unknown legacy table; migration requires review")
		}
		ordered = append(ordered, name)
	}
	err = names.Err()
	names.Close()
	if err != nil {
		return nil, nil, err
	}
	counts := map[string]int{}
	all := []row{}
	for _, table := range ordered {
		rows, e := tx.QueryContext(ctx, `SELECT * FROM "`+table+`" ORDER BY "`+tables[table]+`"`)
		if e != nil {
			return nil, nil, e
		}
		columns, e := rows.Columns()
		if e != nil {
			rows.Close()
			return nil, nil, e
		}
		counts[table] = 0
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if e = rows.Scan(dest...); e != nil {
				rows.Close()
				return nil, nil, e
			}
			fields := map[string]any{}
			for i, col := range columns {
				v := values[i]
				if b, ok := v.([]byte); ok {
					v = string(b)
				}
				fields[col] = v
			}
			key := stringValue(fields, tables[table])
			if key == "" {
				rows.Close()
				return nil, nil, errors.New("legacy row has no primary key")
			}
			raw, e := json.Marshal(fields)
			if e != nil {
				rows.Close()
				return nil, nil, e
			}
			all = append(all, row{table, key, raw, fields})
			counts[table]++
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, nil, e
		}
	}
	root, err := os.OpenRoot(opts.LocalRoot)
	if err != nil {
		return nil, nil, errors.New("legacy local object directory is unavailable")
	}
	defer root.Close()
	// Include object hashes in the snapshot digest; equal metadata is insufficient
	// evidence that the bytes being migrated are the original bytes.
	bytesRows := []row{}
	for _, r := range all {
		if r.table != "files" {
			continue
		}
		var meta map[string]any
		if decode(stringValue(r.fields, "metadata"), &meta) != nil || meta == nil || !validKey(r.key) {
			return nil, nil, errors.New("invalid legacy file metadata or path")
		}
		channel := stringValue(meta, "Channel")
		if channel == "" {
			channel = stringValue(r.fields, "channel")
		}
		if channel != "CloudflareR2" && channel != "S3" {
			return nil, nil, errors.New("historical storage channel requires review before migration")
		}
		if channel == "S3" {
			continue
		}
		f, e := root.Open(r.key)
		if e != nil {
			return nil, nil, errors.New("a legacy local image is missing")
		}
		st, e := f.Stat()
		if e != nil || !st.Mode().IsRegular() {
			f.Close()
			return nil, nil, errors.New("legacy image is not a regular file")
		}
		h := sha256.New()
		n, e := io.Copy(h, f)
		f.Close()
		if e != nil {
			return nil, nil, e
		}
		fields := map[string]any{"sha256": hex.EncodeToString(h.Sum(nil)), "size": n}
		raw, _ := json.Marshal(fields)
		bytesRows = append(bytesRows, row{"file_bytes", r.key, raw, fields})
	}
	all = append(all, bytesRows...)
	counts["file_bytes"] = len(bytesRows)
	sort.Slice(all, func(i, j int) bool {
		if all[i].table == all[j].table {
			return all[i].key < all[j].key
		}
		return all[i].table < all[j].table
	})
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return all, counts, nil
}

func legacyKeys(rows []row) ([]oauth.LegacyImageKey, error) {
	keys := []oauth.LegacyImageKey{}
	for _, r := range rows {
		if r.table != "settings" || r.key != "manage@sysConfig@security" {
			continue
		}
		var securityConfig struct {
			APITokens struct {
				Tokens map[string]json.RawMessage `json:"tokens"`
			} `json:"apiTokens"`
		}
		if json.Unmarshal([]byte(stringValue(r.fields, "value")), &securityConfig) != nil {
			return nil, errors.New("invalid legacy security configuration")
		}
		ids := []string{}
		for id := range securityConfig.APITokens.Tokens {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			var key oauth.LegacyImageKey
			if json.Unmarshal(securityConfig.APITokens.Tokens[id], &key) != nil {
				return nil, errors.New("invalid legacy key data")
			}
			if key.ID != "" && key.ID != id {
				return nil, errors.New("legacy key IDs differ")
			}
			key.ID = id
			if key.CreatedAt.IsZero() {
				return nil, errors.New("legacy key creation time missing")
			}
			if key.UpdatedAt.IsZero() {
				key.UpdatedAt = key.CreatedAt
			}
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func liveSetting(key string) bool {
	// Old authentication and KV index/task records remain in the encrypted
	// archive. They must never become credentials or active jobs in the Go API.
	lower := strings.ToLower(key)
	for _, part := range []string{"session", "upload_session", "index", "task", "oauth", "password", "security"} {
		if strings.Contains(lower, part) {
			return false
		}
	}
	return true
}

func Import(ctx context.Context, opts Options) (Result, error) {
	result := Result{}
	if opts.Target == nil || opts.Keys == nil {
		return result, errors.New("image database and OAuth key authority required")
	}
	cipher, err := security.NewSecretCipher(opts.EncryptionKey)
	if err != nil {
		return result, err
	}
	rows, counts, err := snapshot(ctx, opts)
	if err != nil {
		return result, err
	}
	result.Counts = counts
	h := sha256.New()
	for _, r := range rows {
		encoded, _ := json.Marshal([]string{r.table, r.key, digest(r.data)})
		h.Write(encoded)
	}
	result.Digest = hex.EncodeToString(h.Sum(nil))
	keys, err := legacyKeys(rows)
	if err != nil {
		return result, err
	}
	result.Keys = len(keys)
	tx, err := opts.Target.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = tx.Exec(ctx, `LOCK TABLE image_files,image_settings,image_legacy_rows,image_imports IN EXCLUSIVE MODE`); err != nil {
		return result, err
	}
	var prior string
	err = tx.QueryRow(ctx, `SELECT digest FROM image_imports WHERE key='sqlite-v1'`).Scan(&prior)
	if err == nil {
		if prior != result.Digest {
			return result, errors.New("legacy source changed after import; refusing to overwrite migrated data")
		}
		if err = verifyArchive(ctx, tx, cipher, rows); err != nil {
			return result, err
		}
		if err = opts.Keys.Import(ctx, keys, result.Digest); err != nil {
			return result, err
		}
		result.AlreadyImported = true
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	var occupied bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM image_files UNION ALL SELECT 1 FROM image_settings UNION ALL SELECT 1 FROM image_legacy_rows UNION ALL SELECT 1 FROM image_imports UNION ALL SELECT 1 FROM image_uploads)`).Scan(&occupied); err != nil {
		return result, err
	}
	if occupied {
		return result, errors.New("image destination is not empty; refusing to overwrite data")
	}
	for _, r := range rows {
		encrypted, e := cipher.Encrypt(domain(r), string(r.data))
		if e != nil {
			return result, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO image_legacy_rows(source_table,row_key,ciphertext,digest) VALUES($1,$2,$3,$4)`, r.table, r.key, encrypted, digest(r.data)); e != nil {
			return result, e
		}
		if r.table == "files" {
			var meta map[string]any
			if e = decode(stringValue(r.fields, "metadata"), &meta); e != nil {
				return result, e
			}
			if stringValue(meta, "Channel") == "" {
				meta["Channel"] = stringValue(r.fields, "channel")
			}
			if stringValue(meta, "ChannelName") == "" {
				meta["ChannelName"] = stringValue(r.fields, "channel_name")
			}
			raw, e := json.Marshal(meta)
			if e != nil {
				return result, e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO image_files(id,value,metadata) VALUES($1,$2,$3)`, r.key, stringValue(r.fields, "value"), raw); e != nil {
				return result, e
			}
			var equal bool
			if e = tx.QueryRow(ctx, `SELECT value=$2 AND metadata=$3::jsonb FROM image_files WHERE id=$1`, r.key, stringValue(r.fields, "value"), raw).Scan(&equal); e != nil || !equal {
				return result, errors.New("migrated file metadata did not verify")
			}
		} else if r.table == "settings" && liveSetting(r.key) {
			if _, e = tx.Exec(ctx, `INSERT INTO image_settings(key,value) VALUES($1,$2)`, r.key, stringValue(r.fields, "value")); e != nil {
				return result, e
			}
		}
	}
	if err = verifyArchive(ctx, tx, cipher, rows); err != nil {
		return result, err
	}
	// OAuth has its own durable import marker. If this transaction fails after
	// the key commit, retry verifies that marker rather than inserting twice.
	if err = opts.Keys.Import(ctx, keys, result.Digest); err != nil {
		return result, err
	}
	rawCounts, _ := json.Marshal(counts)
	if _, err = tx.Exec(ctx, `INSERT INTO image_imports(key,source,digest,counts) VALUES('sqlite-v1',$1,$2,$3)`, filepath.Base(opts.SQLitePath), result.Digest, rawCounts); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func verifyArchive(ctx context.Context, tx pgx.Tx, cipher *security.SecretCipher, rows []row) error {
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM image_legacy_rows`).Scan(&total); err != nil {
		return err
	}
	if total != len(rows) {
		return errors.New("legacy archive row count differs")
	}
	for _, r := range rows {
		var encrypted, storedDigest string
		if err := tx.QueryRow(ctx, `SELECT ciphertext,digest FROM image_legacy_rows WHERE source_table=$1 AND row_key=$2`, r.table, r.key).Scan(&encrypted, &storedDigest); err != nil {
			return err
		}
		raw, err := cipher.Decrypt(domain(r), encrypted)
		if err != nil || storedDigest != digest(r.data) || digest([]byte(raw)) != storedDigest {
			return fmt.Errorf("legacy archive integrity verification failed")
		}
	}
	return nil
}
