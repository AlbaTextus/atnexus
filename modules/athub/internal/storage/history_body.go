package storage

import (
	"context"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Offsets count Unicode code points; line numbers start at one.
// Only changed fragments and a little context are persisted in the service DB.
type BodyHunk struct {
	BeforeStart int        `json:"beforeStart"`
	AfterStart  int        `json:"afterStart"`
	BeforeLine  int        `json:"beforeLine"`
	AfterLine   int        `json:"afterLine"`
	Parts       []BodyPart `json:"parts"`
}

type BodyPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Called under the parent novel lock, before writing. New chapters retain the
// existing INSERT marker instead of copying a complete initial body to history.
func (p *Postgres) trackChapterBodies(ctx context.Context, q database, novelID int32, orders []int32) error {
	m, ok := ctx.Value(mutationKey{}).(*mutation)
	if !ok || len(orders) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT id,content FROM "Chapter" WHERE "novelId"=$1 AND "order"=ANY($2::integer[])`, novelID, orders)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		var body string
		if err := rows.Scan(&id, &body); err != nil {
			return err
		}
		if _, tracked := m.chapterBodies[id]; !tracked {
			m.chapterBodies[id] = body
		}
	}
	return rows.Err()
}

func chapterBodyDiff(before, after string) []BodyHunk {
	if before == after {
		return nil
	}
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 50 * time.Millisecond
	diffs := dmp.DiffMain(before, after, true)
	oldPositions, newPositions := make([]int, len(diffs)+1), make([]int, len(diffs)+1)
	for i, part := range diffs {
		oldPositions[i+1], newPositions[i+1] = oldPositions[i], newPositions[i]
		size := utf8.RuneCountInString(part.Text)
		if part.Type != diffmatchpatch.DiffInsert {
			oldPositions[i+1] += size
		}
		if part.Type != diffmatchpatch.DiffDelete {
			newPositions[i+1] += size
		}
	}
	lineStarts := func(text string) []int {
		starts := []int{0}
		position := 0
		for _, char := range text {
			position++
			if char == '\n' {
				starts = append(starts, position)
			}
		}
		return starts
	}
	oldLines, newLines := lineStarts(before), lineStarts(after)
	lineAt := func(lines []int, offset int) int {
		return sort.Search(len(lines), func(i int) bool { return lines[i] > offset })
	}
	const contextRunes = 48
	var hunks []BodyHunk
	for i := 0; i < len(diffs); i++ {
		if diffs[i].Type == diffmatchpatch.DiffEqual {
			continue
		}
		last := i
		for j := i + 1; j < len(diffs); j++ {
			if diffs[j].Type == diffmatchpatch.DiffEqual && utf8.RuneCountInString(diffs[j].Text) > contextRunes*2 {
				break
			}
			if diffs[j].Type != diffmatchpatch.DiffEqual {
				last = j
			}
		}
		hunk := BodyHunk{BeforeStart: oldPositions[i], AfterStart: newPositions[i]}
		if i > 0 {
			context := []rune(diffs[i-1].Text)
			context = context[max(0, len(context)-contextRunes):]
			hunk.BeforeStart -= len(context)
			hunk.AfterStart -= len(context)
			hunk.Parts = append(hunk.Parts, BodyPart{Kind: "equal", Text: string(context)})
		}
		hunk.BeforeLine, hunk.AfterLine = lineAt(oldLines, hunk.BeforeStart), lineAt(newLines, hunk.AfterStart)
		for _, part := range diffs[i : last+1] {
			kind := "equal"
			if part.Type == diffmatchpatch.DiffDelete {
				kind = "delete"
			} else if part.Type == diffmatchpatch.DiffInsert {
				kind = "insert"
			}
			hunk.Parts = append(hunk.Parts, BodyPart{Kind: kind, Text: part.Text})
		}
		if last+1 < len(diffs) {
			context := []rune(diffs[last+1].Text)
			context = context[:min(contextRunes, len(context))]
			hunk.Parts = append(hunk.Parts, BodyPart{Kind: "equal", Text: string(context)})
		}
		hunks = append(hunks, hunk)
		i = last
	}
	return hunks
}
