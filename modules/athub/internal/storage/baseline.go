package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type baselineKey struct{}

// WithBaseline makes Mutate apply a change that was rehearsed earlier, given
// the baseline that rehearsal produced. Only what the change altered then is
// written, everything else keeps what it holds now, and the change is refused
// with ErrStale when what it altered has been changed since. An empty baseline
// leaves ctx as it is, so the change is applied in full.
func WithBaseline(ctx context.Context, baseline []byte) context.Context {
	if trimmed := bytes.TrimSpace(baseline); len(trimmed) == 0 || string(trimmed) == "null" {
		return ctx
	}
	return context.WithValue(ctx, baselineKey{}, baseline)
}

// A baseline written under another version cannot be compared the way this
// code compares, so it is an error rather than a guess.
const baselineVersion = 1

// A baseline lists the records a rehearsed change altered, among those that
// existed before it. The host stores it and hands it back unread.
type baselineDocument struct {
	Version int                       `json:"v"`
	Novels  map[int32][]baselineEntry `json:"novels,omitempty"`
	Shared  []baselineEntry           `json:"shared,omitempty"`
}

// Later values are not kept: they are never compared, and the IDs a rehearsal
// gave to what it created are not the ones the applied change will get.
type baselineEntry struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	// The row an update altered. A record removed and created again under the
	// same key is another record.
	ID any `json:"id,omitempty"`
	// What the fields an update altered held before, or the whole removed
	// record. Absent for a created record.
	Before map[string]any `json:"before,omitempty"`
}

// baselineRecords holds baseline entries by natural key.
type baselineRecords map[string][]baselineEntry

type baseline struct {
	novels map[int32]baselineRecords
	shared baselineRecords
}

// These fields of a novel repeat names held by other records. They change when
// an author, a platform or a tag is renamed, which is no change to the novel.
var reflectedFields = map[string]map[string]bool{
	"Novel": {"authorName": true, "platformName": true, "tagNames": true},
}

// naturalKey names a changed record by what a submitter knows it by. Rows a
// rehearsal created were rolled back, so their IDs differ from the ones the
// same change creates when it is applied.
func naturalKey(change Change) string {
	record := change.Before
	if record == nil {
		record = change.After
	}
	switch change.Entity {
	case "Novel", "IllustrationLayout":
		return change.Entity
	case "Volume", "Chapter":
		return change.Entity + ":" + keyText(record["order"])
	case "Illustration":
		return change.Entity + ":" + keyText(record["sha256"])
	}
	return change.Entity + ":" + keyText(record["id"])
}

func keyText(value any) string {
	if number, ok := value.(float64); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}

func orderKey(entity string, order int32) string {
	return entity + ":" + strconv.Itoa(int(order))
}

// alteredFields returns what the fields an update changed held before, leaving
// out the fields that repeat other records.
func alteredFields(change Change) map[string]any {
	fields := make(map[string]any)
	for name, value := range change.Before {
		if reflectedFields[change.Entity][name] {
			continue
		}
		if next, ok := change.After[name]; !ok || !reflect.DeepEqual(value, next) {
			fields[name] = value
		}
	}
	for name := range change.After {
		if _, ok := change.Before[name]; !ok && !reflectedFields[change.Entity][name] {
			fields[name] = nil
		}
	}
	return fields
}

func baselineEntries(changes []Change) []baselineEntry {
	var entries []baselineEntry
	for _, change := range changes {
		entry := baselineEntry{Key: naturalKey(change), Action: change.Action}
		switch change.Action {
		case "UPDATE":
			entry.ID, entry.Before = change.Before["id"], alteredFields(change)
			if len(entry.Before) == 0 {
				continue
			}
		case "DELETE":
			entry.Before = make(map[string]any, len(change.Before))
			for name, value := range change.Before {
				if !reflectedFields[change.Entity][name] {
					entry.Before[name] = value
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// encodeBaseline describes what a rehearsal did to the novels that existed
// before it and to the shared records. A rehearsal that altered nothing still
// gets a baseline: applied with it, the change may alter nothing either.
func encodeBaseline(novels map[int32][]Change, shared []Change) ([]byte, error) {
	document := baselineDocument{Version: baselineVersion, Shared: baselineEntries(shared)}
	for id, changes := range novels {
		if entries := baselineEntries(changes); len(entries) > 0 {
			if document.Novels == nil {
				document.Novels = make(map[int32][]baselineEntry)
			}
			document.Novels[id] = entries
		}
	}
	return json.Marshal(document)
}

// decodeBaseline reads values the way snapshot does, so that what a baseline
// kept compares equal to the same record read again, however the host's
// database has rewritten the document in between.
func decodeBaseline(raw []byte) (*baseline, error) {
	var document baselineDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("content baseline: %w", err)
	}
	if document.Version != baselineVersion {
		return nil, fmt.Errorf("content baseline has unsupported version %d", document.Version)
	}
	index := func(entries []baselineEntry) baselineRecords {
		records := make(baselineRecords, len(entries))
		for _, entry := range entries {
			records[entry.Key] = append(records[entry.Key], entry)
		}
		return records
	}
	result := &baseline{novels: make(map[int32]baselineRecords, len(document.Novels)), shared: index(document.Shared)}
	for id, entries := range document.Novels {
		result.novels[id] = index(entries)
	}
	return result, nil
}

// permits reports whether a change may be applied under this baseline, given
// what it did to the novels that existed before it and to the shared records.
// A novel the baseline does not list was left alone by the rehearsal.
func (b *baseline) permits(novels map[int32][]Change, shared []Change) bool {
	if !b.shared.permit(shared) {
		return false
	}
	for id, changes := range novels {
		if !b.novels[id].permit(changes) {
			return false
		}
	}
	return true
}

// permit reports whether each change is one the rehearsal made as well, to a
// record that still holds what the rehearsal found in the fields it alters. A
// change of the rehearsal that is not made now is no objection: someone made
// it in the meantime, or what it removed is already gone.
func (r baselineRecords) permit(changes []Change) bool {
	for _, change := range changes {
		var altered map[string]any
		if change.Action == "UPDATE" {
			if altered = alteredFields(change); len(altered) == 0 {
				continue
			}
		}
		var entry *baselineEntry
		for i := range r[naturalKey(change)] {
			if candidate := &r[naturalKey(change)][i]; candidate.Action == change.Action {
				entry = candidate
				break
			}
		}
		if entry == nil {
			return false
		}
		switch change.Action {
		case "UPDATE":
			if !reflect.DeepEqual(change.Before["id"], entry.ID) {
				return false
			}
			for name, value := range altered {
				if submitted, ok := entry.Before[name]; !ok || !reflect.DeepEqual(value, submitted) {
					return false
				}
			}
		case "DELETE":
			// A field the baseline does not know was added to the records later.
			for name, submitted := range entry.Before {
				if value, ok := change.Before[name]; !ok || !reflect.DeepEqual(value, submitted) {
					return false
				}
			}
		}
	}
	return true
}

// altered returns the fields of a record that the rehearsed change altered.
// whole is true when the rehearsal created or removed the record under this
// key, so that no part of it was left alone.
func (r baselineRecords) altered(key string) (fields map[string]any, whole bool) {
	entries := r[key]
	if len(entries) == 0 {
		return nil, false
	}
	if len(entries) > 1 || entries[0].Action != "UPDATE" {
		return nil, true
	}
	return entries[0].Before, false
}

// keptChapter tells which parts of a chapter a rehearsed change left alone.
type keptChapter struct{ title, volume, body bool }

// ok is false when the rehearsal created or removed the chapter.
func (r baselineRecords) keptChapter(order int32) (kept keptChapter, ok bool) {
	altered, whole := r.altered(orderKey("Chapter", order))
	if whole {
		return kept, false
	}
	_, title := altered["title"]
	_, volume := altered["volumeId"]
	_, hash := altered["contentHash"]
	_, length := altered["contentLength"]
	return keptChapter{title: !title, volume: !volume, body: !hash && !length}, true
}

// keptVolume reports whether the rehearsal left the title of a volume alone.
func (r baselineRecords) keptVolume(order int32) bool {
	altered, whole := r.altered(orderKey("Volume", order))
	_, title := altered["title"]
	return !whole && !title
}

// keepNovel gives the fields of a novel that a rehearsed edit left alone what
// the novel holds now, whatever the edit carries for them.
func keepNovel(n *NovelInput, current NovelInput, altered map[string]any) {
	if _, ok := altered["platformId"]; !ok {
		n.PlatformID = current.PlatformID
	}
	if _, ok := altered["authorId"]; !ok {
		// A name would be looked up again, and created when it is missing.
		n.AuthorID, n.AuthorName = current.AuthorID, nil
	}
	if _, ok := altered["title"]; !ok {
		n.Title = current.Title
	}
	if _, ok := altered["summary"]; !ok {
		n.Summary = current.Summary
	}
	if _, ok := altered["coverUrl"]; !ok {
		n.CoverURL = current.CoverURL
	}
	if _, ok := altered["status"]; !ok {
		n.Status = current.Status
	}
	if _, ok := altered["tagIds"]; !ok {
		n.TagIDs, n.TagNames = current.TagIDs, nil
	}
}

// submitted returns what the rehearsed change that is being applied did to a
// novel. ok is false when the change is written as it was given: outside the
// application of a rehearsed change, and for a novel the change itself creates.
func submitted(ctx context.Context, novelID int32) (records baselineRecords, ok bool) {
	m, tracked := ctx.Value(mutationKey{}).(*mutation)
	if !tracked || m.baseline == nil || m.created[novelID] {
		return nil, false
	}
	return m.baseline.novels[novelID], true
}

// keepChapter gives the title and the volume of a stored chapter to an edit
// whose rehearsal left them alone. It reports whether the rehearsal left the
// body alone as well; the body is then not to be written.
func (p *Postgres) keepChapter(ctx context.Context, c *ChapterInput) (keepBody bool, err error) {
	records, ok := submitted(ctx, c.NovelID)
	if !ok {
		return false, nil
	}
	kept, ok := records.keptChapter(c.Order)
	if !ok {
		return false, nil
	}
	var title string
	var volumeID *int32
	err = p.db(ctx).QueryRow(ctx, `SELECT "title", "volumeId" FROM "Chapter" WHERE "novelId" = $1 AND "order" = $2`,
		c.NovelID, c.Order).Scan(&title, &volumeID)
	if errors.Is(err, pgx.ErrNoRows) {
		// A chapter that is gone is written as it was submitted, and the
		// comparison with the baseline decides about it.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kept.title {
		c.Title = title
	}
	if kept.volume {
		c.VolumeID = volumeID
	}
	return kept.body, nil
}

// keepVolumeTitle gives the title of a stored volume to an edit whose
// rehearsal left it alone.
func (p *Postgres) keepVolumeTitle(ctx context.Context, v *VolumeInput) error {
	records, ok := submitted(ctx, v.NovelID)
	if !ok || !records.keptVolume(v.Order) {
		return nil
	}
	var title string
	err := p.db(ctx).QueryRow(ctx, `SELECT "title" FROM "Volume" WHERE "novelId" = $1 AND "order" = $2`,
		v.NovelID, v.Order).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err == nil {
		v.Title = title
	}
	return err
}

// A shared record is a catalog row that novels refer to. Renaming or removing
// it changes no novel, so history never lists it, but a submitted rename or
// removal is compared with its baseline like any other change.
type sharedRecord struct {
	entity string
	id     int32
	before map[string]any
}

// trackShared remembers a shared record before it is changed. It does nothing
// outside a rehearsal and the application of a rehearsed change.
func (p *Postgres) trackShared(ctx context.Context, entity string, id int32) error {
	m, ok := ctx.Value(mutationKey{}).(*mutation)
	if !ok || m.shared == nil {
		return nil
	}
	key := orderKey(entity, id)
	if _, tracked := m.shared[key]; tracked {
		return nil
	}
	// The lock keeps the record as it was read until the change has ended. It
	// is the weakest one that shuts other changes to the record out, so that
	// reading it does not hold up novels that merely start to refer to it.
	before, err := readShared(ctx, m.tx, entity, id, ` FOR NO KEY UPDATE`)
	if err != nil || before == nil {
		// The change itself reports a record that does not exist.
		return err
	}
	m.shared[key] = sharedRecord{entity: entity, id: id, before: before}
	return nil
}

func readShared(ctx context.Context, q database, entity string, id int32, lock string) (map[string]any, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT to_jsonb(r) FROM `+pgx.Identifier{entity}.Sanitize()+` r WHERE r."id"=$1`+lock, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record map[string]any
	return record, json.Unmarshal(raw, &record)
}

func (m *mutation) sharedChanges(ctx context.Context) ([]Change, error) {
	before := make(map[string]map[string]any, len(m.shared))
	after := make(map[string]map[string]any, len(m.shared))
	for key, record := range m.shared {
		before[key] = record.before
		current, err := readShared(ctx, m.tx, record.entity, record.id, "")
		if err != nil {
			return nil, err
		}
		if current != nil {
			after[key] = current
		}
	}
	return diffSnapshot(before, after), nil
}
