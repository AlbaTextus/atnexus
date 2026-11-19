package storage

import (
	"context"
	"errors"
	"maps"
	"testing"
)

// What the approval of a change may do, given what its rehearsal did: only what
// the rehearsal did as well, to records that still hold what it found there.
func TestBaselinePermits(t *testing.T) {
	chapter := func(id float64, title, hash string) map[string]any {
		return map[string]any{"id": id, "novelId": 1.0, "volumeId": nil, "order": 2.0, "title": title, "contentHash": hash, "contentLength": 9.0}
	}
	novel := func(title, authorName string) map[string]any {
		return map[string]any{"id": 1.0, "title": title, "tagIds": []any{3.0}, "authorName": authorName, "platformName": "Platform", "tagNames": []any{"tag"}}
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
	tests := map[string]struct {
		rehearsed, approved []Change
		want                bool
	}{
		"the same update":                       {retitled, retitled, true},
		"nothing left to do":                    {retitled, nil, true},
		"update of a field that was left alone": {retitled, update("Chapter", stored, chapter(7, "New title", "other hash")), false},
		"update from another value":             {retitled, update("Chapter", chapter(7, "Their title", "hash"), chapter(7, "New title", "hash")), false},
		// Chapter 8 took the place of chapter 7 and reads the same.
		"update of another row in the same place": {retitled, update("Chapter", chapter(8, "Title", "hash"), chapter(8, "New title", "hash")), false},
		"update that was not rehearsed":           {nil, retitled, false},
		"update where a creation was rehearsed":   {created, retitled, false},
		"renamed author alone":                    {nil, update("Novel", novel("Title", "Author"), novel("Title", "Renamed author")), true},
		"renamed author next to the same update": {
			update("Novel", novel("Title", "Author"), novel("New title", "Author")),
			update("Novel", novel("Title", "Author"), novel("New title", "Renamed author")), true},
		// The row a rehearsal creates is rolled back, so the next one has another ID.
		"creation that was rehearsed":               {created, insert("Chapter", chapter(9, "Title", "hash")), true},
		"creation that was not rehearsed":           {nil, created, false},
		"creation where an update was rehearsed":    {retitled, insert("Chapter", chapter(9, "New title", "hash")), false},
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
