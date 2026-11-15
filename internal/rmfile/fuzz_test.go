package rmfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

// FuzzUnmarshalBinary checks that the parser does not panic on a file
// from an account holder. The seeds are the fixtures and one file with a
// stroke and a highlight.
func FuzzUnmarshalBinary(f *testing.F) {
	fixtures, err := filepath.Glob("testdata/*.rm")
	if err != nil {
		f.Fatal(err)
	}
	for _, fn := range fixtures {
		b, err := os.ReadFile(fn)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add(rmtest.File(
		[]rmtest.Stroke{{Points: []rmtest.Point{{X: 1, Y: 2, W: 3}, {X: 4, Y: 5, W: 6}}}},
		[]rmtest.Glyph{{Text: "text", Rects: []rmtest.Rect{{X: 1, Y: 2, W: 3, H: 4}}}},
	))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_ = New().UnmarshalBinary(data)
	})
}
