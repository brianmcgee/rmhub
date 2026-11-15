package render

import (
	"bytes"
	"context"
	"image/color"
	"testing"

	"github.com/brianmcgee/rmhub/internal/render/rendertest"
	"github.com/brianmcgee/rmhub/internal/rmfile"
	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

func TestPDF(t *testing.T) {
	p, err := NewPDF(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	pdf := rendertest.PDF()
	ctx := context.Background()

	// 1404 units for 612 points: 1 point is 2.294 units. The page is
	// 1404 × 1817 units.
	strokes := rmtest.File([]rmtest.Stroke{line(rmfile.Fineliner, rmfile.Black, -400, -200, 1000)}, nil)
	out, err := p.Render(ctx, pdf, 1, strokes, 1404)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 1404 || b.Dy() != 1817 {
		t.Fatalf("size %v", b)
	}
	red := color.RGBA{255, 0, 0, 255}
	if got := rgb(img, 344, 287); !near(got, red, 10) {
		t.Errorf("PDF rectangle: %v", got)
	}
	if got := rgb(img, 402, 1000); !near(got, color.RGBA{0, 0, 0, 255}, 10) {
		t.Errorf("stroke: %v", got)
	}
	dark := 0
	for y := 380; y < 500; y++ {
		for x := 680; x < 1000; x++ {
			if rgb(img, x, y).R < 100 {
				dark++
			}
		}
	}
	if dark < 100 {
		t.Errorf("PDF text: %d dark pixels", dark)
	}

	// A page without .rm file shows only the PDF.
	img = decode(t, must(p.Render(ctx, pdf, 1, nil, 702)))
	if got := rgb(img, 172, 143); !near(got, red, 10) {
		t.Errorf("PDF only, rectangle: %v", got)
	}
	if got := rgb(img, 201, 500); got != white {
		t.Errorf("PDF only, no stroke: %v", got)
	}

	if n, err := p.PageCount(ctx, pdf); err != nil || n != 1 {
		t.Errorf("page count: %d %v", n, err)
	}

	again := must(p.Render(ctx, pdf, 1, strokes, 1404))
	if !bytes.Equal(out, again) {
		t.Error("two renders differ")
	}

	if _, err := p.Render(ctx, pdf, 2, nil, 702); err == nil {
		t.Error("page 2 of a PDF of one page gives no error")
	}
	if _, err := p.Render(ctx, []byte("not a PDF"), 1, nil, 702); err == nil {
		t.Error("a bad PDF gives no error")
	}
}

// TestPDFMemoryLimit makes sure that a render that needs more memory
// than the limit gives an error, and that the pool renders after it.
func TestPDFMemoryLimit(t *testing.T) {
	ctx := context.Background()
	p, err := newPDF(ctx, 1, 48<<20>>16)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	if _, err := p.Render(ctx, rendertest.PDF(), 1, nil, 4096); err == nil {
		t.Fatal("a render of 84 MiB with a limit of 48 MiB gives no error")
	}
	if _, err := p.Render(ctx, rendertest.PDF(), 1, nil, 200); err != nil {
		t.Fatalf("render after the refused render: %v", err)
	}
}

// TestPDFLargestWidth makes sure that the largest width fits in the
// memory limit.
func TestPDFLargestWidth(t *testing.T) {
	ctx := context.Background()
	p, err := NewPDF(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	if _, err := p.Render(ctx, rendertest.PDF(), 1, nil, 4096); err != nil {
		t.Fatal(err)
	}
}
