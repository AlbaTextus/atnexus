package image

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
)

var unsafeName = regexp.MustCompile("[\\\\/:*?\"'<>| ()\\[\\]{}#%^`~;@&=+$,\\x00-\\x1f]")

func sanitizeFolder(value string) string {
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ReplaceAll(value, "\\", "/")
	parts := []string{}
	for _, p := range strings.Split(value, "/") {
		p = strings.ReplaceAll(p, "..", "_")
		if p == "." {
			p = "_"
		}
		p = unsafeName.ReplaceAllString(p, "_")
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}
func validEntity(root, value string) bool {
	if root == "user" {
		_, err := uuid.Parse(value)
		return err == nil && len(value) == 36
	}
	return root == "book" && regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`).MatchString(value) && value != "covers" && value != "illustrations"
}
func categoryFolder(category, relative, entity string) (string, error) {
	parts := strings.Split(category, "/")
	root := parts[0]
	if len(parts) == 1 && (entity == "" || root == "forum") {
		return path.Join(root, relative), nil
	}
	segments := strings.Split(relative, "/")
	if entity == "" {
		entity = segments[0]
	}
	if !validEntity(root, entity) {
		return "", ErrInvalid
	}
	if segments[0] == entity {
		segments = segments[1:]
	}
	prefix := root + "/" + entity
	if len(parts) > 1 {
		prefix += "/" + parts[1]
	}
	return path.Join(prefix, strings.Join(segments, "/")), nil
}

type parsedForm struct {
	Values                  map[string]string
	File                    *os.File
	ScratchName, Name, Type string
	Size                    int64
}
type uploadReceiptKey struct{}

func (m *Module) readForm(w http.ResponseWriter, r *http.Request, limit int64) (*parsedForm, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+(128<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, ErrInvalid
	}
	f := &parsedForm{Values: map[string]string{}}
	fail := func(err error) (*parsedForm, error) { f.close(m); return nil, err }
	fields := 0
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			return fail(ErrInvalid)
		}
		fields++
		if fields > 32 {
			return fail(ErrInvalid)
		}
		name := part.FormName()
		if part.FileName() == "" {
			data, e := io.ReadAll(io.LimitReader(part, 64<<10+1))
			part.Close()
			if e != nil || len(data) > 64<<10 {
				return fail(ErrInvalid)
			}
			if _, exists := f.Values[name]; exists {
				return fail(ErrInvalid)
			}
			f.Values[name] = string(data)
			continue
		}
		if name != "file" || f.File != nil {
			return fail(ErrInvalid)
		}
		f.ScratchName = "request-" + uuid.NewString()
		f.File, err = m.scratch.OpenFile(f.ScratchName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return fail(err)
		}
		f.Name = part.FileName()
		f.Type = part.Header.Get("Content-Type")
		if f.Type == "" {
			f.Type = "application/octet-stream"
		}
		f.Size, err = io.Copy(f.File, io.LimitReader(part, limit+1))
		part.Close()
		if err != nil {
			return fail(err)
		}
		if f.Size > limit {
			return fail(&http.MaxBytesError{Limit: limit})
		}
	}
	if f.File != nil {
		_, err = f.File.Seek(0, 0)
		if err != nil {
			return fail(err)
		}
	}
	return f, nil
}
func (f *parsedForm) close(m *Module) {
	if f.File != nil {
		f.File.Close()
	}
	if f.ScratchName != "" {
		_ = m.scratch.Remove(f.ScratchName)
	}
}

func (m *Module) categoryUpload(w http.ResponseWriter, r *http.Request, category string) {
	if !method(w, r, http.MethodPost) {
		return
	}
	q := r.URL.Query()
	for _, name := range []string{"initChunked", "chunked", "merge", "cleanup"} {
		if q.Has(name) {
			respondError(w, 400, "category_requires_regular_upload")
			return
		}
	}
	folder, err := categoryFolder(category, sanitizeFolder(q.Get("uploadFolder")), q.Get("entityId"))
	if err != nil {
		m.failure(w, err)
		return
	}
	copy := r.Clone(r.Context())
	q.Set("uploadFolder", folder)
	copy.URL.RawQuery = q.Encode()
	m.upload(w, copy)
}

func (m *Module) upload(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	blocked, err := m.setting(r.Context(), "manage@blockipList")
	if err != nil {
		m.failure(w, err)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	for _, value := range strings.Split(blocked, ",") {
		if strings.TrimSpace(value) == ip && ip != "" {
			respondError(w, 403, "upload_ip_blocked")
			return
		}
	}
	if r.URL.Query().Get("cleanup") == "true" {
		m.cleanupChunks(w, r)
		return
	}
	limit := m.cfg.MaxUploadBytes
	if r.URL.Query().Get("chunked") == "true" {
		limit = m.cfg.MaxChunkBytes
	}
	form, err := m.readForm(w, r, limit)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondError(w, 413, "upload_too_large")
		} else {
			m.failure(w, err)
		}
		return
	}
	defer form.close(m)
	if r.URL.Query().Get("initChunked") == "true" {
		m.initChunks(w, r, form)
		return
	}
	if r.URL.Query().Get("chunked") == "true" {
		m.chunk(w, r, form)
		return
	}
	if form.File == nil {
		respondError(w, 400, "file_required")
		return
	}
	result, err := m.saveUpload(r.Context(), form.Name, form.Type, form.File, form.Size, r.URL.Query(), ip)
	if err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, result)
}

func (m *Module) chooseChannels(ctx context.Context, q url.Values) ([]Channel, error) {
	group := q.Get("uploadChannel")
	if group != "" && group != "cfr2" && group != "s3" {
		return nil, ErrInvalid
	}
	cfg, err := m.channels(ctx)
	if err != nil {
		return nil, err
	}
	selected := cfg.Local
	fallback := cfg.S3
	if group == "s3" {
		selected, fallback = cfg.S3, cfg.Local
	}
	choose := func(g channelGroup) []Channel {
		enabled := []Channel{}
		for _, c := range g.Channels {
			if c.Enabled && (q.Get("channelName") == "" || c.Name == q.Get("channelName")) {
				enabled = append(enabled, c)
			}
		}
		if len(enabled) > 1 && g.LoadBalance.Enabled {
			b := make([]byte, 1)
			_, _ = rand.Read(b)
			return []Channel{enabled[int(b[0])%len(enabled)]}
		}
		if len(enabled) > 0 {
			return enabled[:1]
		}
		return nil
	}
	selectedChannels := choose(selected)
	if len(selectedChannels) == 0 {
		return nil, errors.New("requested storage channel unavailable")
	}
	if q.Get("autoRetry") != "false" && q.Get("channelName") == "" {
		selectedChannels = append(selectedChannels, choose(fallback)...)
	}
	return selectedChannels, nil
}

func (m *Module) saveUpload(ctx context.Context, fileName, fileType string, body io.ReadSeeker, size int64, q url.Values, ip string) ([]map[string]string, error) {
	receipt, _ := ctx.Value(uploadReceiptKey{}).(string)
	if receipt != "" {
		var id, state string
		err := m.pool.QueryRow(ctx, `SELECT id,state FROM image_files WHERE metadata ? 'UploadSession' AND metadata->>'UploadSession'=$1`, receipt).Scan(&id, &state)
		if err == nil {
			if state != "ready" {
				return nil, ErrConflict
			}
			return m.uploadResult(id, q), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	channels, err := m.chooseChannels(ctx, q)
	if err != nil {
		return nil, err
	}
	folder := sanitizeFolder(q.Get("uploadFolder"))
	name := unsafeName.ReplaceAllString(path.Base(strings.ReplaceAll(fileName, "\\", "/")), "_")
	if name == "" || name == "." || name == ".." {
		return nil, ErrInvalid
	}
	if len(name) > 240 {
		return nil, ErrInvalid
	}
	contentType, _, err := mime.ParseMediaType(fileType)
	if err != nil {
		contentType = "application/octet-stream"
	}
	metadata := map[string]any{"FileName": fileName, "FileType": contentType, "FileSize": fmt.Sprintf("%.2f", float64(size)/(1024*1024)), "FileSizeBytes": size, "UploadIP": ip, "ListType": "None", "TimeStamp": time.Now().UnixMilli(), "Directory": "", "Tags": []string{}}
	if receipt != "" {
		metadata["UploadSession"] = receipt
	}
	if folder != "" {
		metadata["Directory"] = folder + "/"
	}
	if _, err = body.Seek(0, 0); err != nil {
		return nil, err
	}
	if cfg, _, e := image.DecodeConfig(io.LimitReader(body, 1<<20)); e == nil {
		metadata["Width"] = cfg.Width
		metadata["Height"] = cfg.Height
	}
	suffix := path.Ext(name)
	if suffix == "" {
		suffix = ".bin"
	}
	base := name
	switch q.Get("uploadNameType") {
	case "origin":
	case "index":
		base = strconv.FormatInt(time.Now().UnixMilli(), 10) + "-" + uuid.NewString()[:8] + suffix
	case "short":
		random := make([]byte, 8)
		if _, err = rand.Read(random); err != nil {
			return nil, err
		}
		base = hex.EncodeToString(random) + suffix
	case "", "default":
		base = strconv.FormatInt(time.Now().UnixMilli(), 10) + "-" + uuid.NewString()[:8] + "_" + name
	default:
		return nil, ErrInvalid
	}
	for _, channel := range channels {
		tx, err := m.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,208))`, channel.Name); err != nil {
			tx.Rollback(context.WithoutCancel(ctx))
			return nil, err
		}
		if channel.Quota.Enabled && channel.Quota.LimitGB > 0 {
			var used float64
			err = tx.QueryRow(ctx, `SELECT COALESCE(sum(CASE WHEN metadata->>'FileSizeBytes' ~ '^[0-9]+$' THEN (metadata->>'FileSizeBytes')::numeric ELSE COALESCE(NULLIF(metadata->>'FileSize',''),'0')::numeric*1048576 END),0) FROM image_files WHERE metadata->>'ChannelName'=$1`, channel.Name).Scan(&used)
			threshold := channel.Quota.Threshold
			if threshold <= 0 {
				threshold = 95
			}
			if err != nil || used+float64(size) > channel.Quota.LimitGB*1024*1024*1024*threshold/100 {
				tx.Rollback(context.WithoutCancel(ctx))
				if err != nil {
					return nil, err
				}
				return nil, ErrConflict
			}
		}
		var store objectStore = m.local
		metadata["Channel"] = "CloudflareR2"
		metadata["ChannelName"] = channel.Name
		if channel.Endpoint != "" {
			store, err = m.s3For(channel)
			metadata["Channel"] = "S3"
		}
		if err != nil {
			tx.Rollback(context.WithoutCancel(ctx))
			continue
		}
		var id string
		reserved := false
		for n := 0; n < 1000; n++ {
			candidate := base
			if n > 0 {
				candidate = strings.TrimSuffix(base, path.Ext(base)) + "(" + strconv.Itoa(n) + ")" + path.Ext(base)
			}
			id = path.Join(folder, candidate)
			if !validKey(id) {
				tx.Rollback(context.WithoutCancel(ctx))
				return nil, ErrInvalid
			}
			if metadata["Channel"] == "S3" {
				metadata["S3FileKey"] = id
			} else {
				delete(metadata, "S3FileKey")
			}
			raw, e := json.Marshal(metadata)
			if e != nil {
				tx.Rollback(context.WithoutCancel(ctx))
				return nil, e
			}
			tag, e := tx.Exec(ctx, `INSERT INTO image_files(id,metadata,state) VALUES($1,$2,'pending') ON CONFLICT(id) DO NOTHING`, id, raw)
			if e != nil {
				tx.Rollback(context.WithoutCancel(ctx))
				return nil, e
			}
			if tag.RowsAffected() == 1 {
				reserved = true
				break
			}
		}
		if !reserved {
			tx.Rollback(context.WithoutCancel(ctx))
			return nil, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		if _, err = body.Seek(0, 0); err != nil {
			return nil, err
		}
		err = store.Put(ctx, id, body, size, contentType)
		if err != nil {
			// A remote timeout can occur after S3 committed the object. Keep the
			// reservation for reconciliation instead of creating a duplicate elsewhere.
			if _, remote := store.(*s3Store); remote {
				return nil, err
			}
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_, cleanupErr := m.pool.Exec(cleanup, `DELETE FROM image_files WHERE id=$1 AND state='pending'`, id)
			cancel()
			if cleanupErr != nil {
				return nil, cleanupErr
			}
			continue
		}
		// After bytes exist, preserve a pending row if finalization fails, so an
		// operator can reconcile it. Never retry another channel after this point.
		// The bytes are stored: a client disconnect must not strand the row.
		finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, err = m.pool.Exec(finalize, `UPDATE image_files SET state='ready',updated_at=now() WHERE id=$1 AND state='pending'`, id)
		cancel()
		if err != nil {
			return nil, err
		}
		return m.uploadResult(id, q), nil
	}
	return nil, errors.New("image upload failed on selected stores")
}

func (m *Module) uploadResult(id string, q url.Values) []map[string]string {
	link := "/file/" + id
	if q.Get("returnFormat") == "full" {
		link = fileURL(m.cfg.PublicURL, id)
	}
	return []map[string]string{{"src": link}}
}

// Save stores an image for another module in this process, which has already
// authenticated its user, and returns the public file URL.
func (m *Module) Save(ctx context.Context, category, entityID, fileName, contentType string, data []byte) (string, error) {
	folder, err := categoryFolder(category, "", entityID)
	if err != nil {
		return "", err
	}
	q := url.Values{"uploadFolder": {folder}, "returnFormat": {"full"}}
	result, err := m.saveUpload(ctx, fileName, contentType, bytes.NewReader(data), int64(len(data)), q, "")
	if err != nil {
		m.deps.Logger.Error("image operation failed", "error", safeError(err))
		return "", err
	}
	return result[0]["src"], nil
}
