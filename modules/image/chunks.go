package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type chunkSession struct {
	Name        string              `json:"originalFileName"`
	Type        string              `json:"originalFileType"`
	Total       int                 `json:"totalChunks"`
	Channel     string              `json:"uploadChannel"`
	ChannelName string              `json:"channelName"`
	Folder      string              `json:"uploadFolder"`
	NameType    string              `json:"uploadNameType"`
	Result      []map[string]string `json:"result,omitempty"`
}

func (m *Module) initChunks(w http.ResponseWriter, r *http.Request, f *parsedForm) {
	total, err := strconv.Atoi(f.Values["totalChunks"])
	if err != nil || total < 1 || total > m.cfg.MaxChunks || f.Values["originalFileName"] == "" || f.Values["originalFileType"] == "" || f.File != nil {
		m.failure(w, ErrInvalid)
		return
	}
	channels, err := m.chooseChannels(r.Context(), r.URL.Query())
	if err != nil {
		m.failure(w, err)
		return
	}
	channel := "cfr2"
	if channels[0].Endpoint != "" {
		channel = "s3"
	}
	id := "upload_" + uuid.NewString()
	session := chunkSession{Name: f.Values["originalFileName"], Type: f.Values["originalFileType"], Total: total, Channel: channel, ChannelName: channels[0].Name, Folder: sanitizeFolder(r.URL.Query().Get("uploadFolder")), NameType: r.URL.Query().Get("uploadNameType")}
	raw, err := json.Marshal(session)
	if err != nil {
		m.failure(w, err)
		return
	}
	expires := time.Now().Add(m.cfg.UploadTTL)
	who := r.Context().Value(actorKey{}).(actor)
	// Each unfinished session may stage one maximum-sized file, so bound how
	// many an owner can hold open. Merged sessions keep only their receipt.
	tx, err := m.pool.Begin(r.Context())
	if err != nil {
		m.failure(w, err)
		return
	}
	defer tx.Rollback(context.WithoutCancel(r.Context()))
	// Sessions started together would each count the others as absent. The
	// owner lock makes them take turns; the count below is a later statement,
	// so it sees what the previous holder committed.
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "image_uploads:"+who.ID); err != nil {
		m.failure(w, err)
		return
	}
	tag, err := tx.Exec(r.Context(), `INSERT INTO image_uploads(id,owner,data,expires_at) SELECT $1,$2,$3,$4 WHERE (SELECT count(*) FROM image_uploads WHERE owner=$2 AND expires_at>now() AND NOT data ? 'result')<$5`, id, who.ID, raw, expires, m.cfg.MaxSessionsPerOwner)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		m.failure(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, 429, "too_many_upload_sessions")
		return
	}
	respond(w, 200, map[string]any{"success": true, "uploadId": id, "session": map[string]any{"uploadId": id, "totalChunks": total, "uploadChannel": channel, "channelName": session.ChannelName, "expiresAt": expires.UnixMilli()}})
}

func (m *Module) lockedUpload(ctx context.Context, id string) (pgx.Tx, chunkSession, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, chunkSession{}, err
	}
	var owner string
	var raw []byte
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT owner,data,expires_at FROM image_uploads WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &raw, &expires)
	who, _ := ctx.Value(actorKey{}).(actor)
	if err == nil && (owner != who.ID && !who.Admin || !expires.After(time.Now()) && ctx.Value(chunkCleanupKey{}) != true) {
		err = ErrNotFound
	}
	var session chunkSession
	if err == nil {
		err = json.Unmarshal(raw, &session)
	}
	if err != nil {
		tx.Rollback(context.WithoutCancel(ctx))
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return nil, session, err
	}
	return tx, session, nil
}

func (m *Module) chunk(w http.ResponseWriter, r *http.Request, f *parsedForm) {
	if err := m.acquireChunkSlot(r.Context()); err != nil {
		m.failure(w, err)
		return
	}
	defer func() { <-m.chunkSlots }()
	id := f.Values["uploadId"]
	tx, session, err := m.lockedUpload(r.Context(), id)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer tx.Rollback(context.WithoutCancel(r.Context()))
	total, e := strconv.Atoi(f.Values["totalChunks"])
	if e != nil || total != session.Total || f.Values["originalFileName"] != session.Name || f.Values["originalFileType"] != session.Type {
		m.failure(w, ErrInvalid)
		return
	}
	q := r.URL.Query()
	if q.Has("uploadChannel") && q.Get("uploadChannel") != session.Channel || q.Has("channelName") && q.Get("channelName") != session.ChannelName || q.Has("uploadFolder") && sanitizeFolder(q.Get("uploadFolder")) != session.Folder {
		m.failure(w, ErrInvalid)
		return
	}
	if len(session.Result) > 0 {
		respond(w, 200, session.Result)
		return
	}
	if q.Get("merge") == "true" {
		var count int
		var size int64
		if err = tx.QueryRow(r.Context(), `SELECT count(*),COALESCE(sum(size),0) FROM image_upload_parts WHERE upload_id=$1`, id).Scan(&count, &size); err != nil {
			m.failure(w, err)
			return
		}
		if count != session.Total {
			respondError(w, 409, "upload_parts_incomplete")
			return
		}
		if size > m.cfg.MaxUploadBytes {
			respondError(w, 413, "upload_too_large")
			return
		}
		mergedName := "merge-" + uuid.NewString()
		merged, err := m.scratch.OpenFile(mergedName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			m.failure(w, err)
			return
		}
		defer merged.Close()
		defer m.scratch.Remove(mergedName)
		for n := 0; n < session.Total; n++ {
			var expected string
			var partSize int64
			if err = tx.QueryRow(r.Context(), `SELECT size,digest FROM image_upload_parts WHERE upload_id=$1 AND number=$2`, id, n).Scan(&partSize, &expected); err != nil {
				m.failure(w, err)
				return
			}
			part, err := m.scratch.Open(chunkPartName(id, n))
			if err != nil {
				m.failure(w, err)
				return
			}
			hash := sha256.New()
			copied, err := io.Copy(io.MultiWriter(merged, hash), io.LimitReader(part, partSize+1))
			part.Close()
			if err != nil || copied != partSize || hex.EncodeToString(hash.Sum(nil)) != expected {
				respondError(w, 409, "upload_part_corrupted")
				return
			}
		}
		q.Set("uploadChannel", session.Channel)
		q.Set("channelName", session.ChannelName)
		q.Set("uploadFolder", session.Folder)
		q.Set("uploadNameType", session.NameType)
		q.Set("autoRetry", "false")
		result, err := m.saveUpload(context.WithValue(r.Context(), uploadReceiptKey{}, id), session.Name, session.Type, merged, size, q, "")
		if err != nil {
			m.failure(w, err)
			return
		}
		session.Result = result
		raw, _ := json.Marshal(session)
		_, err = tx.Exec(r.Context(), `UPDATE image_uploads SET data=$2 WHERE id=$1`, id, raw)
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			m.failure(w, err)
			return
		}
		for n := 0; n < session.Total; n++ {
			_ = m.scratch.Remove(chunkPartName(id, n))
		}
		respond(w, 200, result)
		return
	}
	n, e := strconv.Atoi(f.Values["chunkIndex"])
	if e != nil || n < 0 || n >= session.Total || f.File == nil {
		m.failure(w, ErrInvalid)
		return
	}
	hash := sha256.New()
	if _, err = f.File.Seek(0, 0); err == nil {
		_, err = io.Copy(hash, f.File)
	}
	if err != nil {
		m.failure(w, err)
		return
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	var previous string
	err = tx.QueryRow(r.Context(), `SELECT digest FROM image_upload_parts WHERE upload_id=$1 AND number=$2`, id, n).Scan(&previous)
	if err == nil {
		if previous != digest {
			m.failure(w, ErrConflict)
			return
		}
		respond(w, 200, map[string]any{"success": true, "uploadId": id, "chunkIndex": n})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		m.failure(w, err)
		return
	}
	// Reject oversized uploads while parts arrive, not only at merge time, so a
	// session can never stage more than one maximum-sized file on disk.
	var staged int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(size),0) FROM image_upload_parts WHERE upload_id=$1`, id).Scan(&staged); err != nil {
		m.failure(w, err)
		return
	}
	if staged+f.Size > m.cfg.MaxUploadBytes {
		respondError(w, 413, "upload_too_large")
		return
	}
	target := chunkPartName(id, n)
	if _, err = f.File.Seek(0, 0); err != nil {
		m.failure(w, err)
		return
	}
	// The session row lock is held and no part row exists, so a file left here
	// by an interrupted request is unreferenced and safe to replace.
	part, err := m.scratch.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		m.failure(w, err)
		return
	}
	_, err = io.Copy(part, f.File)
	part.Close()
	if err != nil {
		m.scratch.Remove(target)
		m.failure(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO image_upload_parts(upload_id,number,size,digest) VALUES($1,$2,$3,$4)`, id, n, f.Size, digest)
	if err != nil {
		m.scratch.Remove(target)
		m.failure(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		// The part may be committed even though the reply was lost, so its file
		// stays. If it was not, a retry replaces it and the janitor removes it
		// with the session.
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"success": true, "uploadId": id, "chunkIndex": n})
}

func chunkPartName(id string, n int) string { return fmt.Sprintf("part-%s-%06d", id, n) }

type chunkCleanupKey struct{}

func (m *Module) cleanupChunks(w http.ResponseWriter, r *http.Request) {
	if err := m.acquireChunkSlot(r.Context()); err != nil {
		m.failure(w, err)
		return
	}
	defer func() { <-m.chunkSlots }()
	id := r.URL.Query().Get("uploadId")
	tx, session, err := m.lockedUpload(context.WithValue(r.Context(), chunkCleanupKey{}, true), id)
	if err != nil {
		m.failure(w, err)
		return
	}
	defer tx.Rollback(context.WithoutCancel(r.Context()))
	for n := 0; n < session.Total; n++ {
		if err = m.scratch.Remove(chunkPartName(id, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			m.failure(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM image_uploads WHERE id=$1`, id)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		m.failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"success": true, "uploadId": id})
}

func (m *Module) acquireChunkSlot(ctx context.Context) error {
	select {
	case m.chunkSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
