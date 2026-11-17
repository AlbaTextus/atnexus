package image

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func readBody(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	decoder.UseNumber()
	if decoder.Decode(value) != nil {
		return ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func (m *Module) deleteFile(ctx context.Context, id string) error {
	if !validKey(id) {
		return ErrInvalid
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	f, err := scanFile(tx.QueryRow(ctx, `SELECT `+fileColumns+` FROM image_files WHERE id=$1 AND state IN ('ready','deleting') FOR UPDATE`, id))
	if err != nil {
		return err
	}
	store, key, err := m.storeForWith(ctx, f.Metadata, tx)
	if err != nil {
		return err
	}
	if key == "" {
		key = id
	}
	if _, err = tx.Exec(ctx, `UPDATE image_files SET state='deleting',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// The row is committed as deleting: finish even if the client went away,
	// otherwise it would stay invisible while its bytes remain.
	finish, cancel := m.detached(ctx, time.Minute)
	defer cancel()
	if err = store.Delete(finish, key); err != nil {
		return err
	}
	_, err = m.pool.Exec(finish, `DELETE FROM image_files WHERE id=$1 AND state='deleting'`, id)
	return err
}

// moveSettled reports whether a move whose commit returned an error took
// effect anyway, which happens when the reply is lost after the server has
// committed. Locking the source row waits out a transaction that is still in
// doubt, so the answer is final.
func (m *Module) moveSettled(ctx context.Context, id, target string) (bool, error) {
	var state string
	err := m.pool.QueryRow(ctx, `SELECT state FROM image_files WHERE id=$1 FOR UPDATE`, id).Scan(&state)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	// The target row is inserted by the same transaction and by nothing else:
	// the name was free when the move reserved it.
	var committed bool
	err = m.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM image_files WHERE id=$1)`, target).Scan(&committed)
	return committed, err
}

func (m *Module) moveFile(ctx context.Context, id, target string) (File, error) {
	if !validKey(id) || !validKey(target) {
		return File{}, ErrInvalid
	}
	if id == target {
		return m.getFile(ctx, id)
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	f, err := scanFile(tx.QueryRow(ctx, `SELECT `+fileColumns+` FROM image_files WHERE id=$1 AND state='ready' FOR UPDATE`, id))
	if err != nil {
		return f, err
	}
	store, key, err := m.storeForWith(ctx, f.Metadata, tx)
	if err != nil {
		return f, err
	}
	if key == "" {
		key = id
	}
	next := File{ID: target, Value: f.Value, Metadata: map[string]any{}}
	for k, v := range f.Metadata {
		next.Metadata[k] = v
	}
	next.Metadata["FileName"] = path.Base(target)
	next.Metadata["Directory"] = ""
	if dir := path.Dir(target); dir != "." {
		next.Metadata["Directory"] = dir + "/"
	}
	if text(next.Metadata, "Channel") == "S3" {
		next.Metadata["S3FileKey"] = target
	}
	raw, err := json.Marshal(next.Metadata)
	if err != nil {
		return next, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO image_files(id,value,metadata) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, target, next.Value, raw)
	if err != nil {
		return next, err
	}
	if tag.RowsAffected() == 0 {
		return next, ErrConflict
	}
	object, err := store.Open(ctx, key, "", false)
	if err != nil {
		return next, err
	}
	defer object.Body.Close()
	if err = store.Put(ctx, target, object.Body, object.Size, text(f.Metadata, "FileType")); err != nil {
		return next, err
	}
	finish, cancel := m.detached(ctx, time.Minute)
	defer cancel()
	committed := false
	_, err = tx.Exec(ctx, `UPDATE image_files SET state='deleting',updated_at=now() WHERE id=$1`, id)
	if err == nil {
		if err = tx.Commit(ctx); err == nil {
			committed = true
		} else {
			// A failed commit is not proof of a rollback: the reply can be lost
			// after the server committed. Removing the copy then would destroy
			// the only bytes of a row that is already live.
			var settleErr error
			if committed, settleErr = m.moveSettled(finish, id, target); settleErr != nil {
				m.deps.Logger.Error("image move outcome is unknown; its copy was kept", "error", safeError(settleErr))
				return next, err
			}
		}
	}
	if !committed {
		// The target row rolled back, so remove the copy that would otherwise
		// be orphaned and block that name for later uploads.
		if cleanupErr := store.Delete(finish, target); cleanupErr != nil {
			m.deps.Logger.Error("image move left an orphaned copy", "error", safeError(cleanupErr))
		}
		return next, err
	}
	// The move is committed. Removing the source is cleanup: report success and
	// leave a failed removal to the janitor, which retries deleting rows.
	if err = store.Delete(finish, key); err != nil {
		m.deps.Logger.Error("image move could not remove its source", "error", safeError(err))
		return next, nil
	}
	if _, err = m.pool.Exec(finish, `DELETE FROM image_files WHERE id=$1 AND state='deleting'`, id); err != nil {
		m.deps.Logger.Error("image move could not clear its source row", "error", safeError(err))
	}
	return next, nil
}

func (m *Module) fileAction(w http.ResponseWriter, r *http.Request, op, id string) {
	if !validKey(id) {
		m.failure(w, ErrInvalid)
		return
	}
	if op == "delete" {
		if !method(w, r, http.MethodDelete, http.MethodPost) {
			return
		}
		if r.URL.Query().Get("folder") == "true" {
			m.folderAction(w, r, id, "")
			return
		}
		err := m.deleteFile(r.Context(), id)
		if err != nil {
			m.failure(w, err)
			return
		}
		respond(w, 200, map[string]bool{"success": true})
		return
	}
	if op == "tags" && r.Method == http.MethodGet {
		f, err := m.getFile(r.Context(), id)
		if err != nil {
			m.failure(w, err)
			return
		}
		tags := f.Metadata["Tags"]
		if tags == nil {
			tags = []string{}
		}
		respond(w, 200, map[string]any{"success": true, "fileId": id, "tags": tags})
		return
	}
	if !method(w, r, http.MethodPatch, http.MethodPost) {
		return
	}
	if op == "block" || op == "unblock" {
		f, err := m.patchMetadata(r.Context(), id, func(meta map[string]any) error {
			meta["ListType"] = "None"
			if op == "block" {
				meta["ListType"] = "Block"
			}
			return nil
		})
		if err != nil {
			m.failure(w, err)
			return
		}
		respond(w, 200, map[string]any{"success": true, "metadata": safeMetadata(f.Metadata)})
		return
	}
	if op == "move" {
		dir := sanitizeFolder(r.URL.Query().Get("dist"))
		if r.URL.Query().Get("folder") == "true" {
			m.folderAction(w, r, id, dir)
			return
		}
		target := path.Join(dir, path.Base(id))
		f, err := m.moveFile(r.Context(), id, target)
		if err != nil {
			m.failure(w, err)
			return
		}
		respond(w, 200, map[string]any{"success": true, "newFileId": f.ID})
		return
	}
	var body map[string]any
	if err := readBody(w, r, &body); err != nil {
		m.failure(w, err)
		return
	}
	if op == "rename" {
		target := text(body, "newFileId")
		if target != sanitizeFolder(target) {
			m.failure(w, ErrInvalid)
			return
		}
		f, err := m.moveFile(r.Context(), id, target)
		if err != nil {
			m.failure(w, err)
			return
		}
		respond(w, 200, map[string]any{"success": true, "newFileId": f.ID})
		return
	}
	f, err := m.patchMetadata(r.Context(), id, func(meta map[string]any) error {
		if op == "tags" {
			return applyTags(meta, body)
		}
		if op != "metadata" {
			return ErrInvalid
		}
		changed := false
		for _, key := range []string{"FileName", "FileType"} {
			if value, ok := body[key]; ok {
				s, ok := value.(string)
				if !ok || len(s) > 1000 || strings.ContainsAny(s, "\r\n\x00") {
					return ErrInvalid
				}
				meta[key] = s
				changed = true
			}
		}
		if !changed {
			return ErrInvalid
		}
		return nil
	})
	if err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"success": true, "metadata": safeMetadata(f.Metadata), "fileId": id, "tags": f.Metadata["Tags"]})
}

func (m *Module) folderAction(w http.ResponseWriter, r *http.Request, folder, dest string) {
	if !validKey(folder) {
		m.failure(w, ErrInvalid)
		return
	}
	rows, err := m.pool.Query(r.Context(), `SELECT id FROM image_files WHERE starts_with(id,$1) AND state='ready' ORDER BY id`, folder+"/")
	if err != nil {
		m.failure(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			m.failure(w, err)
			return
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		m.failure(w, err)
		return
	}
	success := []string{}
	failed := []string{}
	// A move keeps its connection for the whole copy, so moves stay serial.
	workers := 1
	if !strings.Contains(r.URL.Path, "/move/") {
		workers = deleteWorkers
	}
	errs := eachFile(ids, workers, func(id string) error {
		if workers == 1 {
			_, err := m.moveFile(r.Context(), id, path.Join(dest, strings.TrimPrefix(id, folder+"/")))
			return err
		}
		return m.deleteFile(r.Context(), id)
	})
	for i, id := range ids {
		if errs[i] != nil {
			failed = append(failed, id)
		} else {
			success = append(success, id)
		}
	}
	respond(w, 200, map[string]any{"success": len(failed) == 0, "deleted": success, "failed": failed})
}

// Each delete is mostly a storage round trip and holds a database connection
// only briefly, so a few run at once without crowding the metadata pool.
const deleteWorkers = 4

// eachFile runs fn for every id with bounded concurrency and returns the
// results in input order.
func eachFile(ids []string, workers int, fn func(string) error) []error {
	errs := make([]error, len(ids))
	slots := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, id := range ids {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			errs[i] = fn(id)
		})
	}
	wg.Wait()
	return errs
}

func (m *Module) batchDelete(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input struct {
		IDs []string `json:"fileIds"`
	}
	if readBody(w, r, &input) != nil || len(input.IDs) == 0 || len(input.IDs) > 500 {
		m.failure(w, ErrInvalid)
		return
	}
	deleted := []string{}
	failed := []map[string]string{}
	errs := eachFile(input.IDs, deleteWorkers, func(id string) error { return m.deleteFile(r.Context(), id) })
	for i, id := range input.IDs {
		if err := errs[i]; err != nil {
			failed = append(failed, map[string]string{"fileId": id, "error": safeError(err)})
		} else {
			deleted = append(deleted, id)
		}
	}
	respond(w, 200, map[string]any{"success": len(failed) == 0, "deleted": deleted, "failed": failed})
}

var tagPattern = regexp.MustCompile(`^[\p{L}\p{N}_-]{1,50}$`)

func applyTags(meta, body map[string]any) error {
	action := text(body, "action")
	if action == "" {
		action = "set"
	}
	if action != "set" && action != "add" && action != "remove" {
		return ErrInvalid
	}
	raw, ok := body["tags"].([]any)
	if !ok || len(raw) > 100 {
		return ErrInvalid
	}
	tags := []string{}
	for _, v := range raw {
		s, ok := v.(string)
		if !ok || !tagPattern.MatchString(s) {
			return ErrInvalid
		}
		if !slices.Contains(tags, s) {
			tags = append(tags, s)
		}
	}
	old := []string{}
	if values, ok := meta["Tags"].([]any); ok {
		for _, v := range values {
			if s, ok := v.(string); ok {
				old = append(old, s)
			}
		}
	}
	if values, ok := meta["Tags"].([]string); ok {
		old = values
	}
	if action == "add" {
		for _, s := range tags {
			if !slices.Contains(old, s) {
				old = append(old, s)
			}
		}
		tags = old
	} else if action == "remove" {
		result := []string{}
		for _, s := range old {
			if !slices.Contains(tags, s) {
				result = append(result, s)
			}
		}
		tags = result
	}
	if len(tags) > 100 {
		return ErrInvalid
	}
	meta["Tags"] = tags
	return nil
}
func (m *Module) batchTags(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input map[string]any
	if readBody(w, r, &input) != nil {
		m.failure(w, ErrInvalid)
		return
	}
	raw, ok := input["fileIds"].([]any)
	if !ok || len(raw) == 0 || len(raw) > 500 {
		m.failure(w, ErrInvalid)
		return
	}
	if applyTags(map[string]any{}, input) != nil {
		m.failure(w, ErrInvalid)
		return
	}
	success := []string{}
	failed := []string{}
	for _, v := range raw {
		id, ok := v.(string)
		if !ok {
			m.failure(w, ErrInvalid)
			return
		}
		_, err := m.patchMetadata(r.Context(), id, func(meta map[string]any) error { return applyTags(meta, input) })
		if err != nil {
			failed = append(failed, id)
		} else {
			success = append(success, id)
		}
	}
	respond(w, 200, map[string]any{"success": len(failed) == 0, "updated": success, "failed": failed})
}
func (m *Module) tagsAutocomplete(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	rows, err := m.pool.Query(r.Context(), `SELECT tag,count(*) FROM image_files CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(metadata->'Tags')='array' THEN metadata->'Tags' ELSE '[]'::jsonb END) AS tag WHERE state='ready' AND starts_with(lower(tag),lower($1)) GROUP BY tag ORDER BY count(*) DESC,tag LIMIT 100`, r.URL.Query().Get("q"))
	if err != nil {
		m.failure(w, err)
		return
	}
	defer rows.Close()
	tags := []map[string]any{}
	for rows.Next() {
		var tag string
		var count int64
		if err = rows.Scan(&tag, &count); err != nil {
			m.failure(w, err)
			return
		}
		tags = append(tags, map[string]any{"tag": tag, "count": count})
	}
	if rows.Err() != nil {
		m.failure(w, rows.Err())
		return
	}
	respond(w, 200, map[string]any{"tags": tags})
}

func (m *Module) quota(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	rows, err := m.pool.Query(r.Context(), `SELECT COALESCE(metadata->>'ChannelName',''),count(*),COALESCE(sum(CASE WHEN metadata->>'FileSizeBytes' ~ '^[0-9]+$' THEN (metadata->>'FileSizeBytes')::numeric/1048576 WHEN metadata->>'FileSize' ~ '^[0-9]+(\.[0-9]+)?$' THEN (metadata->>'FileSize')::numeric ELSE 0 END),0) FROM image_files WHERE state='ready' GROUP BY metadata->>'ChannelName'`)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer rows.Close()
	stats := map[string]any{}
	var count int64
	var total float64
	for rows.Next() {
		var name string
		var n int64
		var size float64
		if err = rows.Scan(&name, &n, &size); err != nil {
			m.failure(w, err)
			return
		}
		stats[name] = map[string]any{"usedMB": size, "fileCount": n}
		count += n
		total += size
	}
	if rows.Err() != nil {
		m.failure(w, rows.Err())
		return
	}
	respond(w, 200, map[string]any{"success": true, "quotaStats": stats, "channelStats": stats, "totalSizeMB": total, "totalCount": count, "lastUpdated": time.Now().UnixMilli()})
}

func (m *Module) channelList(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	cfg, err := m.channels(r.Context())
	if err != nil {
		m.failure(w, err)
		return
	}
	local := []map[string]any{}
	remote := []map[string]any{}
	for _, group := range []struct {
		channels []Channel
		out      *[]map[string]any
	}{{cfg.Local.Channels, &local}, {cfg.S3.Channels, &remote}} {
		for _, c := range group.channels {
			if c.Enabled {
				*group.out = append(*group.out, map[string]any{"name": c.Name, "enabled": true})
			}
		}
	}
	respond(w, 200, map[string]any{"cfr2": map[string]any{"channels": local}, "s3": map[string]any{"channels": remote}})
}

func (m *Module) uploadSettings(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	cfg, err := m.channels(r.Context())
	if err != nil {
		m.failure(w, err)
		return
	}
	if r.Method == http.MethodGet {
		for i := range cfg.S3.Channels {
			cfg.S3.Channels[i].Secret = ""
			cfg.S3.Channels[i].AccessKey = ""
		}
		respond(w, 200, cfg)
		return
	}
	var next uploadConfig
	if readBody(w, r, &next) != nil {
		m.failure(w, ErrInvalid)
		return
	}
	seen := map[string]bool{}
	for i := range next.S3.Channels {
		c := &next.S3.Channels[i]
		if c.Name == "" || seen[c.Name] {
			m.failure(w, ErrInvalid)
			return
		}
		seen[c.Name] = true
		for _, old := range cfg.S3.Channels {
			if old.Name == c.Name {
				if c.Secret == "" {
					c.Secret = old.Secret
				}
				if c.AccessKey == "" {
					c.AccessKey = old.AccessKey
				}
			}
		}
		u, e := url.Parse(c.Endpoint)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || c.Bucket == "" || c.AccessKey == "" || c.Secret == "" {
			m.failure(w, ErrInvalid)
			return
		}
	}
	raw, err := json.Marshal(next)
	if err == nil {
		err = m.setSetting(r.Context(), "manage@sysConfig@upload", string(raw))
	}
	if err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]bool{"success": true})
}

func (m *Module) blockedIPs(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet, http.MethodPut) {
		return
	}
	if r.Method == http.MethodGet {
		raw, err := m.setting(r.Context(), "manage@blockipList")
		if err != nil {
			m.failure(w, err)
			return
		}
		ips := []string{}
		for _, s := range strings.Split(raw, ",") {
			if s != "" {
				ips = append(ips, s)
			}
		}
		respond(w, 200, map[string]any{"ips": ips})
		return
	}
	var body struct {
		IPs []string `json:"ips"`
	}
	if readBody(w, r, &body) != nil || len(body.IPs) > 10000 {
		m.failure(w, ErrInvalid)
		return
	}
	if err := m.setSetting(r.Context(), "manage@blockipList", strings.Join(body.IPs, ",")); err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]bool{"success": true})
}

func (m *Module) exportFiles(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 1000 {
			m.failure(w, ErrInvalid)
			return
		}
		limit = n
	}
	rows, err := m.pool.Query(r.Context(), `SELECT `+fileColumns+` FROM image_files WHERE id>$1 AND state='ready' ORDER BY id LIMIT $2`, r.URL.Query().Get("cursor"), limit+1)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer rows.Close()
	records := []map[string]any{}
	ids := []string{}
	for rows.Next() {
		f, e := scanFile(rows)
		if e != nil {
			m.failure(w, e)
			return
		}
		records = append(records, map[string]any{"id": f.ID, "value": f.Value, "metadata": safeMetadata(f.Metadata)})
		ids = append(ids, f.ID)
	}
	if rows.Err() != nil {
		m.failure(w, rows.Err())
		return
	}
	complete := len(records) <= limit
	cursor := ""
	if !complete {
		records = records[:limit]
		cursor = ids[limit-1]
	}
	respond(w, 200, map[string]any{"success": true, "records": records, "cursor": cursor, "list_complete": complete})
}

func (m *Module) settingsBackup(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	rows, err := m.pool.Query(r.Context(), `SELECT key,value FROM image_settings WHERE key NOT LIKE 'manage@session@%' AND key NOT LIKE 'manage@index%' AND key<>'manage@sysConfig@security' AND key<>'manage@sysConfig@upload' ORDER BY key`)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if err = rows.Scan(&k, &v); err != nil {
			m.failure(w, err)
			return
		}
		settings[k] = v
	}
	if rows.Err() != nil {
		m.failure(w, rows.Err())
		return
	}
	respond(w, 200, map[string]any{"success": true, "settings": settings})
}
func (m *Module) restoreFiles(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input struct {
		Type string `json:"type"`
		Data map[string]struct {
			Value    string         `json:"value"`
			Metadata map[string]any `json:"metadata"`
		} `json:"data"`
	}
	if readBody(w, r, &input) != nil || input.Type != "files" || len(input.Data) == 0 || len(input.Data) > 1000 {
		m.failure(w, ErrInvalid)
		return
	}
	tx, err := m.pool.Begin(r.Context())
	if err != nil {
		m.failure(w, err)
		return
	}
	defer tx.Rollback(context.WithoutCancel(r.Context()))
	for id, record := range input.Data {
		if !validKey(id) || record.Metadata == nil {
			m.failure(w, ErrInvalid)
			return
		}
		// Export removes connection details. A restored S3 record therefore needs
		// its configured channel identity rather than anonymous legacy credentials.
		if text(record.Metadata, "Channel") == "S3" && text(record.Metadata, "ChannelName") == "" {
			m.failure(w, ErrInvalid)
			return
		}
		store, key, e := m.storeForWith(r.Context(), record.Metadata, tx)
		if e != nil {
			m.failure(w, e)
			return
		}
		if key == "" {
			key = id
		}
		info, e := store.Open(r.Context(), key, "", true)
		if info.Body != nil {
			info.Body.Close()
		}
		if e != nil {
			m.failure(w, e)
			return
		}
		raw, e := json.Marshal(safeMetadata(record.Metadata))
		if e != nil {
			m.failure(w, e)
			return
		}
		tag, e := tx.Exec(r.Context(), `INSERT INTO image_files(id,value,metadata) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, id, record.Value, raw)
		if e != nil {
			m.failure(w, e)
			return
		}
		if tag.RowsAffected() == 0 {
			m.failure(w, ErrConflict)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"success": true, "restoredCount": len(input.Data), "failedCount": 0})
}
