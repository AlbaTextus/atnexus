package api

import (
	"math"
	"testing"

	"albatextus/athub/internal/storage"
)

func TestIllustrationLayoutValidation(t *testing.T) {
	block := storage.IllustrationBlock{ImageID: 23, X: 0, Y: 0, Width: 300, Height: 200}
	for _, scenario := range []struct {
		name  string
		edit  func(*storage.IllustrationLayout)
		valid bool
	}{
		{"normal", func(*storage.IllustrationLayout) {}, true},
		{"empty canvas", func(v *storage.IllustrationLayout) { v.Blocks = []storage.IllustrationBlock{} }, true},
		{"wrong parent", func(v *storage.IllustrationLayout) { v.NovelID = 99 }, false},
		{"missing blocks", func(v *storage.IllustrationLayout) { v.Blocks = nil }, false},
		{"negative revision", func(v *storage.IllustrationLayout) { v.Revision = -1 }, false},
		{"duplicate image", func(v *storage.IllustrationLayout) { v.Blocks = append(v.Blocks, block) }, false},
		{"negative position", func(v *storage.IllustrationLayout) { v.Blocks[0].X = -1 }, false},
		{"too wide", func(v *storage.IllustrationLayout) { v.Blocks[0].Width = 1001 }, false},
		{"too low", func(v *storage.IllustrationLayout) { v.Blocks[0].Y = 4900 }, false},
		{"too small", func(v *storage.IllustrationLayout) { v.Blocks[0].Height = 2 }, false},
		{"nonfinite", func(v *storage.IllustrationLayout) { v.Blocks[0].X = math.NaN() }, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := storage.IllustrationLayout{NovelID: 17, Blocks: []storage.IllustrationBlock{block}}
			scenario.edit(&input)
			if err := validateIllustrationLayout(input, 17); (err == nil) != scenario.valid {
				t.Fatalf("valid=%v error=%v", scenario.valid, err)
			}
		})
	}
}
