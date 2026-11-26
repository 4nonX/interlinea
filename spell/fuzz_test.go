package spell

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

// FuzzLoad reads dictionaries (.aff and .dic) people add to a server, and
// checks and corrects words with them.
func FuzzLoad(f *testing.F) {
	f.Add([]byte(englishAff), []byte(englishDic), "recolors")
	f.Add([]byte("SET UTF-8\nFLAG long\nSFX Aa Y 1\nSFX Aa 0 en .\nCOMPOUNDFLAG Cc\n"), []byte("2\nMiete/AaCc\nVertrag/Cc\n"), "Mietvertrag")
	f.Add([]byte("SET ISO8859-1\nFLAG num\nPFX 1 Y 1\nPFX 1 0 un .\nREP 1\nREP ss ß\n"), []byte("1\nklar/1\n"), "unklar")
	f.Fuzz(func(t *testing.T, aff, dic []byte, word string) {
		d, err := Load(aff, dic)
		if err != nil {
			return
		}
		d.Spell(word)
		d.Suggest(word, 5)
	})
}

// FuzzReadArchive reads the archives dictionaries come in (LibreOffice
// extensions).
func FuzzReadArchive(f *testing.F) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"dictionaries.xcu": `<?xml version="1.0"?><oor:component-data xmlns:oor="http://openoffice.org/2001/registry" oor:name="Linguistic" oor:package="org.openoffice.Office"><node oor:name="ServiceManager"><node oor:name="Dictionaries"><node oor:name="HunSpellDic_de" oor:op="fuse"><prop oor:name="Locations"><value>%origin%/de_DE.aff %origin%/de_DE.dic</value></prop><prop oor:name="Locales"><value>de-DE</value></prop></node></node></node></oor:component-data>`,
		"de_DE.aff":        englishAff,
		"de_DE.dic":        englishDic,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Add(buf.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		ReadArchive(data)
	})
}

// A word too long to correct gets no suggestions, at once.
func TestSuggestLongWord(t *testing.T) {
	d := load(t, englishAff, englishDic)
	long := strings.Repeat("ab", MaxSuggestLength)
	start := time.Now()
	if got := d.Suggest(long, 5); got != nil {
		t.Fatalf("suggestions for %d characters: %v", len(long), got)
	}
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Fatalf("took %v", took)
	}
	if got := d.Suggest("bots", 5); len(got) == 0 {
		t.Fatal("no suggestions for a short word")
	}
}

// A word of many dashes is checked as it is, not split every way: the
// splits double with each dash.
func TestManyDashes(t *testing.T) {
	d := load(t, englishAff, englishDic)
	word := "boot" + strings.Repeat("-boot", 30)
	start := time.Now()
	if d.Spell(word) {
		t.Fatal("thirty dashes accepted")
	}
	d.Suggest(word, 5)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %v", took)
	}
	if !d.Spell("boot-box") {
		t.Fatal("a word of two parts is wrong")
	}
}
