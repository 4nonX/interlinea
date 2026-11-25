package pdf

import (
	"bytes"
	"compress/lzw"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"
)

// maxDecoded is how large one decoded stream may be.
const maxDecoded = 256 << 20

// decodeStream undoes the filters of a stream.
func decodeStream(f *file, s *stream) ([]byte, error) {
	var filters []name
	switch x := f.resolve(first(s.dict, "Filter", "F")).(type) {
	case name:
		filters = []name{x}
	case []any:
		for _, v := range x {
			filters = append(filters, f.name(v))
		}
	}
	var parms []dict
	switch x := f.resolve(first(s.dict, "DecodeParms", "DP")).(type) {
	case dict:
		parms = []dict{x}
	case []any:
		for _, v := range x {
			parms = append(parms, f.dict(v))
		}
	}
	data := s.raw
	for i, flt := range filters {
		var p dict
		if i < len(parms) {
			p = parms[i]
		}
		var err error
		switch flt {
		case "FlateDecode", "Fl":
			data, err = inflate(data)
			if err == nil {
				data, err = unpredict(f, data, p)
			}
		case "LZWDecode", "LZW":
			r := lzw.NewReader(bytes.NewReader(data), lzw.MSB, 8)
			data, err = readSome(r)
			r.Close()
			if err == nil {
				data, err = unpredict(f, data, p)
			}
		case "ASCII85Decode", "A85":
			data, err = unascii85(data)
		case "ASCIIHexDecode", "AHx":
			l := &lexer{b: data}
			data = []byte(l.hex())
		case "RunLengthDecode", "RL":
			data = unrunlength(data)
		default:
			return nil, fmt.Errorf("the filter %s is not read", flt)
		}
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func first(d dict, keys ...name) any {
	for _, k := range keys {
		if v, ok := d[k]; ok {
			return v
		}
	}
	return nil
}

// readSome reads all a reader gives; what was read before an error in the
// data counts, as writers of PDF files leave such errors behind.
func readSome(r io.Reader) ([]byte, error) {
	var out bytes.Buffer
	_, err := io.Copy(&out, io.LimitReader(r, maxDecoded+1))
	if out.Len() > maxDecoded {
		return nil, errors.New("a stream is too large")
	}
	if err != nil && out.Len() == 0 {
		return nil, err
	}
	return out.Bytes(), nil
}

func inflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return readSome(r)
}

func unascii85(data []byte) ([]byte, error) {
	data = bytes.TrimSpace(data)
	data = bytes.TrimPrefix(data, []byte("<~"))
	if end := bytes.Index(data, []byte("~>")); end >= 0 {
		data = data[:end]
	}
	return readSome(ascii85.NewDecoder(bytes.NewReader(data)))
}

func unrunlength(data []byte) []byte {
	var out []byte
	for i := 0; i < len(data); {
		n := int(data[i])
		i++
		switch {
		case n < 128:
			end := min(len(data), i+n+1)
			out = append(out, data[i:end]...)
			i = end
		case n > 128:
			if i < len(data) {
				out = append(out, bytes.Repeat(data[i:i+1], 257-n)...)
				i++
			}
		default:
			return out
		}
		if len(out) > maxDecoded {
			return out
		}
	}
	return out
}

// unpredict undoes the PNG and TIFF predictors of Flate and LZW data.
func unpredict(f *file, data []byte, p dict) ([]byte, error) {
	if p == nil {
		return data, nil
	}
	pred, _ := f.number(p["Predictor"])
	if pred < 2 {
		return data, nil
	}
	num := func(k name, def int) int {
		if v, ok := f.number(p[k]); ok {
			return int(v)
		}
		return def
	}
	colors, bpc, columns := num("Colors", 1), num("BitsPerComponent", 8), num("Columns", 1)
	if colors < 1 || bpc < 1 || columns < 1 || colors > 32 || bpc > 16 || columns > 1<<20 {
		return nil, errors.New("a stream's predictor is out of range")
	}
	bpp := (colors*bpc + 7) / 8
	row := (colors*bpc*columns + 7) / 8
	if pred == 2 {
		if bpc != 8 {
			return data, nil
		}
		for r := 0; r+row <= len(data); r += row {
			for i := bpp; i < row; i++ {
				data[r+i] += data[r+i-bpp]
			}
		}
		return data, nil
	}
	out := make([]byte, 0, len(data))
	prev := make([]byte, row)
	for p := 0; p+1 <= len(data); p += row + 1 {
		kind := data[p]
		line := make([]byte, row)
		copy(line, data[p+1:min(len(data), p+1+row)])
		for i := range line {
			var a, b, c int
			if i >= bpp {
				a, c = int(line[i-bpp]), int(prev[i-bpp])
			}
			b = int(prev[i])
			switch kind {
			case 1:
				line[i] += byte(a)
			case 2:
				line[i] += byte(b)
			case 3:
				line[i] += byte((a + b) / 2)
			case 4:
				pa, pb, pc := abs(b-c), abs(a-c), abs(a+b-2*c)
				switch {
				case pa <= pb && pa <= pc:
					line[i] += byte(a)
				case pb <= pc:
					line[i] += byte(b)
				default:
					line[i] += byte(c)
				}
			}
		}
		out = append(out, line...)
		prev = line
	}
	return out, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
