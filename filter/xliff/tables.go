package xliff

import (
	"bytes"
	"errors"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/rtf"
)

// table is a bilingual table for review: a Trados review document, a
// memoQ two-column RTF or a Déjà Vu RTF external view.
type table interface {
	extraction() *filter.Extraction
	merge(data []byte, updates map[string]Update) ([]byte, error)
}

func (f *reviewFile) extraction() *filter.Extraction { return &f.ext }
func (f *rtfTable) extraction() *filter.Extraction   { return &f.ext }

type rtfTableFile struct{ *rtfTable }

func (f rtfTableFile) merge(_ []byte, updates map[string]Update) ([]byte, error) {
	return f.rtfTable.merge(updates)
}

var (
	errNotTable  = errors.New("xliff: the RTF file is not a bilingual table (two columns, or an external view)")
	errNotReview = errors.New("xliff: the Word document is not a bilingual table for review")
)

// noLanguagesNote is the warning for a table that does not say its
// languages: the project's are taken.
const noLanguagesNote = "The file does not say which languages it is in; the project's source and target languages are taken."

func isRTF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n\ufeff"), []byte(`{\rtf`))
}

func isZip(data []byte) bool { return bytes.HasPrefix(data, []byte("PK\x03\x04")) }

// detectTable tells which bilingual table data is.
func detectTable(data []byte) (string, bool) {
	switch {
	case isRTF(data):
		doc, err := rtf.Read(data)
		if err != nil {
			return "", false
		}
		switch {
		case isMQRTF(doc):
			return FormatMQRTF, true
		case isDVRTF(doc):
			return FormatDVRTF, true
		}
	case isZip(data):
		if f := docxTableFormat(data); f != "" {
			return f, true
		}
	}
	return "", false
}

// readTable reads a bilingual table.
func readTable(data []byte) (table, error) {
	if isZip(data) {
		if !IsReviewDOCX(data) {
			return nil, errNotReview
		}
		return readReviewDOCX(data)
	}
	doc, err := rtf.Read(data)
	if err != nil {
		return nil, err
	}
	var t *rtfTable
	switch {
	case isMQRTF(doc):
		t, err = readMQRTF(doc)
	case isDVRTF(doc):
		t, err = readDVRTF(doc)
	default:
		return nil, errNotTable
	}
	if err != nil {
		return nil, err
	}
	return rtfTableFile{t}, nil
}

// Fixed reports whether a bilingual format comes from a program that keeps
// its own record of every segment (Trados, memoQ, Phrase, Wordfast,
// TagEditor, Déjà Vu), so its segments must not be split or joined.
func Fixed(format string) bool {
	switch format {
	case FormatSDL, FormatMQ, FormatPhrase, FormatTXML, FormatTTX, FormatSDLReview, FormatMQRTF, FormatMQDOCX, FormatDVRTF:
		return true
	}
	return false
}
