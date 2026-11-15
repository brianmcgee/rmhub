// Package render draws the strokes and highlights of a .rm file, on a
// white page or on a page of a PDF, as PNG or SVG.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"

	"github.com/brianmcgee/rmhub/internal/rmfile"
)

// Version is the version of the renderer. It is part of the cache key.
// Increment it for each change that changes the output.
const Version = 1

// Page size of the device in .rm units. These are the values of the
// reMarkable 2.
const (
	PageWidth  = 1404.0
	PageHeight = 1872.0
)

// margin is the space around a stroke outside the page, in .rm units.
const margin = 20.0

// highlightAlpha is the opacity of the highlighter and of glyph
// highlights.
const highlightAlpha = 0.4

// Format is an output format.
type Format string

// The output formats.
const (
	PNG Format = "png"
	SVG Format = "svg"
)

// ErrSVGWithPDF is the error for an SVG request of a page with a PDF
// page. It is a bad request.
var ErrSVGWithPDF = errors.New("a page with a PDF page has no SVG output; request PNG")

// ErrTooLarge is the error for a page that is too large to draw. It is
// a bad request: a smaller width can be correct.
var ErrTooLarge = errors.New("the page is too large to render; request a smaller width")

// Limits of the page area. A page can grow to maxGrowX page widths and
// maxGrowY page heights. A PNG image can have maxPixels pixels, which is
// 256 MiB of memory.
const (
	maxGrowX  = 16
	maxGrowY  = 256
	maxPixels = 64 << 20
	// maxWork is the limit of the drawing work of one image, in touched
	// pixels. It keeps a render to some seconds.
	maxWork = 32 * maxPixels
)

// finite is true if each value is a finite number.
func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// Base is the PDF page under the strokes.
type Base interface {
	// Size returns the size of the PDF page in points.
	Size() (w, h float64, err error)
	// Render draws the PDF page with the width px pixels.
	Render(px int) (image.Image, error)
}

// colors maps a brush color to RGB.
var colors = map[rmfile.BrushColor]color.RGBA{
	rmfile.Black:           {0x00, 0x00, 0x00, 0xff},
	rmfile.Grey:            {0x7f, 0x7f, 0x7f, 0xff},
	rmfile.GreyOverlap:     {0x7f, 0x7f, 0x7f, 0xff},
	rmfile.White:           {0xff, 0xff, 0xff, 0xff},
	rmfile.Blue:            {0x1e, 0x4f, 0xd8, 0xff},
	rmfile.Red:             {0xd8, 0x26, 0x1e, 0xff},
	rmfile.Green2:          {0x2a, 0x9d, 0x3a, 0xff},
	rmfile.Cyan:            {0x00, 0xa6, 0xc4, 0xff},
	rmfile.Magenta:         {0xc4, 0x00, 0x8c, 0xff},
	rmfile.Yellow2:         {0xe0, 0xc0, 0x00, 0xff},
	rmfile.HighlightYellow: {0xff, 0xf1, 0x76, 0xff},
	rmfile.HighlightGreen:  {0xa5, 0xd6, 0xa7, 0xff},
	rmfile.HighlightPink:   {0xf4, 0x8f, 0xb1, 0xff},
	// HighlightDynamic without an explicit color.
	rmfile.HighlightDynamic: {0xff, 0xf1, 0x76, 0xff},
}

// colorOf returns the RGB value of a brush color. An unknown color is
// black.
func colorOf(c rmfile.BrushColor) color.RGBA {
	if v, ok := colors[c]; ok {
		return v
	}
	return colors[rmfile.Black]
}

func withAlpha(c color.RGBA, a float64) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(math.Round(255 * a))}
}

// isHighlighter is true for the brushes that draw with 40 % opacity: the
// highlighter and the shader.
func isHighlighter(t rmfile.BrushType) bool {
	return t == rmfile.Highlighter || t == rmfile.HighlighterV5 || t == rmfile.Shader
}

func isEraser(t rmfile.BrushType) bool {
	return t == rmfile.Eraser || t == rmfile.EraseArea
}

// Parse reads a .rm file. An unknown header or a broken file gives an
// error.
func Parse(data []byte) (*rmfile.Rm, error) {
	var rm rmfile.Rm
	if err := rm.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return &rm, nil
}

// layout is the page area in .rm units, after the origin shift.
type layout struct {
	minX, minY, maxX, maxY float64
	// dx is the shift of the X origin: half the page width for a v6
	// file, else 0.
	dx float64
}

func (l layout) w() float64 { return l.maxX - l.minX }
func (l layout) h() float64 { return l.maxY - l.minY }

// height returns the height in pixels of the image with the width
// width. It returns ErrTooLarge if the page area or the image is larger
// than the limits.
func (l layout) height(width int) (int, error) {
	if !(l.w() <= maxGrowX*PageWidth && l.h() <= maxGrowY*PageHeight) {
		return 0, ErrTooLarge
	}
	h := int(l.h()*float64(width)/l.w() + 0.5)
	if h < 1 || h > maxPixels/width {
		return 0, ErrTooLarge
	}
	return h, nil
}

// newLayout returns the page area: the page of the size w × h, grown to
// include each stroke and highlight with a margin. It ignores a point or
// a highlight with a value that is not finite.
func newLayout(rm *rmfile.Rm, w, h float64) layout {
	l := layout{maxX: w, maxY: h}
	if rm == nil {
		return l
	}
	if rm.Version == rmfile.V6 {
		l.dx = PageWidth / 2
	}
	grow := func(x0, y0, x1, y1 float64) {
		l.minX = math.Min(l.minX, x0-margin)
		l.minY = math.Min(l.minY, y0-margin)
		l.maxX = math.Max(l.maxX, x1+margin)
		l.maxY = math.Max(l.maxY, y1+margin)
	}
	for _, ly := range rm.Layers {
		for _, ln := range ly.Lines {
			if isEraser(ln.BrushType) {
				continue
			}
			for _, p := range ln.Points {
				x, y, r := float64(p.X)+l.dx, float64(p.Y), float64(p.Width)/2
				if !finite(x, y, r) {
					continue
				}
				if x-r < 0 || y-r < 0 || x+r > w || y+r > h {
					grow(x-r, y-r, x+r, y+r)
				}
			}
		}
	}
	for _, hl := range rm.Highlights {
		for _, r := range hl.Rects {
			x0, y0 := r.X+l.dx, r.Y
			x1, y1 := x0+r.W, y0+r.H
			if !finite(x0, y0, x1, y1) {
				continue
			}
			if x0 < 0 || y0 < 0 || x1 > w || y1 > h {
				grow(x0, y0, x1, y1)
			}
		}
	}
	return l
}

// stroke is a stroke in page units, with the origin at the top left of
// the layout.
type stroke struct {
	color     color.RGBA
	highlight bool
	pts       []pt
}

type pt struct{ x, y, w float64 }

// rect is a glyph highlight in page units.
type rect struct {
	color      color.RGBA
	x, y, w, h float64
}

// shapes returns the strokes and highlights of the file in file order,
// without erasers, empty strokes and values that are not finite.
func shapes(rm *rmfile.Rm, l layout) ([]stroke, []rect) {
	if rm == nil {
		return nil, nil
	}
	ox, oy := l.dx-l.minX, -l.minY
	var ss []stroke
	for _, ly := range rm.Layers {
		for _, ln := range ly.Lines {
			if isEraser(ln.BrushType) || len(ln.Points) == 0 {
				continue
			}
			s := stroke{color: colorOf(ln.BrushColor), highlight: isHighlighter(ln.BrushType)}
			if ln.ColorRGBA != nil {
				s.color = color.RGBA{ln.ColorRGBA[0], ln.ColorRGBA[1], ln.ColorRGBA[2], 0xff}
			}
			for _, p := range ln.Points {
				if !finite(float64(p.X), float64(p.Y), float64(p.Width)) {
					continue
				}
				s.pts = append(s.pts, pt{float64(p.X) + ox, float64(p.Y) + oy, math.Max(float64(p.Width), 1)})
			}
			if len(s.pts) == 0 {
				continue
			}
			ss = append(ss, s)
		}
	}
	var rs []rect
	for _, hl := range rm.Highlights {
		c := colorOf(hl.Color)
		if hl.ColorRGBA != nil {
			c = color.RGBA{hl.ColorRGBA[0], hl.ColorRGBA[1], hl.ColorRGBA[2], 0xff}
		}
		for _, r := range hl.Rects {
			if !finite(r.X, r.Y, r.W, r.H) {
				continue
			}
			rs = append(rs, rect{c, r.X + ox, r.Y + oy, r.W, r.H})
		}
	}
	return ss, rs
}

// capSteps is the number of polygon points of a half circle of a cap.
const capSteps = 8

// capsule adds a segment with round caps from a to b, with the width w,
// as one convex polygon. A segment of length 0 is a circle.
func capsule(z *vector.Rasterizer, ax, ay, bx, by, w, ox, oy float64) {
	r := w / 2
	th := math.Atan2(by-ay, bx-ax)
	first := true
	add := func(cx, cy, a float64) {
		x, y := float32(cx+r*math.Cos(a)-ox), float32(cy+r*math.Sin(a)-oy)
		if first {
			z.MoveTo(x, y)
			first = false
		} else {
			z.LineTo(x, y)
		}
	}
	for i := 0; i <= capSteps; i++ {
		add(bx, by, th-math.Pi/2+math.Pi*float64(i)/capSteps)
	}
	for i := 0; i <= capSteps; i++ {
		add(ax, ay, th+math.Pi/2+math.Pi*float64(i)/capSteps)
	}
	z.ClosePath()
}

// lineWork is the work for one pixel of the length of a segment, in
// units of maxWork.
const lineWork = 8

// painter draws polygons onto img. Without img, it only adds up the
// work of the drawing.
type painter struct {
	img    *image.RGBA
	bounds image.Rectangle
	// work is the number of pixels that the drawing touches. One small
	// file can ask for much work: each segment touches each pixel of
	// its bounds.
	work int64
}

// paint fills the polygons that add gives, with the color c. The bounds
// x0, y0, x1, y1 are in pixels and contain each polygon.
func (p *painter) paint(c color.Color, x0, y0, x1, y1 float64, add func(z *vector.Rasterizer, ox, oy float64)) {
	// The layout contains each stroke, so bounds outside the image are a
	// rounding case at the edge. Clip the bounds; the rasterizer clips
	// the polygon.
	b := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))).Intersect(p.bounds)
	if b.Empty() {
		return
	}
	p.work += int64(b.Dx()) * int64(b.Dy())
	if p.img == nil {
		return
	}
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	z.DrawOp = draw.Over
	add(z, float64(b.Min.X), float64(b.Min.Y))
	z.Draw(p.img, b, image.NewUniform(c), image.Point{})
}

// segment adds the work for the outline of one segment from a to b.
func (p *painter) segment(a, b pt, w, res float64) {
	p.work += lineWork * int64((math.Abs(a.x-b.x)+math.Abs(a.y-b.y))*res+w)
}

// checkWork returns ErrTooLarge if the drawing of the strokes and
// highlights onto an image with the bounds needs more work than maxWork.
func checkWork(bounds image.Rectangle, ss []stroke, rs []rect, res float64) error {
	p := painter{bounds: bounds}
	p.run(ss, rs, res)
	if p.work > maxWork {
		return ErrTooLarge
	}
	return nil
}

// rasterize draws the strokes and highlights onto img. res is the number
// of pixels for one page unit.
func rasterize(img *image.RGBA, ss []stroke, rs []rect, res float64) {
	p := painter{img: img, bounds: img.Bounds()}
	p.run(ss, rs, res)
}

func (p *painter) run(ss []stroke, rs []rect, res float64) {
	for _, s := range ss {
		pts := s.pts
		if len(pts) == 1 {
			pts = []pt{pts[0], pts[0]}
		}
		if s.highlight {
			// One mask for the full stroke, so the overlap of two
			// segments does not add opacity.
			var sum float64
			x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			for _, p := range pts {
				sum += p.w
			}
			w := sum / float64(len(pts)) * res
			for _, p := range pts {
				x0, y0 = math.Min(x0, p.x*res-w), math.Min(y0, p.y*res-w)
				x1, y1 = math.Max(x1, p.x*res+w), math.Max(y1, p.y*res+w)
			}
			for i := 1; i < len(pts); i++ {
				p.segment(pts[i-1], pts[i], w, res)
			}
			p.paint(withAlpha(s.color, highlightAlpha), x0, y0, x1, y1, func(z *vector.Rasterizer, ox, oy float64) {
				for i := 1; i < len(pts); i++ {
					capsule(z, pts[i-1].x*res, pts[i-1].y*res, pts[i].x*res, pts[i].y*res, w, ox, oy)
				}
			})
			continue
		}
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			w := (a.w + b.w) / 2 * res
			p.segment(a, b, w, res)
			p.paint(s.color,
				math.Min(a.x, b.x)*res-w, math.Min(a.y, b.y)*res-w,
				math.Max(a.x, b.x)*res+w, math.Max(a.y, b.y)*res+w,
				func(z *vector.Rasterizer, ox, oy float64) {
					capsule(z, a.x*res, a.y*res, b.x*res, b.y*res, w, ox, oy)
				})
		}
	}
	for _, r := range rs {
		x0, y0, x1, y1 := r.x*res, r.y*res, (r.x+r.w)*res, (r.y+r.h)*res
		p.paint(withAlpha(r.color, highlightAlpha), x0, y0, x1, y1, func(z *vector.Rasterizer, ox, oy float64) {
			z.MoveTo(float32(x0-ox), float32(y0-oy))
			z.LineTo(float32(x1-ox), float32(y0-oy))
			z.LineTo(float32(x1-ox), float32(y1-oy))
			z.LineTo(float32(x0-ox), float32(y1-oy))
			z.ClosePath()
		})
	}
}

// num formats a coordinate for SVG with at most two decimals.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	for s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return strings.TrimSuffix(s, ".")
}

func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// writeSVG writes the page as SVG. The view box uses page units.
func writeSVG(ss []stroke, rs []rect, l layout, width int) []byte {
	var b bytes.Buffer
	h := l.h() * float64(width) / l.w()
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%s" viewBox="0 0 %s %s">`+"\n", width, num(h), num(l.w()), num(l.h()))
	fmt.Fprintf(&b, `<rect width="%s" height="%s" fill="#ffffff"/>`+"\n", num(l.w()), num(l.h()))
	for _, s := range ss {
		pts := s.pts
		if len(pts) == 1 {
			pts = []pt{pts[0], pts[0]}
		}
		if s.highlight {
			var sum float64
			b.WriteString(`<polyline fill="none" stroke-linecap="round" stroke-linejoin="round" stroke-opacity="0.4" points="`)
			for i, p := range pts {
				sum += p.w
				if i > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(num(p.x) + "," + num(p.y))
			}
			fmt.Fprintf(&b, `" stroke="%s" stroke-width="%s"/>`+"\n", hex(s.color), num(sum/float64(len(pts))))
			continue
		}
		fmt.Fprintf(&b, `<g stroke="%s" stroke-linecap="round">`, hex(s.color))
		for i := 1; i < len(pts); i++ {
			a, c := pts[i-1], pts[i]
			fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke-width="%s"/>`, num(a.x), num(a.y), num(c.x), num(c.y), num((a.w+c.w)/2))
		}
		b.WriteString("</g>\n")
	}
	for _, r := range rs {
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s" fill-opacity="0.4"/>`+"\n", num(r.x), num(r.y), num(r.w), num(r.h), hex(r.color))
	}
	b.WriteString("</svg>\n")
	return b.Bytes()
}

// Render draws a page. data is the .rm file of the page, or nil for a
// page without strokes. base is the PDF page, or nil. width is the image
// width in pixels.
func Render(data []byte, base Base, format Format, width int) ([]byte, error) {
	var rm *rmfile.Rm
	if data != nil {
		var err error
		if rm, err = Parse(data); err != nil {
			return nil, err
		}
	}
	if format != PNG && format != SVG {
		return nil, fmt.Errorf("the format %q is not known", format)
	}
	if base != nil && format != PNG {
		return nil, ErrSVGWithPDF
	}
	if base != nil {
		return renderPDF(rm, base, width)
	}
	l := newLayout(rm, PageWidth, PageHeight)
	height, err := l.height(width)
	if format == SVG {
		// An SVG has no pixels, but the page area has the same limits.
		if _, err := l.height(1); err != nil {
			return nil, err
		}
		ss, rs := shapes(rm, l)
		return writeSVG(ss, rs, l, width), nil
	}
	if err != nil {
		return nil, err
	}
	ss, rs := shapes(rm, l)
	res := float64(width) / l.w()
	bounds := image.Rect(0, 0, width, height)
	if err := checkWork(bounds, ss, rs, res); err != nil {
		return nil, err
	}
	img := image.NewRGBA(bounds)
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	rasterize(img, ss, rs, res)
	return encodePNG(img)
}

// renderPDF draws the PDF page at the top left with the page width, and
// the strokes on top.
func renderPDF(rm *rmfile.Rm, base Base, width int) ([]byte, error) {
	pw, ph, err := base.Size()
	if err != nil {
		return nil, err
	}
	if !finite(pw, ph) || pw <= 0 || ph <= 0 {
		return nil, fmt.Errorf("the PDF page has the size %g × %g", pw, ph)
	}
	l := newLayout(rm, PageWidth, PageWidth*ph/pw)
	height, err := l.height(width)
	if err != nil {
		return nil, err
	}
	res := float64(width) / l.w()
	bounds := image.Rect(0, 0, width, height)
	ss, rs := shapes(rm, l)
	// Check the strokes first: a refusal then costs no PDF render.
	if err := checkWork(bounds, ss, rs, res); err != nil {
		return nil, err
	}
	img := image.NewRGBA(bounds)
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	pdf, err := base.Render(int(PageWidth*res + 0.5))
	if err != nil {
		return nil, err
	}
	at := image.Pt(int(-l.minX*res+0.5), int(-l.minY*res+0.5))
	draw.Draw(img, pdf.Bounds().Sub(pdf.Bounds().Min).Add(at), pdf, pdf.Bounds().Min, draw.Over)
	rasterize(img, ss, rs, res)
	return encodePNG(img)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
