// Package rmtest writes small version 6 .rm files for tests.
package rmtest

import (
	"encoding/binary"
	"math"
)

// header is the header of a version 6 file.
const header = "reMarkable .lines file, version=6          "

// Point is a point of a stroke in raw v6 units. W is the width.
type Point struct{ X, Y, W float32 }

// Stroke is a line item.
type Stroke struct {
	// Deleted is the deleted length. A value above 0 gives a block with
	// no value.
	Deleted uint32
	Pen     uint32
	Color   uint32
	Points  []Point
	// RGBA is the explicit color, or nil.
	RGBA *[4]uint8
}

// Rect is a highlight rectangle.
type Rect struct{ X, Y, W, H float64 }

// Glyph is a glyph highlight item.
type Glyph struct {
	Color uint32
	Text  string
	Rects []Rect
	// RGBA is the explicit color, or nil.
	RGBA *[4]uint8
}

type buf []byte

func (b *buf) varuint(v uint32) {
	for v >= 0x80 {
		*b = append(*b, byte(v)|0x80)
		v >>= 7
	}
	*b = append(*b, byte(v))
}

func (b *buf) tag(idx, typ uint32) { b.varuint(idx<<4 | typ) }
func (b *buf) u32(v uint32)        { *b = binary.LittleEndian.AppendUint32(*b, v) }
func (b *buf) u16(v uint16)        { *b = binary.LittleEndian.AppendUint16(*b, v) }
func (b *buf) f32(v float32)       { b.u32(math.Float32bits(v)) }
func (b *buf) f64(v float64)       { *b = binary.LittleEndian.AppendUint64(*b, math.Float64bits(v)) }

// item writes the scene item header and the value subblock.
func (b *buf) item(n int, deleted uint32, value buf) {
	for idx := uint32(1); idx <= 4; idx++ {
		b.tag(idx, 0xF)
		*b = append(*b, 0)
		b.varuint(uint32(n))
	}
	b.tag(5, 0x4)
	b.u32(deleted)
	if deleted == 0 {
		b.tag(6, 0xC)
		b.u32(uint32(len(value)))
		*b = append(*b, value...)
	}
}

// File returns a v6 file with one block for each stroke and then one for
// each glyph. Points use point version 2, as reMarkable OS 3 does.
func File(strokes []Stroke, glyphs []Glyph) []byte {
	out := buf(header)
	block := func(typ byte, body buf) {
		out.u32(uint32(len(body)))
		out = append(out, 0, 1, 2, typ)
		out = append(out, body...)
	}
	n := 0
	for _, s := range strokes {
		n++
		var v buf
		v = append(v, 0x03)
		v.tag(1, 0x4)
		v.u32(s.Pen)
		v.tag(2, 0x4)
		v.u32(s.Color)
		v.tag(3, 0x8)
		v.f64(2)
		v.tag(4, 0x4)
		v.f32(0)
		v.tag(5, 0xC)
		v.u32(uint32(14 * len(s.Points)))
		for _, p := range s.Points {
			v.f32(p.X)
			v.f32(p.Y)
			v.u16(0)
			v.u16(uint16(p.W * 4))
			v = append(v, 0, 255)
		}
		if s.RGBA != nil {
			v.tag(6, 0xF)
			v = append(v, 0)
			v.varuint(1)
			c := s.RGBA
			v.tag(8, 0x4)
			v.u32(uint32(c[3])<<24 | uint32(c[0])<<16 | uint32(c[1])<<8 | uint32(c[2]))
		}
		var b buf
		b.item(n, s.Deleted, v)
		block(0x05, b)
	}
	for _, g := range glyphs {
		n++
		var v buf
		v = append(v, 0x01)
		v.tag(4, 0x4)
		v.u32(g.Color)
		var t buf
		t.varuint(uint32(len(g.Text)))
		t = append(t, 1)
		t = append(t, g.Text...)
		v.tag(5, 0xC)
		v.u32(uint32(len(t)))
		v = append(v, t...)
		var r buf
		r.varuint(uint32(len(g.Rects)))
		for _, rc := range g.Rects {
			r.f64(rc.X)
			r.f64(rc.Y)
			r.f64(rc.W)
			r.f64(rc.H)
		}
		v.tag(6, 0xC)
		v.u32(uint32(len(r)))
		v = append(v, r...)
		if g.RGBA != nil {
			v.tag(10, 0x4)
			c := g.RGBA
			v.u32(uint32(c[3])<<24 | uint32(c[0])<<16 | uint32(c[1])<<8 | uint32(c[2]))
		}
		var b buf
		b.item(n, 0, v)
		block(0x03, b)
	}
	return out
}
