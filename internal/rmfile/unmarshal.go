package rmfile

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// UnmarshalBinary implements encoding.UnmarshalBinary for
// transforming bytes into a Rm page
func (rm *Rm) UnmarshalBinary(data []byte) error {
	r := newReader(data)
	if err := r.checkHeader(); err != nil {
		return err
	}

	if r.version == V6 {
		result, err := parseV6(data)
		if err != nil {
			return err
		}
		*rm = *result
		return nil
	}

	rm.Version = r.version

	nbLayers, err := r.readNumber()
	if err != nil {
		return err
	}

	if err := r.checkCount(nbLayers, minLayerLen); err != nil {
		return err
	}
	rm.Layers = make([]Layer, nbLayers)
	for i := uint32(0); i < nbLayers; i++ {
		nbLines, err := r.readNumber()
		if err != nil {
			return err
		}

		if err := r.checkCount(nbLines, minLineLen); err != nil {
			return err
		}
		rm.Layers[i].Lines = make([]Line, nbLines)
		for j := uint32(0); j < nbLines; j++ {
			line, err := r.readLine()
			if err != nil {
				return err
			}
			rm.Layers[i].Lines[j] = line
		}
	}

	return nil
}

type reader struct {
	bytes.Reader
	version Version
}

func newReader(data []byte) reader {
	br := bytes.NewReader(data)

	// we set V5 as default but the real value is
	// analysed when checking the header
	return reader{*br, V5}
}

func (r *reader) checkHeader() error {
	buf := make([]byte, HeaderLen)

	n, err := r.Read(buf)
	if err != nil {
		return err
	}

	if n != HeaderLen {
		return errors.New("the header is too short")
	}

	switch string(buf) {
	case HeaderV5:
		r.version = V5
	case HeaderV3:
		r.version = V3
	case HeaderV6:
		r.version = V6
	default:
		return errors.New("the header is not a known .rm header")
	}

	return nil
}

// The smallest sizes in bytes of a layer, a line and a point in a v3 or
// v5 file.
const (
	minLayerLen = 4
	minLineLen  = 20
	pointLen    = 24
)

// checkCount returns an error if the remaining bytes cannot hold n items
// of the size size. The check prevents a large allocation for a count
// that the file cannot satisfy.
func (r *reader) checkCount(n uint32, size int) error {
	if int64(n)*int64(size) > int64(r.Len()) {
		return errors.New("a count is larger than the file")
	}
	return nil
}

func (r *reader) readNumber() (uint32, error) {
	var nb uint32
	if err := binary.Read(r, binary.LittleEndian, &nb); err != nil {
		return 0, errors.New("cannot read a number")
	}
	return nb, nil
}

func (r *reader) readLine() (Line, error) {
	var line Line

	if err := binary.Read(r, binary.LittleEndian, &line.BrushType); err != nil {
		return line, errLine
	}

	if err := binary.Read(r, binary.LittleEndian, &line.BrushColor); err != nil {
		return line, errLine
	}

	if err := binary.Read(r, binary.LittleEndian, &line.Padding); err != nil {
		return line, errLine
	}

	if err := binary.Read(r, binary.LittleEndian, &line.BrushSize); err != nil {
		return line, errLine
	}

	// this new attribute has been added in v5
	if r.version == V5 {
		if err := binary.Read(r, binary.LittleEndian, &line.Unknown); err != nil {
			return line, errLine
		}
	}

	nbPoints, err := r.readNumber()
	if err != nil {
		return line, err
	}

	if nbPoints == 0 {
		return line, nil
	}

	if err := r.checkCount(nbPoints, pointLen); err != nil {
		return line, err
	}
	line.Points = make([]Point, nbPoints)

	for i := uint32(0); i < nbPoints; i++ {
		p, err := r.readPoint()
		if err != nil {
			return line, err
		}

		line.Points[i] = p
	}

	return line, nil
}

func (r *reader) readPoint() (Point, error) {
	var point Point

	if err := binary.Read(r, binary.LittleEndian, &point.X); err != nil {
		return point, errPoint
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Y); err != nil {
		return point, errPoint
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Speed); err != nil {
		return point, errPoint
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Direction); err != nil {
		return point, errPoint
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Width); err != nil {
		return point, errPoint
	}
	if err := binary.Read(r, binary.LittleEndian, &point.Pressure); err != nil {
		return point, errPoint
	}

	return point, nil
}

var (
	errLine  = errors.New("cannot read a line: the file is too short")
	errPoint = errors.New("cannot read a point: the file is too short")
)
