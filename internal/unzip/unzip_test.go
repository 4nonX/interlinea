package unzip

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"testing"
)

func archive(t *testing.T, add func(zw *zip.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add(zw)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOpenAndRead(t *testing.T) {
	data := archive(t, func(zw *zip.Writer) {
		w, _ := zw.Create("word/document.xml")
		w.Write(bytes.Repeat([]byte("<w:t>Miete</w:t>"), 100_000))
	})
	zr, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(zr.File[0], 0)
	if err != nil || len(b) != 1_600_000 {
		t.Fatalf("read %d bytes, %v", len(b), err)
	}
	if _, err := Read(zr.File[0], 1<<20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("an entry over its limit: %v", err)
	}
}

// Entries whose sizes add up to more than MaxTotal: a few bytes each in
// the file, gigabytes unpacked.
func TestTooMuchUnpacked(t *testing.T) {
	data := archive(t, func(zw *zip.Writer) {
		for _, name := range []string{"a.xml", "b.xml", "c.xml"} {
			w, err := zw.CreateRaw(&zip.FileHeader{Name: name, Method: zip.Store, UncompressedSize64: 400 << 20, CompressedSize64: 1, CRC32: crc32.ChecksumIEEE([]byte("x"))})
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte("x"))
		}
	})
	if _, err := Open(data); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("1.2 GB unpacked: %v", err)
	}
}

// More entries than MaxEntries is refused from the end record, before
// the directory is read; that many entries also make it a zip64 archive.
func TestTooManyEntries(t *testing.T) {
	data := archive(t, func(zw *zip.Writer) {
		for i := 0; i <= MaxEntries; i++ {
			zw.CreateHeader(&zip.FileHeader{Name: "e", Method: zip.Store})
		}
	})
	if n, ok := entries(data); !ok || n != MaxEntries+1 {
		t.Fatalf("entries %d %v", n, ok)
	}
	if _, err := Open(data); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too many entries: %v", err)
	}
	small := archive(t, func(zw *zip.Writer) { zw.Create("a") })
	if n, ok := entries(small); !ok || n != 1 {
		t.Fatalf("one entry: %d %v", n, ok)
	}
	if _, ok := entries([]byte("not an archive")); ok {
		t.Fatal("an end record in no archive")
	}
}
