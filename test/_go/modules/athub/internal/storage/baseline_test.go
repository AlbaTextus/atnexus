package storage

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
)

// What the approval of a change may do, given what its rehearsal did: only what
// the rehearsal did as well, to the records it did it to. What a record holds
// by now is no objection to updating it, and is one to removing it.
func TestBaselinePermits(t *testing.T) {
	chapter := func(id float64, title, hash string) map[string]any {
		return map[string]any{"id": id, "novelId": 1.0, "volumeId": nil, "order": 2.0, "title": title, "contentHash": hash, "contentLength": 9.0}
	}
	novel := func(title, authorName string) map[string]any {
		return map[string]any{"id": 1.0, "title": title, "tagIds": []any{3.0}, "authorName": authorName, "platformName": "Platform", "tagNames": []any{"tag"}}
	}
	// A layout has no ID of its own: a novel has one layout or none.
	layout := func(images ...float64) map[string]any {
		blocks := make([]any, 0, len(images))
		for _, image := range images {
			blocks = append(blocks, map[string]any{"imageId": image, "x": 0.0, "y": 0.0, "width": 400.0, "height": 300.0})
		}
		return map[string]any{"novelId": 1.0, "blocks": blocks}
	}
	update := func(entity string, before, after map[string]any) []Change {
		return []Change{{Entity: entity, Action: "UPDATE", Before: before, After: after}}
	}
	insert := func(entity string, record map[string]any) []Change {
		return []Change{{Entity: entity, Action: "INSERT", After: record}}
	}
	remove := func(entity string, record map[string]any) []Change {
		return []Change{{Entity: entity, Action: "DELETE", Before: record}}
	}
	with := func(record map[string]any, field string, value any) map[string]any {
		record = maps.Clone(record)
		record[field] = value
		return record
	}
	without := func(record map[string]any, field string) map[string]any {
		record = maps.Clone(record)
		delete(record, field)
		return record
	}
	stored := chapter(7, "Title", "hash")
	retitled := update("Chapter", stored, chapter(7, "New title", "hash"))
	created, removed := insert("Chapter", stored), remove("Chapter", stored)
	// A body is known by its hash and its length. Rewritten into a body of the
	// same length it alters the hash alone, and into a longer one both.
	rewritten := update("Chapter", stored, chapter(7, "Title", "new hash"))
	extended := update("Chapter", stored, with(chapter(7, "Title", "new hash"), "contentLength", 12.0))
	// What someone else made of the body in the meantime.
	theirs := with(chapter(7, "Title", "their hash"), "contentLength", 12.0)
	tests := map[string]struct {
		rehearsed, approved []Change
		want                bool
	}{
		"the same update":                       {retitled, retitled, true},
		"nothing left to do":                    {retitled, nil, true},
		"update of a field that was left alone": {retitled, update("Chapter", stored, chapter(7, "New title", "other hash")), false},
		// Someone else retitled the chapter first: the change applied later stands.
		"update over another value": {retitled, update("Chapter", chapter(7, "Their title", "hash"), chapter(7, "New title", "hash")), true},
		// Chapter 8 took the place of chapter 7 and reads the same.
		"update of another row in the same place": {retitled, update("Chapter", chapter(8, "Title", "hash"), chapter(8, "New title", "hash")), false},
		"update that was not rehearsed":           {nil, retitled, false},
		"update where a creation was rehearsed":   {created, retitled, false},
		// Written over a body of another length, the same body alters the length
		// too, which its rehearsal never did.
		"new body over a body of another length": {rewritten, update("Chapter", theirs, chapter(7, "Title", "new hash")), true},
		// And a longer body alters the hash alone once the body is that long.
		"longer body over a body of that length": {extended, update("Chapter", theirs, with(chapter(7, "Title", "new hash"), "contentLength", 12.0)), true},
		"new title where a body was rehearsed":   {rewritten, retitled, false},
		"new body where a title was rehearsed":   {retitled, rewritten, false},
		"renamed author alone":                   {nil, update("Novel", novel("Title", "Author"), novel("Title", "Renamed author")), true},
		"renamed author next to the same update": {
			update("Novel", novel("Title", "Author"), novel("New title", "Author")),
			update("Novel", novel("Title", "Author"), novel("New title", "Renamed author")), true},
		// The row a rehearsal creates is rolled back, so the next one has another ID.
		"creation that was rehearsed":            {created, insert("Chapter", chapter(9, "Title", "hash")), true},
		"creation that was not rehearsed":        {nil, created, false},
		"creation where an update was rehearsed": {retitled, insert("Chapter", chapter(9, "New title", "hash")), false},
		// The novel had no layout when one was submitted, and has one by now.
		"first layout over one stored since":        {insert("IllustrationLayout", layout(1)), update("IllustrationLayout", layout(2), layout(1)), true},
		"layout over another layout":                {update("IllustrationLayout", layout(3), layout(1)), update("IllustrationLayout", layout(2), layout(1)), true},
		"layout that was not rehearsed":             {nil, update("IllustrationLayout", layout(2), layout(1)), false},
		"the same removal":                          {removed, removed, true},
		"nothing left to remove":                    {removed, nil, true},
		"removal of a record that was changed":      {removed, remove("Chapter", chapter(7, "Their title", "hash")), false},
		"removal of another row in the same place":  {removed, remove("Chapter", chapter(8, "Title", "hash")), false},
		"removal of a record with a later field":    {removed, remove("Chapter", with(stored, "later", 4.0)), true},
		"removal of a record without a known field": {removed, remove("Chapter", without(stored, "contentLength")), false},
		"removal of a novel with a renamed author":  {remove("Novel", novel("Title", "Author")), remove("Novel", novel("Title", "Renamed author")), true},
		"removal that was not rehearsed":            {nil, removed, false},
		"removal where an update was rehearsed":     {retitled, removed, false},
	}
	// read gives a baseline the way an approval gets it: written and read again.
	read := func(t *testing.T, novels map[int32][]Change, shared []Change) *baseline {
		t.Helper()
		raw, err := encodeBaseline(novels, shared)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeBaseline(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return decoded
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := read(t, map[int32][]Change{1: test.rehearsed}, nil).permits(map[int32][]Change{1: test.approved}, nil); got != test.want {
				t.Errorf("in a novel: permitted %v, want %v", got, test.want)
			}
			// Shared records follow the same rule.
			if got := read(t, nil, test.rehearsed).permits(nil, test.approved); got != test.want {
				t.Errorf("among shared records: permitted %v, want %v", got, test.want)
			}
		})
	}

	// A novel the rehearsal did not touch may not be touched now either.
	elsewhere := read(t, map[int32][]Change{1: retitled}, nil)
	if elsewhere.permits(map[int32][]Change{2: retitled}, nil) {
		t.Error("change to a novel the rehearsal left alone was permitted")
	}
	if !elsewhere.permits(map[int32][]Change{2: nil}, nil) {
		t.Error("novel that is left alone was not permitted")
	}
	if elsewhere.permits(nil, retitled) {
		t.Error("change to a shared record the rehearsal left alone was permitted")
	}
}

func TestBaselineDocuments(t *testing.T) {
	for _, raw := range []string{`{"v":1,"novels":`, `[]`, `"baseline"`, `{}`, `{"v":2}`, `{"v":1,"novels":{"first":[]}}`} {
		if _, err := decodeBaseline([]byte(raw)); err == nil || errors.Is(err, ErrStale) {
			t.Errorf("baseline %s: %v", raw, err)
		}
	}
	// Without a baseline there is nothing to compare with, and the change is
	// applied in full.
	for _, raw := range []string{"", " \n", "null", " null\n"} {
		if WithBaseline(context.Background(), []byte(raw)).Value(baselineKey{}) != nil {
			t.Errorf("baseline %q was attached", raw)
		}
	}
	// A change that altered nothing has a baseline all the same.
	raw, err := encodeBaseline(map[int32][]Change{1: nil}, nil)
	if err != nil || string(raw) != `{"v":1}` {
		t.Fatalf("empty baseline %s: %v", raw, err)
	}
	if WithBaseline(context.Background(), raw).Value(baselineKey{}) == nil {
		t.Error("empty baseline was not attached")
	}
}

// A baseline keeps what a change makes of the records it alters, next to what
// they held before, so that a queued change can be shown to whoever edits the
// same novel. SubmittedChanges is that view of it.
func TestSubmittedChanges(t *testing.T) {
	novel := map[string]any{"id": 17.0, "platformId": 1.0, "authorId": 4.0, "title": "Title", "summary": "Summary", "coverUrl": nil, "status": "ONGOING",
		"tagIds": []any{3.0}, "authorName": "Author", "platformName": "Platform", "tagNames": []any{"tag"}}
	edited := maps.Clone(novel)
	edited["title"], edited["summary"] = "New title", nil
	edited["tagIds"], edited["tagNames"] = []any{3.0, 5.0}, []any{"tag", "another tag"}
	// The platform was renamed while the change ran, which is no change to the novel.
	edited["platformName"] = "Platform, renamed"
	chapter := map[string]any{"id": 9.0, "novelId": 17.0, "volumeId": nil, "order": 2.0, "title": "Chapter", "contentHash": "hash", "contentLength": 9.0}
	image := map[string]any{"id": 41.0, "novelId": 17.0, "sha256": "abc", "url": "https://images.example.test/a.webp", "width": 800.0, "height": 600.0}
	block := map[string]any{"imageId": 41.0, "x": 0.0, "y": 0.0, "width": 400.0, "height": 300.0}
	other := map[string]any{"id": 18.0, "title": "Another novel", "tagIds": []any{}, "authorName": nil, "platformName": "Platform", "tagNames": []any{}}
	raw, err := encodeBaseline(map[int32][]Change{
		17: {
			{Entity: "Novel", Action: "UPDATE", Before: novel, After: edited},
			{Entity: "Chapter", Action: "INSERT", After: chapter},
			{Entity: "Illustration", Action: "DELETE", Before: image},
			{Entity: "IllustrationLayout", Action: "UPDATE", Before: map[string]any{"novelId": 17.0, "blocks": []any{block}}, After: map[string]any{"novelId": 17.0, "blocks": []any{}}},
		},
		18: {{Entity: "Novel", Action: "UPDATE", Before: other, After: maps.Clone(other)}},
		// Nothing but a name this novel repeats has changed.
		19: {{Entity: "Novel", Action: "UPDATE", Before: novel, After: func() map[string]any {
			renamed := maps.Clone(novel)
			renamed["authorName"] = "Author, renamed"
			return renamed
		}()}},
	}, []Change{{Entity: "Tag", Action: "UPDATE", Before: map[string]any{"id": 3.0, "name": "tag"}, After: map[string]any{"id": 3.0, "name": "tag, renamed"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := SubmittedChanges(raw, 17)
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	want := []SubmittedChange{
		// A field that refers to other records comes with the names it will show,
		// and a cleared field is there, holding nothing.
		{Entity: "Novel", Action: "UPDATE", ID: 17.0, Fields: []string{"summary", "tagIds", "title"},
			After: map[string]any{"title": "New title", "summary": nil, "tagIds": []any{3.0, 5.0}, "tagNames": []any{"tag", "another tag"}}},
		// The ID a rehearsal gave to what it created is not the one it will get.
		{Entity: "Chapter", Action: "INSERT", After: map[string]any{"novelId": 17.0, "volumeId": nil, "order": 2.0, "title": "Chapter", "contentHash": "hash", "contentLength": 9.0}},
		{Entity: "Illustration", Action: "DELETE", ID: 41.0},
		{Entity: "IllustrationLayout", Action: "UPDATE", Fields: []string{"blocks"}, After: map[string]any{"blocks": []any{}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes to novel 17: %+v, baseline %s", got, raw)
	}
	// A novel the change leaves as it is has an empty list, which is served as
	// [] and not as null. So has one of which only a repeated name changed.
	for _, untouched := range []int32{18, 19, 20} {
		if got, err := SubmittedChanges(raw, untouched); err != nil || got == nil || len(got) != 0 {
			t.Fatalf("changes to novel %d: %+v %v", untouched, got, err)
		}
	}
	// What the change does to shared records is no change to any novel.
	if encoded, err := json.Marshal(got); err != nil || strings.Contains(string(encoded), "renamed") || strings.Contains(string(encoded), "before") {
		t.Fatalf("served %s: %v", encoded, err)
	}

	// A change that was queued before later values were kept names the fields
	// it alters and cannot tell what they become.
	got, err = SubmittedChanges([]byte(`{"v":1,"novels":{"17":[
		{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"Title","coverUrl":null}},
		{"key":"Volume:3","action":"INSERT"},
		{"key":"Chapter:2","action":"DELETE","before":{"id":9,"order":2,"title":"Chapter"}}
	]}}`), 17)
	want = []SubmittedChange{
		{Entity: "Novel", Action: "UPDATE", ID: 17.0, Fields: []string{"coverUrl", "title"}},
		{Entity: "Volume", Action: "INSERT"},
		{Entity: "Chapter", Action: "DELETE", ID: 9.0},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("changes of an earlier baseline: %+v %v", got, err)
	}
	encoded, err := json.Marshal(got)
	if want := `[{"entity":"Novel","action":"UPDATE","id":17,"fields":["coverUrl","title"]},{"entity":"Volume","action":"INSERT"},{"entity":"Chapter","action":"DELETE","id":9}]`; err != nil || string(encoded) != want {
		t.Fatalf("served %s: %v", encoded, err)
	}

	// A baseline that cannot be read describes nothing.
	for _, document := range []string{"", `{"v":1,"novels":`, `[]`, `{}`, `{"v":2,"novels":{"17":[]}}`, `{"v":1,"novels":{"first":[]}}`} {
		if got, err := SubmittedChanges([]byte(document), 17); err == nil || got != nil {
			t.Errorf("baseline %q: %+v %v", document, got, err)
		}
	}
}
