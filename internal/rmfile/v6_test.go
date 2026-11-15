package rmfile_test

import (
	"os"
	"testing"

	. "github.com/brianmcgee/rmhub/internal/rmfile"
	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

func encodeV6(strokes ...rmtest.Stroke) []byte { return rmtest.File(strokes, nil) }

func TestV6RawX(t *testing.T) {
	var rm Rm
	if err := rm.UnmarshalBinary(encodeV6(rmtest.Stroke{Pen: uint32(Fineliner), Points: []rmtest.Point{{X: -500, Y: 100, W: 3}, {X: 300, Y: 200, W: 3}}})); err != nil {
		t.Fatal(err)
	}
	lines := rm.Layers[0].Lines
	if len(lines) != 1 || len(lines[0].Points) != 2 {
		t.Fatalf("lines: %+v", lines)
	}
	if p := lines[0].Points[0]; p.X != -500 || p.Y != 100 || p.Width != 3 {
		t.Fatalf("point %+v, want the raw X -500", p)
	}
}

func TestV6DeletedStroke(t *testing.T) {
	var rm Rm
	data := encodeV6(
		rmtest.Stroke{Points: []rmtest.Point{{X: 0, Y: 0, W: 3}}},
		rmtest.Stroke{Deleted: 1},
		rmtest.Stroke{Color: uint32(Red), Points: []rmtest.Point{{X: 1, Y: 1, W: 3}}},
	)
	if err := rm.UnmarshalBinary(data); err != nil {
		t.Fatal(err)
	}
	lines := rm.Layers[0].Lines
	if len(lines) != 2 || lines[1].BrushColor != Red {
		t.Fatalf("lines: %+v", lines)
	}
}

func TestV6Truncated(t *testing.T) {
	data := encodeV6(rmtest.Stroke{Points: []rmtest.Point{{X: 0, Y: 0, W: 3}, {X: 1, Y: 1, W: 3}, {X: 2, Y: 2, W: 3}}})
	for _, n := range []int{len(data) - 1, len(data) - 20, HeaderLen + 2} {
		var rm Rm
		if err := rm.UnmarshalBinary(data[:n]); err == nil {
			t.Errorf("a file cut at %d of %d bytes gives no error", n, len(data))
		}
	}
	// A block with bad content gives an error too.
	bad := encodeV6(rmtest.Stroke{Points: []rmtest.Point{{X: 0, Y: 0, W: 3}}})
	bad[HeaderLen+8] = 0x25 // tag 2 with the type byte4 instead of the ID tag 1
	var rm Rm
	if err := rm.UnmarshalBinary(bad); err == nil {
		t.Error("a block with a wrong tag gives no error")
	}
}

// TestV6Fixtures reads files of reMarkable OS 3 from rmscene.
func TestV6Fixtures(t *testing.T) {
	for _, tc := range []struct {
		file       string
		lines      int
		highlights int
	}{
		{"Color_and_tool_v3.14.4.rm", 1, 0},
		{"Normal_A_stroke_2_layers_v3.3.2.rm", 1, 0},
		{"Wikipedia_highlighted_p1.rm", 0, 1},
	} {
		b, err := os.ReadFile("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		var rm Rm
		if err := rm.UnmarshalBinary(b); err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if rm.Version != V6 || len(rm.Layers[0].Lines) < tc.lines || len(rm.Highlights) < tc.highlights {
			t.Errorf("%s: %d lines, %d highlights", tc.file, len(rm.Layers[0].Lines), len(rm.Highlights))
		}
		// The raw X of a v6 file is relative to the center of the page.
		for _, l := range rm.Layers[0].Lines {
			for _, p := range l.Points {
				if p.X < -float32(Width) || p.X > float32(Width) {
					t.Errorf("%s: X %f is outside the page", tc.file, p.X)
				}
			}
		}
	}
}

// TestV6LineColor reads the explicit color of shader strokes.
func TestV6LineColor(t *testing.T) {
	b, err := os.ReadFile("testdata/Color_and_tool_v3.14.4.rm")
	if err != nil {
		t.Fatal(err)
	}
	var rm Rm
	if err := rm.UnmarshalBinary(b); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range rm.Layers[0].Lines {
		if l.BrushColor == HighlightDynamic {
			if l.ColorRGBA == nil {
				t.Fatalf("a line with the color %d has no RGBA value", l.BrushColor)
			}
			n++
		} else if l.ColorRGBA != nil {
			t.Errorf("line with the color %d has the RGBA value %v", l.BrushColor, *l.ColorRGBA)
		}
	}
	if n == 0 {
		t.Fatal("the fixture has no line with an RGBA value")
	}
}
