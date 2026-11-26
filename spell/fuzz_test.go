package spell

import (
	"archive/zip"
	"bytes"
	"testing"
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
