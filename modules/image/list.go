package image

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type filter struct {
	conditions []string
	args       []any
}

func (f *filter) add(template string, args ...any) {
	for _, arg := range args {
		f.args = append(f.args, arg)
		template = strings.Replace(template, "?", fmt.Sprintf("$%d", len(f.args)), 1)
	}
	f.conditions = append(f.conditions, template)
}
func (f filter) where() string { return strings.Join(f.conditions, " AND ") }
func imageFilter(q url.Values, category string) (filter, string, error) {
	f := filter{conditions: []string{"state='ready'"}}
	dir := sanitizeFolder(q.Get("dir"))
	entity := q.Get("entityId")
	if category != "" {
		p := strings.Split(category, "/")
		root := p[0]
		if entity != "" && !validEntity(root, entity) {
			return f, dir, ErrInvalid
		}
		if len(p) == 1 {
			prefix := root + "/"
			if entity != "" {
				prefix += entity + "/"
			}
			if dir != "" {
				prefix += dir + "/"
			}
			f.add("starts_with(id,?)", prefix)
			dir = strings.TrimSuffix(prefix, "/")
		} else {
			kind := p[1]
			suffix := dir
			if entity != "" {
				if suffix == entity || strings.HasPrefix(suffix, entity+"/") {
					suffix = strings.TrimPrefix(strings.TrimPrefix(suffix, entity), "/")
				}
				f.add("(starts_with(id,?) OR starts_with(id,?))", pathPrefix(root, entity, kind, suffix), pathPrefix(root, kind, entity, suffix))
			} else if suffix != "" {
				owner, rest, _ := strings.Cut(suffix, "/")
				if !validEntity(root, owner) {
					return f, dir, ErrInvalid
				}
				f.add("(starts_with(id,?) OR starts_with(id,?))", pathPrefix(root, owner, kind, rest), pathPrefix(root, kind, owner, rest))
			} else {
				f.add("(starts_with(id,?) OR (split_part(id,'/',1)=? AND split_part(id,'/',3)=?))", root+"/"+kind+"/", root, kind)
			}
			dir = root
		}
	} else if dir != "" {
		f.add("starts_with(id,?)", dir+"/")
	}
	if search := strings.TrimSpace(q.Get("search")); search != "" {
		f.add("(strpos(lower(id),lower(?))>0 OR strpos(lower(COALESCE(metadata->>'FileName','')),lower(?))>0)", search, search)
	}
	for _, item := range [][2]string{{"channel", "Channel"}, {"channelName", "ChannelName"}, {"listType", "ListType"}} {
		if raw := q.Get(item[0]); raw != "" {
			f.add("metadata->>'"+item[1]+"'=ANY(?::text[])", strings.Split(raw, ","))
		}
	}
	if raw := q.Get("fileType"); raw != "" {
		pieces := []string{}
		for _, kind := range strings.Split(raw, ",") {
			pieces = append(pieces, "starts_with(COALESCE(metadata->>'FileType',''),?)")
			f.args = append(f.args, kind)
			pieces[len(pieces)-1] = strings.Replace(pieces[len(pieces)-1], "?", fmt.Sprintf("$%d", len(f.args)), 1)
		}
		f.conditions = append(f.conditions, "("+strings.Join(pieces, " OR ")+")")
	}
	for _, item := range []struct {
		key     string
		exclude bool
	}{{"includeTags", false}, {"excludeTags", true}} {
		for _, tag := range strings.Split(q.Get(item.key), ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			// Containment on the whole column can use the metadata GIN index.
			condition := "metadata @> ?::jsonb"
			if item.exclude {
				condition = "NOT (" + condition + ")"
			}
			encoded, _ := json.Marshal(map[string][]string{"Tags": {tag}})
			f.add(condition, string(encoded))
		}
	}
	if status := q.Get("accessStatus"); status != "" {
		switch status {
		case "blocked":
			f.add("metadata->>'ListType'='Block'")
		case "accessible":
			f.add("COALESCE(metadata->>'ListType','None')<>'Block'")
		default:
			return f, dir, ErrInvalid
		}
	}
	return f, dir, nil
}
func pathPrefix(parts ...string) string {
	out := []string{}
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/") + "/"
}

func safeMetadata(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	for _, k := range []string{"S3AccessKeyId", "S3SecretAccessKey", "TgBotToken", "DiscordBotToken", "HfToken", "WebDAVUsername", "WebDAVPassword", "WebDAVHeaders", "S3Location", "S3Endpoint", "S3PathStyle", "S3Region", "S3BucketName", "S3CdnFileUrl", "UploadSession"} {
		delete(out, k)
	}
	if value, ok := out["WebDAVBaseUrl"].(string); ok {
		if u, e := url.Parse(value); e == nil {
			u.User = nil
			out["WebDAVBaseUrl"] = u.String()
		} else {
			delete(out, "WebDAVBaseUrl")
		}
	}
	return out
}

func (m *Module) list(w http.ResponseWriter, r *http.Request, category string) {
	if !method(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	if q.Has("action") {
		if category != "" {
			respondError(w, 400, "category_list_action_forbidden")
			return
		}
		if q.Get("action") == "info" || q.Get("action") == "index-storage-stats" {
			m.quota(w, r)
			return
		}
		respondError(w, 400, "PostgreSQL_indexes_are_managed_by_migrations")
		return
	}
	f, dir, err := imageFilter(q, category)
	if err != nil {
		m.failure(w, err)
		return
	}
	limit := 50
	if q.Has("count") {
		limit, err = strconv.Atoi(q.Get("count"))
		if err != nil || limit == 0 || limit < -1 || limit > 10000 {
			m.failure(w, ErrInvalid)
			return
		}
	}
	offset := 0
	if q.Has("start") {
		offset, err = strconv.Atoi(q.Get("start"))
		if err != nil || offset < 0 {
			m.failure(w, ErrInvalid)
			return
		}
	}
	tx, err := m.pool.Begin(r.Context())
	if err != nil {
		m.failure(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`); err != nil {
		m.failure(w, err)
		return
	}
	var total int64
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM image_files WHERE `+f.where(), f.args...).Scan(&total); err != nil {
		m.failure(w, err)
		return
	}
	if limit == -1 && q.Get("sum") == "true" {
		respond(w, 200, map[string]any{"sum": total, "indexLastUpdated": time.Now().UnixMilli()})
		return
	}
	// Directory discovery queries actual rows, including paths of imported files.
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	start := utf8.RuneCountInString(prefix) + 1
	args := append(append([]any{}, f.args...), start)
	rows, err := tx.Query(r.Context(), `SELECT DISTINCT split_part(substring(id FROM $`+strconv.Itoa(len(args))+`::integer), '/',1) FROM image_files WHERE `+f.where()+` AND strpos(substring(id FROM $`+strconv.Itoa(len(args))+`::integer),'/')>0`, args...)
	if err != nil {
		m.failure(w, err)
		return
	}
	directories := []string{}
	for rows.Next() {
		var child string
		if err = rows.Scan(&child); err != nil {
			rows.Close()
			m.failure(w, err)
			return
		}
		directories = append(directories, prefix+child)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		m.failure(w, err)
		return
	}
	if category == "" && q.Get("search") == "" && q.Get("channel") == "" && q.Get("channelName") == "" && q.Get("listType") == "" && q.Get("fileType") == "" && q.Get("includeTags") == "" && q.Get("excludeTags") == "" {
		if dir == "" {
			directories = append(directories, "user", "book", "forum")
		}
		parts := strings.Split(dir, "/")
		if len(parts) == 2 && validEntity(parts[0], parts[1]) {
			kinds := []string{"covers", "illustrations"}
			if parts[0] == "user" {
				kinds = []string{"avatars", "backgrounds"}
			}
			for _, kind := range kinds {
				directories = append(directories, dir+"/"+kind)
			}
		}
	}
	sort.Strings(directories)
	directories = uniqueStrings(directories)
	direct := f
	if category == "" && q.Get("recursive") != "true" {
		direct.conditions = append([]string{}, f.conditions...)
		direct.args = append([]any{}, f.args...)
		direct.add("strpos(substring(id FROM ?::integer),'/')=0", start)
	}
	var directCount int64
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM image_files WHERE `+direct.where(), direct.args...).Scan(&directCount); err != nil {
		m.failure(w, err)
		return
	}
	query := `SELECT ` + fileColumns + ` FROM image_files WHERE ` + direct.where() + ` ORDER BY CASE WHEN metadata->>'TimeStamp' ~ '^[0-9]+$' THEN (metadata->>'TimeStamp')::numeric ELSE 0 END DESC,id`
	args = append([]any{}, direct.args...)
	if limit > 0 {
		args = append(args, limit, offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err = tx.Query(r.Context(), query, args...)
	if err != nil {
		m.failure(w, err)
		return
	}
	files := []File{}
	for rows.Next() {
		item, e := scanFile(rows)
		if e != nil {
			rows.Close()
			m.failure(w, e)
			return
		}
		item.Metadata = safeMetadata(item.Metadata)
		files = append(files, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"files": files, "directories": directories, "totalCount": total, "directFileCount": directCount, "directFolderCount": len(directories), "returnedCount": len(files), "indexLastUpdated": time.Now().UnixMilli(), "isIndexedResponse": true})
}
func uniqueStrings(v []string) []string {
	if len(v) == 0 {
		return []string{}
	}
	out := v[:1]
	for _, s := range v[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

func (m *Module) directoryTree(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	rows, err := m.pool.Query(r.Context(), `SELECT DISTINCT metadata->>'Directory' FROM image_files WHERE state='ready'`)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer rows.Close()
	dirs := []string{"user", "book", "forum"}
	for rows.Next() {
		var value *string
		if err = rows.Scan(&value); err != nil {
			m.failure(w, err)
			return
		}
		if value != nil {
			parts := strings.Split(strings.Trim(*value, "/"), "/")
			for i := 1; i <= len(parts); i++ {
				if parts[0] != "" {
					dirs = append(dirs, strings.Join(parts[:i], "/"))
				}
			}
		}
	}
	if rows.Err() != nil {
		m.failure(w, rows.Err())
		return
	}
	sort.Strings(dirs)
	respond(w, 200, map[string]any{"directories": uniqueStrings(dirs)})
}
