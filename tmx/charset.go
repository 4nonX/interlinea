package tmx

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// decodeInput returns a UTF-8 reader for r. TMX files exported by Windows
// tools are often UTF-16 with a byte order mark; encoding/xml reads only
// UTF-8, so UTF-16 input is converted here.
func decodeInput(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	bom, err := br.Peek(3)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(bom, []byte{0xEF, 0xBB, 0xBF}):
		if _, err := br.Discard(3); err != nil {
			return nil, err
		}
		return br, nil
	case bytes.HasPrefix(bom, []byte{0xFF, 0xFE}):
		return utf16Reader(br, false)
	case bytes.HasPrefix(bom, []byte{0xFE, 0xFF}):
		return utf16Reader(br, true)
	}
	return br, nil
}

func utf16Reader(r *bufio.Reader, bigEndian bool) (io.Reader, error) {
	if _, err := r.Discard(2); err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		bw := bufio.NewWriter(pw)
		var pending []uint16
		buf := make([]byte, 2)
		for {
			if _, err := io.ReadFull(r, buf); err != nil {
				if err == io.EOF {
					break
				}
				if err == io.ErrUnexpectedEOF {
					pw.CloseWithError(fmt.Errorf("tmx: truncated UTF-16 input"))
					return
				}
				pw.CloseWithError(err)
				return
			}
			var u uint16
			if bigEndian {
				u = uint16(buf[0])<<8 | uint16(buf[1])
			} else {
				u = uint16(buf[1])<<8 | uint16(buf[0])
			}
			pending = append(pending, u)
			if utf16.IsSurrogate(rune(u)) && len(pending) == 1 {
				continue
			}
			for _, rn := range utf16.Decode(pending) {
				var tmp [utf8.UTFMax]byte
				n := utf8.EncodeRune(tmp[:], rn)
				if _, err := bw.Write(tmp[:n]); err != nil {
					pw.CloseWithError(err)
					return
				}
			}
			pending = pending[:0]
		}
		for _, rn := range utf16.Decode(pending) {
			bw.WriteRune(rn)
		}
		if err := bw.Flush(); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.Close()
	}()
	return pr, nil
}

// charsetReader handles the encoding named in the XML declaration. UTF-16
// was already converted by decodeInput, so it passes through.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "utf-16", "utf16", "utf-16le", "utf-16be", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1", "iso8859-1":
		return &singleByteReader{r: bufio.NewReader(input), table: nil}, nil
	case "windows-1252", "cp1252":
		return &singleByteReader{r: bufio.NewReader(input), table: &cp1252}, nil
	default:
		return nil, fmt.Errorf("tmx: unsupported encoding %q", label)
	}
}

// singleByteReader converts ISO-8859-1 or Windows-1252 to UTF-8.
type singleByteReader struct {
	r     *bufio.Reader
	table *[32]rune // mapping for 0x80..0x9F, nil for ISO-8859-1
	buf   []byte
}

func (s *singleByteReader) Read(p []byte) (int, error) {
	for len(s.buf) < len(p) {
		b, err := s.r.ReadByte()
		if err != nil {
			if len(s.buf) == 0 {
				return 0, err
			}
			break
		}
		rn := rune(b)
		if s.table != nil && b >= 0x80 && b <= 0x9F {
			rn = s.table[b-0x80]
		}
		s.buf = utf8.AppendRune(s.buf, rn)
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

var cp1252 = [32]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178,
}
