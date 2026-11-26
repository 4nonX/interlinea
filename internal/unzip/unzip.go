// Package unzip opens the zip archives people give (Word, Excel,
// PowerPoint, InDesign and OpenDocument files, packages, spelling
// dictionaries) within limits, so that a small file that unpacks to
// gigabytes, or lists millions of entries, is refused before it fills
// the memory.
//
// The archive reader stops an entry that unpacks to more than its
// directory says, so the sizes in the directory are an upper bound: an
// archive whose entries add up to no more than MaxTotal never unpacks to
// more.
package unzip

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxEntries is the most entries an archive may have.
	MaxEntries = 100_000
	// MaxTotal is the most all its entries together may unpack to.
	MaxTotal = 1 << 30
	// MaxEntry is the most one entry may unpack to.
	MaxEntry = 512 << 20
)

// ErrTooLarge means an archive has too many entries or unpacks to too
// much.
var ErrTooLarge = errors.New("unzip: the archive is too large")

// Open reads the directory of an archive within the limits.
func Open(data []byte) (*zip.Reader, error) {
	if n, ok := entries(data); ok && n > MaxEntries {
		return nil, fmt.Errorf("%w: it has %d entries, more than %d", ErrTooLarge, n, MaxEntries)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > MaxEntries {
		return nil, fmt.Errorf("%w: it has %d entries, more than %d", ErrTooLarge, len(zr.File), MaxEntries)
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if f.UncompressedSize64 > MaxTotal || total > MaxTotal {
			return nil, fmt.Errorf("%w: it unpacks to more than %d MB", ErrTooLarge, MaxTotal>>20)
		}
	}
	return zr, nil
}

// Read unpacks an entry, refusing one larger than limit bytes (MaxEntry
// when limit is 0 or more than that).
func Read(f *zip.File, limit int64) ([]byte, error) {
	if limit <= 0 || limit > MaxEntry {
		limit = MaxEntry
	}
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w: %s unpacks to more than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%w: %s unpacks to more than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	return b, nil
}

// entries reads how many entries the archive's end record says it has,
// before the directory itself is read. ok is false when there is no end
// record to read; the archive reader then says what is wrong.
func entries(data []byte) (n uint64, ok bool) {
	const (
		endSig      = 0x06054b50
		endLen      = 22
		locatorSig  = 0x07064b50
		locatorLen  = 20
		end64Sig    = 0x06064b50
		maxComment  = 0xffff
		count16Off  = 10
		count64Off  = 32
		end64OffOff = 8
	)
	from := max(0, len(data)-endLen-maxComment)
	for i := len(data) - endLen; i >= from; i-- {
		if binary.LittleEndian.Uint32(data[i:]) != endSig {
			continue
		}
		n16 := binary.LittleEndian.Uint16(data[i+count16Off:])
		if n16 != 0xffff {
			return uint64(n16), true
		}
		// A zip64 archive: its locator stands just before the end record
		// and says where the zip64 end record is.
		l := i - locatorLen
		if l < 0 || binary.LittleEndian.Uint32(data[l:]) != locatorSig {
			return 0, false
		}
		at := binary.LittleEndian.Uint64(data[l+end64OffOff:])
		if at > uint64(len(data)-count64Off-8) || binary.LittleEndian.Uint32(data[at:]) != end64Sig {
			return 0, false
		}
		return binary.LittleEndian.Uint64(data[at+count64Off:]), true
	}
	return 0, false
}
