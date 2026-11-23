package spell

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func load(t *testing.T, aff, dic string) *Dictionary {
	t.Helper()
	d, err := Load([]byte(aff), []byte(dic))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func expect(t *testing.T, d *Dictionary, good, wrong string) {
	t.Helper()
	for _, w := range strings.Fields(good) {
		if !d.Spell(w) {
			t.Errorf("rejects %q", w)
		}
	}
	for _, w := range strings.Fields(wrong) {
		if d.Spell(w) {
			t.Errorf("accepts %q", w)
		}
	}
}

const englishAff = `SET UTF-8
TRY esianrtolcdugmphbyfvkwzESIANRTOLCDUGMPHBYFVKWZ'
REP 2
REP f ph
REP alot a_lot
PFX A Y 1
PFX A 0 re .
SFX S Y 3
SFX S y ies [^aeiou]y
SFX S 0 s [^sxzhy]
SFX S 0 es [sxzh]
SFX D Y 2
SFX D 0 ed [^ey]
SFX D 0 d e
SFX N N 1
SFX N y iness/S y
FORBIDDENWORD !
KEEPCASE K
NOSUGGEST X
`

const englishDic = `12
boot/ASD
city/S
box/S
bake/D
happy/N
phantom
NASA/K
Paris
iPod/K
a
lot
colour/!
`

func TestAffixes(t *testing.T) {
	d := load(t, englishAff, englishDic)
	expect(t, d,
		"boot boots booted reboot reboots rebooted city cities box boxes bake baked happy happiness happinesses",
		"citys boxs bakeed happys rebox recity happinesss colour")
	// Capitalization: Paris is only capitalized; NASA keeps its case.
	expect(t, d, "Boot BOOT REBOOTS Paris PARIS NASA iPod 1,250.50 12-34", "paris Nasa nasa IPOD ipod 1..2")
	// Abbreviation dots and hyphenated words (BREAK defaults).
	expect(t, d, "boot. boot-city city-", "boot-cityx")
	if d.Words() != 12 {
		t.Fatalf("words: %d", d.Words())
	}
	d.Add("Interlinea")
	expect(t, d, "Interlinea INTERLINEA", "interlinea")
}

func TestSuggest(t *testing.T) {
	d := load(t, englishAff, englishDic)
	cases := map[string]string{
		"fantom":  "phantom",
		"alot":    "a lot",
		"bota":    "boot",
		"citys":   "city",
		"Citys":   "City",
		"BOTS":    "BOOTS",
		"nasa":    "NASA",
		"bakd":    "baked",
		"reboted": "rebooted",
	}
	for w, want := range cases {
		got := d.Suggest(w, 5)
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Suggest(%q) = %v, want %q among them", w, got, want)
		}
		for _, g := range got {
			if g == "colour" {
				t.Errorf("forbidden word suggested for %q", w)
			}
		}
	}
	if s := d.Suggest("xqzwv", 5); len(s) != 0 {
		t.Fatalf("suggestions for nonsense: %v", s)
	}
	if s := d.Suggest("bots", 1); len(s) != 1 {
		t.Fatalf("max not respected: %v", s)
	}
}

func TestTwoSuffixesAndCircumfix(t *testing.T) {
	aff := `SET UTF-8
CIRCUMFIX X
NEEDAFFIX N
SFX A Y 1
SFX A 0 s/B .
SFX B Y 1
SFX B 0 en .
PFX C Y 1
PFX C 0 ge/X .
SFX D Y 1
SFX D 0 t/X .
`
	d := load(t, aff, "3\nlach/ACD\nmach/N\nfoo/AN\n")
	expect(t, d, "lach lachs lachsen gelacht foos foosen", "lachen gelach lacht mach foo")
}

func TestCompoundFlags(t *testing.T) {
	aff := `SET UTF-8
COMPOUNDBEGIN B
COMPOUNDMIDDLE M
COMPOUNDEND E
ONLYINCOMPOUND O
COMPOUNDPERMITFLAG P
CHECKCOMPOUNDTRIPLE
CHECKCOMPOUNDDUP
COMPOUNDMIN 3
SFX S Y 1
SFX S 0 s/BMPO .
`
	dic := "7\nArbeit/BES\nzeit/BME\nZeit/BME\nplan/BME\nschiff/BME\nfahrt/E\ngeber/E\n"
	d := load(t, aff, dic)
	expect(t, d, "Arbeit Arbeitszeit Arbeitszeitplan Zeitplan Arbeitgeber schiffgeber",
		"Arbeits Zeitplanplan fahrtzeit schifffahrt")
	aff2 := "SET UTF-8\nCOMPOUNDFLAG Y\nCHECKCOMPOUNDTRIPLE\nSIMPLIFIEDTRIPLE\n"
	d2 := load(t, aff2, "2\nschiff/Y\nfahrt/Y\n")
	expect(t, d2, "schiffahrt", "schifffahrt")
}

func TestCompoundRule(t *testing.T) {
	aff := `SET UTF-8
COMPOUNDMIN 1
ONLYINCOMPOUND c
COMPOUNDRULE 2
COMPOUNDRULE n*1t
COMPOUNDRULE n*mp
`
	dic := "6\n0/nm\n1/n1\n2/nm\n3/nm\nst/tc\nth/pc\n"
	d := load(t, aff, dic)
	expect(t, d, "1st 21st 2th 32th", "1th 2st st th")
}

func TestCompoundPatternReplacement(t *testing.T) {
	aff := "SET UTF-8\nCOMPOUNDFLAG A\nCOMPOUNDMIN 1\nCHECKCOMPOUNDPATTERN 1\nCHECKCOMPOUNDPATTERN o b z\n"
	d := load(t, aff, "2\nfoo/A\nbar/A\n")
	expect(t, d, "barfoo fozar", "foobar")
}

func TestEncodingsAndFlags(t *testing.T) {
	// ISO 8859-1 with an umlaut in the word and in the affix.
	aff := []byte("SET ISO8859-1\nSFX A Y 1\nSFX A 0 \xe4r .\n")
	dic := []byte("1\nk\xfchl/A\n")
	d, err := Load(aff, dic)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, d, "kühl kühlär Kühl", "kuhl")
	for _, c := range []struct{ aff, dic string }{
		{"SET UTF-8\nFLAG long\nSFX Aa Y 1\nSFX Aa 0 s .\n", "1\nfoo/AaBb\n"},
		{"SET UTF-8\nFLAG num\nSFX 101 Y 1\nSFX 101 0 s .\n", "1\nfoo/7,101\n"},
		{"SET UTF-8\nFLAG UTF-8\nSFX Ä Y 1\nSFX Ä 0 s .\n", "1\nfoo/Ä\n"},
		{"SET UTF-8\nAF 2\nAF AB\nAF C\nSFX C Y 1\nSFX C 0 s .\n", "1\nfoo/2\n"},
	} {
		d := load(t, c.aff, c.dic)
		expect(t, d, "foo foos", "fooss")
	}
	if _, err := Load([]byte("SET KOI8-X\n"), []byte("0\n")); err == nil {
		t.Fatal("unknown character set accepted")
	}
}

func TestConversions(t *testing.T) {
	aff := "SET UTF-8\nICONV 1\nICONV ’ '\nOCONV 1\nOCONV ' ’\nIGNORE \u00ad\nWORDCHARS '\n"
	d := load(t, aff, "2\nl'été\naujourd'hui\n")
	expect(t, d, "l'été l’été aujour\u00add'hui", "lété")
	s := d.Suggest("aujourdhui", 3)
	if len(s) == 0 || s[0] != "aujourd’hui" {
		t.Fatalf("OCONV in suggestions: %v", s)
	}
}

func TestCasing(t *testing.T) {
	d := load(t, "SET UTF-8\nCHECKSHARPS\nKEEPCASE k\n", "2\nStraße\nmüßig/k\n")
	expect(t, d, "Straße STRASSE STRAẞE müßig Müßig MÜSSIG", "strasse Strasse MÜẞIG")
	tr := load(t, "SET UTF-8\nLANG tr_TR\n", "1\nistanbul\n")
	expect(t, tr, "İstanbul İSTANBUL", "ISTANBUL")
}

func TestArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	add("META-INF/manifest.xml", "<manifest/>")
	add("dictionaries.xcu", `<?xml version="1.0"?>
<oor:component-data xmlns:oor="http://openoffice.org/2001/registry" oor:name="Linguistic" oor:package="org.openoffice.Office">
 <node oor:name="ServiceManager"><node oor:name="Dictionaries">
  <node oor:name="HunSpellDic_de_DE" oor:op="fuse">
   <prop oor:name="Locations" oor:type="oor:string-list"><value>%origin%/dict/de_DE_frami.aff %origin%/dict/de_DE_frami.dic</value></prop>
   <prop oor:name="Format" oor:type="xs:string"><value>DICT_SPELL</value></prop>
   <prop oor:name="Locales" oor:type="oor:string-list"><value>de-DE de-BE de-LU</value></prop>
  </node>
 </node></node>
</oor:component-data>`)
	add("dict/de_DE_frami.aff", "SET UTF-8\n")
	add("dict/de_DE_frami.dic", "1\nHaus\n")
	add("en_GB.aff", "SET UTF-8\n")
	add("en_GB.dic", "1\ncolour\n")
	zw.Close()
	bs, err := ReadArchive(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[0].Name != "de_DE_frami" || strings.Join(bs[0].Locales, ",") != "de-DE,de-BE,de-LU" ||
		bs[1].Name != "en_GB" || strings.Join(bs[1].Locales, ",") != "en-GB" {
		t.Fatalf("bundles: %+v", bs)
	}
	d, err := Load(bs[0].Aff, bs[0].Dic)
	if err != nil || !d.Spell("Haus") {
		t.Fatalf("load from archive: %v", err)
	}
	if _, err := ReadArchive([]byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
	var empty bytes.Buffer
	zip.NewWriter(&empty).Close()
	if _, err := ReadArchive(empty.Bytes()); err != ErrNoDictionary {
		t.Fatalf("empty archive: %v", err)
	}
}

func TestBadAff(t *testing.T) {
	for _, aff := range []string{
		"SFX A Y 1\nSFX A 0\n",
		"FLAG weird\n",
		"SFX A Y 1\nSFX A 0 s [abc\n",
		"COMPOUNDRULE 1\nCOMPOUNDRULE *a\n",
	} {
		if _, err := Load([]byte(aff), []byte("0\n")); err == nil {
			t.Errorf("accepted %q", aff)
		}
	}
	if _, err := Load([]byte("FLAG num\n"), []byte("1\nfoo/x\n")); err == nil {
		t.Error("bad numeric flag in .dic accepted")
	}
}
