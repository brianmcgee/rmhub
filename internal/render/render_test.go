package render

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/rmfile"
	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func rgb(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) <= tol && int(y)-int(x) <= tol }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

// line returns a horizontal stroke from x0 to x1 at y, in raw v6 units.
func line(pen rmfile.BrushType, c rmfile.BrushColor, x0, x1, y float32) rmtest.Stroke {
	var pts []rmtest.Point
	for x := x0; x <= x1; x += 10 {
		pts = append(pts, rmtest.Point{X: x, Y: y, W: 8})
	}
	return rmtest.Stroke{Pen: uint32(pen), Color: uint32(c), Points: pts}
}

func withRGBA(s rmtest.Stroke, c [4]uint8) rmtest.Stroke {
	s.RGBA = &c
	return s
}

var white = color.RGBA{255, 255, 255, 255}

func TestStrokes(t *testing.T) {
	data := rmtest.File([]rmtest.Stroke{
		line(rmfile.Fineliner, rmfile.Black, -400, -200, 300),
		line(rmfile.BallPoint, rmfile.Red, -400, -200, 400),
		line(rmfile.Highlighter, rmfile.HighlightYellow, -400, -200, 500),
		line(rmfile.Eraser, rmfile.Black, -400, -200, 600),
		line(rmfile.EraseArea, rmfile.Black, -400, -200, 700),
		line(rmfile.Fineliner, rmfile.Black, 200, 400, 300),
		{Deleted: 1},
		withRGBA(line(rmfile.Shader, rmfile.HighlightDynamic, -400, -200, 1000), [4]uint8{0, 0, 255, 255}),
	}, []rmtest.Glyph{
		{Color: uint32(rmfile.HighlightYellow), Text: "a", Rects: []rmtest.Rect{{X: 100, Y: 800, W: 200, H: 40}}},
		{Color: uint32(rmfile.HighlightDynamic), Text: "b", Rects: []rmtest.Rect{{X: 100, Y: 900, W: 200, H: 40}}, RGBA: &[4]uint8{0, 0, 255, 255}},
	})
	out, err := Render(data, nil, PNG, 1404)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 1404 || b.Dy() != 1872 {
		t.Fatalf("size %v", b)
	}
	// X = 0 of a v6 file is the center of the page: -300 is at 402.
	for _, tc := range []struct {
		name string
		x, y int
		want color.RGBA
		tol  int
	}{
		{"black stroke", 402, 300, color.RGBA{0, 0, 0, 255}, 10},
		{"right stroke", 1002, 300, color.RGBA{0, 0, 0, 255}, 10},
		{"next to the stroke", 402, 320, white, 0},
		{"red stroke", 402, 400, color.RGBA{0xd8, 0x26, 0x1e, 255}, 10},
		{"eraser", 402, 600, white, 0},
		{"erase area", 402, 700, white, 0},
		{"left of the strokes", 250, 300, white, 0},
	} {
		if got := rgb(img, tc.x, tc.y); !near(got, tc.want, tc.tol) {
			t.Errorf("%s at (%d,%d): %v, want %v", tc.name, tc.x, tc.y, got, tc.want)
		}
	}
	// 40 % opacity: lighter than the color and darker than white.
	for _, tc := range []struct {
		name string
		x, y int
		full color.RGBA
	}{
		{"highlighter", 402, 500, colors[rmfile.HighlightYellow]},
		{"glyph highlight", 702 + 200, 820, colors[rmfile.HighlightYellow]},
		{"RGBA highlight", 702 + 200, 920, color.RGBA{0, 0, 255, 255}},
		{"RGBA shader", 402, 1000, color.RGBA{0, 0, 255, 255}},
	} {
		got := rgb(img, tc.x, tc.y)
		if got == white || near(got, tc.full, 4) {
			t.Errorf("%s: %v is not a 40 %% tint of %v", tc.name, got, tc.full)
		}
		if tc.full.B > tc.full.R && got.B <= got.R || tc.full.R > tc.full.B && got.R <= got.B {
			t.Errorf("%s: %v does not have the hue of %v", tc.name, got, tc.full)
		}
	}
}

func TestLongPage(t *testing.T) {
	data := rmtest.File([]rmtest.Stroke{line(rmfile.Fineliner, rmfile.Black, -100, 100, 3000)}, nil)
	img := decode(t, must(Render(data, nil, PNG, 702)))
	// Scale 0.5: the page grows to 3000 + 4 (half the width) + 20 units.
	if h := img.Bounds().Dy(); h != 1512 {
		t.Fatalf("height %d, want 1512", h)
	}
	if got := rgb(img, 351, 1500); !near(got, color.RGBA{0, 0, 0, 255}, 40) {
		t.Fatalf("stroke at Y 3000: %v", got)
	}
	// A stroke left of the page moves the page origin.
	data = rmtest.File([]rmtest.Stroke{line(rmfile.Fineliner, rmfile.Black, -800, -760, 100)}, nil)
	img = decode(t, must(Render(data, nil, PNG, 1404+118)))
	if got := rgb(img, 20+20, 100); !near(got, color.RGBA{0, 0, 0, 255}, 10) {
		t.Fatalf("stroke left of the page: %v", got)
	}
}

func TestOldVersions(t *testing.T) {
	for _, f := range []string{"test_v3.rm", "test_v5.rm"} {
		b, err := os.ReadFile("../rmfile/testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		if l := newLayout(rm, PageWidth, PageHeight); l.dx != 0 {
			t.Errorf("%s: X origin %g, want the left edge", f, l.dx)
		}
		img := decode(t, must(Render(b, nil, PNG, 702)))
		dark := 0
		for y := 0; y < img.Bounds().Dy(); y += 2 {
			for x := 0; x < img.Bounds().Dx(); x += 2 {
				if c := rgb(img, x, y); c.R < 128 {
					dark++
				}
			}
		}
		if dark == 0 {
			t.Errorf("%s: the image has no strokes", f)
		}
	}
}

func TestRealV6(t *testing.T) {
	for _, f := range []string{"Color_and_tool_v3.14.4.rm", "Wikipedia_highlighted_p1.rm"} {
		b, err := os.ReadFile("../rmfile/testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}
		img := decode(t, must(Render(b, nil, PNG, 702)))
		n := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				if rgb(img, x, y) != white {
					n++
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: the image is white", f)
		}
	}
}

func TestErrors(t *testing.T) {
	if _, err := Render([]byte("not an rm file at all, but long enough for a header"), nil, PNG, 500); err == nil {
		t.Error("an unknown header gives no error")
	}
	if _, err := Render(nil, fakeBase{}, SVG, 500); !errors.Is(err, ErrSVGWithPDF) {
		t.Errorf("SVG of a PDF page: %v", err)
	}
}

func TestDeterministic(t *testing.T) {
	b, err := os.ReadFile("../rmfile/testdata/Color_and_tool_v3.14.4.rm")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Format{PNG, SVG} {
		a := must(Render(b, nil, f, 800))
		c := must(Render(b, nil, f, 800))
		if !bytes.Equal(a, c) {
			t.Errorf("%s: two renders differ", f)
		}
	}
	s := string(must(Render(b, nil, SVG, 800)))
	if !strings.Contains(s, "<svg") || !strings.Contains(s, `width="800`) {
		t.Errorf("SVG: %.200s", s)
	}
}

type fakeBase struct{}

func (fakeBase) Size() (float64, float64, error) { return 612, 792, nil }
func (fakeBase) Render(px int) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, px, px*792/612)), nil
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// TestDrawingWork makes sure that a small file whose segments each cover
// the full image gives ErrTooLarge and not a render of many minutes. A
// smaller width needs less work and is correct.
func TestDrawingWork(t *testing.T) {
	var pts []rmtest.Point
	for i := range 401 {
		p := rmtest.Point{X: -700, Y: 0, W: 2}
		if i%2 == 1 {
			p = rmtest.Point{X: 700, Y: 20000, W: 2}
		}
		pts = append(pts, p)
	}
	for _, pen := range []rmfile.BrushType{rmfile.Fineliner, rmfile.Highlighter} {
		var strokes []rmtest.Stroke
		for i := 1; i < len(pts); i++ {
			strokes = append(strokes, rmtest.Stroke{Pen: uint32(pen), Points: pts[i-1 : i+1]})
		}
		data := rmtest.File(strokes, nil)
		if _, err := Render(data, nil, PNG, 2048); !errors.Is(err, ErrTooLarge) {
			t.Errorf("pen %d with the width 2048: error %v, want ErrTooLarge", pen, err)
		}
		if _, err := Render(data, fakeBase{}, PNG, 2048); !errors.Is(err, ErrTooLarge) {
			t.Errorf("pen %d on a PDF page with the width 2048: error %v, want ErrTooLarge", pen, err)
		}
		if _, err := Render(data, nil, PNG, 100); err != nil {
			t.Errorf("pen %d with the width 100: %v", pen, err)
		}
	}
}

// TestBadCoordinates makes sure that coordinates far from the page give
// ErrTooLarge and no large image, and that the renderer ignores
// coordinates that are not finite.
func TestBadCoordinates(t *testing.T) {
	far := func(x, y float32) []byte {
		return rmtest.File([]rmtest.Stroke{{Pen: uint32(rmfile.Fineliner), Points: []rmtest.Point{{X: 0, Y: 100, W: 8}, {X: x, Y: y, W: 8}}}}, nil)
	}
	for name, data := range map[string][]byte{
		"far down":  far(0, 1e9),
		"far right": far(1e30, 100),
		"far rect":  rmtest.File(nil, []rmtest.Glyph{{Text: "a", Rects: []rmtest.Rect{{X: 0, Y: 0, W: 10, H: 1e300}}}}),
	} {
		for _, f := range []Format{PNG, SVG} {
			if _, err := Render(data, nil, f, 4096); !errors.Is(err, ErrTooLarge) {
				t.Errorf("%s as %s: error %v, want ErrTooLarge", name, f, err)
			}
		}
	}
	// A long page at the largest width has too many pixels. A smaller
	// width is correct.
	long := far(0, 30000)
	if _, err := Render(long, nil, PNG, 4096); !errors.Is(err, ErrTooLarge) {
		t.Errorf("long page with the width 4096: error %v, want ErrTooLarge", err)
	}
	if _, err := Render(long, nil, PNG, 702); err != nil {
		t.Errorf("long page with the width 702: %v", err)
	}

	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	data := rmtest.File([]rmtest.Stroke{
		{Pen: uint32(rmfile.Fineliner), Points: []rmtest.Point{{X: -300, Y: 300, W: 8}, {X: nan, Y: 300, W: 8}, {X: -200, Y: inf, W: 8}, {X: -200, Y: 300, W: 8}}},
		{Pen: uint32(rmfile.Fineliner), Points: []rmtest.Point{{X: inf, Y: nan, W: 8}}},
	}, []rmtest.Glyph{{Text: "a", Rects: []rmtest.Rect{{X: math.NaN(), Y: 0, W: 10, H: math.Inf(1)}}}})
	out, err := Render(data, nil, PNG, 1404)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 1404 || b.Dy() != 1872 {
		t.Fatalf("size %v", b)
	}
	if c := rgb(img, 452, 300); !near(c, color.RGBA{0, 0, 0, 255}, 40) {
		t.Errorf("the finite points of the stroke give %v, want black", c)
	}
	svg, err := Render(data, nil, SVG, 1404)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(svg), "NaN") || strings.Contains(string(svg), "Inf") {
		t.Errorf("the SVG has a value that is not finite:\n%s", svg)
	}
}
