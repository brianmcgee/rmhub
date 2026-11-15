package rmfile

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

// v6Stroke is a stroke for the test encoder.
type v6Stroke struct {
	deleted uint32
	pen     uint32
	color   uint32
	points  [][2]float32
}

// encodeV6 writes a v6 file with one line block for each stroke. It uses
// point version 2, as reMarkable OS 3 does.
func encodeV6(strokes ...v6Stroke) []byte {
	out := []byte(HeaderV6)
	for i, s := range strokes {
		var b []byte
		varuint := func(v uint32) {
			for v >= 0x80 {
				b = append(b, byte(v)|0x80)
				v >>= 7
			}
			b = append(b, byte(v))
		}
		tag := func(idx, typ uint32) { varuint(idx<<4 | typ) }
		u32 := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
		for idx := uint32(1); idx <= 4; idx++ {
			tag(idx, tagID)
			b = append(b, 0)
			varuint(uint32(i + 1))
		}
		tag(5, tagByte4)
		u32(s.deleted)
		if s.deleted == 0 {
			var v []byte
			v = append(v, lineItemType)
			vt := func(idx, typ uint32) {
				x := idx<<4 | typ
				for x >= 0x80 {
					v = append(v, byte(x)|0x80)
					x >>= 7
				}
				v = append(v, byte(x))
			}
			vt(1, tagByte4)
			v = binary.LittleEndian.AppendUint32(v, s.pen)
			vt(2, tagByte4)
			v = binary.LittleEndian.AppendUint32(v, s.color)
			vt(3, tagByte8)
			v = binary.LittleEndian.AppendUint64(v, math.Float64bits(2))
			vt(4, tagByte4)
			v = binary.LittleEndian.AppendUint32(v, 0)
			vt(5, tagLength4)
			v = binary.LittleEndian.AppendUint32(v, uint32(14*len(s.points)))
			for _, p := range s.points {
				v = binary.LittleEndian.AppendUint32(v, math.Float32bits(p[0]))
				v = binary.LittleEndian.AppendUint32(v, math.Float32bits(p[1]))
				v = binary.LittleEndian.AppendUint16(v, 0)  // speed
				v = binary.LittleEndian.AppendUint16(v, 12) // width * 4
				v = append(v, 0, 255)                       // direction, pressure
			}
			tag(6, tagLength4)
			u32(uint32(len(v)))
			b = append(b, v...)
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(len(b)))
		out = append(out, 0, 1, 2, blockTypeLineItem)
		out = append(out, b...)
	}
	return out
}

func TestV6RawX(t *testing.T) {
	var rm Rm
	if err := rm.UnmarshalBinary(encodeV6(v6Stroke{pen: uint32(Fineliner), points: [][2]float32{{-500, 100}, {300, 200}}})); err != nil {
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
		v6Stroke{points: [][2]float32{{0, 0}}},
		v6Stroke{deleted: 1},
		v6Stroke{color: uint32(Red), points: [][2]float32{{1, 1}}},
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
	data := encodeV6(v6Stroke{points: [][2]float32{{0, 0}, {1, 1}, {2, 2}}})
	for _, n := range []int{len(data) - 1, len(data) - 20, HeaderLen + 2} {
		var rm Rm
		if err := rm.UnmarshalBinary(data[:n]); err == nil {
			t.Errorf("a file cut at %d of %d bytes gives no error", n, len(data))
		}
	}
	// A block with bad content gives an error too.
	bad := encodeV6(v6Stroke{points: [][2]float32{{0, 0}}})
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
