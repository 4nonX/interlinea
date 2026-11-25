package pdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func texts(t *testing.T, data []byte) []string {
	t.Helper()
	doc, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range doc.Paragraphs {
		out = append(out, p.Text)
	}
	return out
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	for i := 0; i < len(got) || i < len(want); i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			t.Fatalf("paragraph %d:\n got %q\nwant %q\n\nall:\n%s", i+1, g, w, strings.Join(got, "\n"))
		}
	}
}

// A page set with the fonts every reader has, named without their widths:
// a heading, two columns with a divided word, a numbered list, a line
// shown word by word, and a second page.
func TestColumns(t *testing.T) {
	want := []string{
		"Safety instructions",
		"Read these instructions before you use the device for the first time. Keep them for later, and hand them on with the device if you give it to someone else.",
		"Do not open the housing. There are no parts inside that you can repair.",
		"1. Connect the cable.",
		"2. Switch the device on.",
		"Note: the lamp blinks twice.",
		"The device is made for use indoors. Do not use it near water, and do not cover its vents.",
		"Unplug the device during a storm and when you do not use it for a long time.",
		"Second page, one line.",
	}
	same(t, texts(t, read(t, "columns.pdf")), want)
	// The same file with its objects packed into streams, and its table
	// of objects a stream.
	same(t, texts(t, read(t, "columns-packed.pdf")), want)

	doc, err := Read(read(t, "columns.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Pages != 2 || doc.Paragraphs[0].Size != 18 || !doc.Paragraphs[0].Bold || doc.Paragraphs[1].Bold || !doc.Paragraphs[5].Italic || doc.Paragraphs[8].Page != 2 {
		t.Fatalf("pages %d, paragraphs %+v", doc.Pages, doc.Paragraphs)
	}
	if int(doc.Width) != 595 || int(doc.Height) != 841 {
		t.Fatalf("page size %v x %v", doc.Width, doc.Height)
	}
}

// A page printed by a browser: embedded fonts with maps to text, text
// set justified word by word, a list, a table and two columns.
func TestBrowser(t *testing.T) {
	same(t, texts(t, read(t, "browser.pdf")), []string{
		"Mietvertrag über Wohnraum",
		"Zwischen Herrn Jonas Hofmann (Vermieter) und Frau Lea Kraus (Mieterin) wird folgender Mietvertrag geschlossen. Die Wohnung im zweiten Obergeschoss des Hauses Lindenstraße 12, 80331 München, wird zu Wohnzwecken vermietet; sie besteht aus drei Zimmern, einer Küche, einem Bad und einem Kellerraum.",
		"Das Mietverhältnis beginnt am 1. Januar 2027 und läuft auf unbestimmte Zeit. Die ordentliche Kündigung richtet sich nach den gesetzlichen Vorschriften.",
		"§ 2 Miete und Betriebskosten",
		"Die monatliche Miete beträgt 1.250,00 EUR zuzüglich einer Vorauszahlung auf die Betriebskosten in Höhe von 220,00 EUR.",
		"Die Miete ist monatlich im Voraus zu zahlen, spätestens am dritten Werktag eines jeden Monats, und zwar kostenfrei auf das Konto des Vermieters.",
		"Die Kaution beträgt drei Monatsmieten.",
		"Position",
		"Kaltmiete",
		"Betriebskosten",
		"Betrag",
		"1.250,00 EUR",
		"220,00 EUR",
		"§ 3 Hausordnung",
		"Die Mieterin verpflichtet sich, die Hausordnung einzuhalten und auf die übrigen Bewohner des Hauses Rücksicht zu nehmen. Ruhestörender Lärm ist zu vermeiden, insbesondere in der Zeit von 22 Uhr bis 6 Uhr.",
		"Haustiere dürfen nur mit vorheriger Zustimmung des Vermieters gehalten werden. Kleintiere wie Fische oder Hamster sind davon ausgenommen. Die Treppenhausreinigung erfolgt im wöchentlichen Wechsel durch die Mietparteien des jeweiligen Stockwerks.",
	})
}

// A manual written in a word processor and saved as PDF: headings, a
// paragraph with bold words in it, bullets from a symbol font, a numbered
// list, a table, several scripts, two columns over a page break, and a
// footer on every page.
func TestManual(t *testing.T) {
	interval := func(n int) string {
		return fmt.Sprintf("Maintenance interval %d. The bearings are lubricated for life and need no maintenance. Check the mechanical seal for leaks every 2,000 operating hours, and replace it if liquid escapes at the lantern. Clean the fan cover so that the motor is cooled sufficiently.", n)
	}
	same(t, texts(t, read(t, "manual.pdf")), []string{
		"Operating manual",
		"This manual describes how the pump is installed, started and serviced. Read it completely before you begin, and keep it where everyone who works on the pump can find it. The manufacturer accepts no liability for damage that results from not following these instructions.",
		"1 Safety",
		"Work on the electrical system may only be carried out by a qualified electrician. Disconnect the pump from the mains before any work, and secure it against being switched on again. Never run the pump dry. Dry running destroys the mechanical seal within seconds – the “F3” message then appears on the display.",
		"• Wear protective gloves and goggles.",
		"• Do not touch hot surfaces: the motor housing can reach more than 80 °C in continuous operation, and it cools down slowly.",
		"• Keep children away.",
		"1. Close the shut-off valves.",
		"2. Loosen the four screws on the housing cover.",
		"3. Lift off the cover.",
		"2 Technical data",
		"Model", "P 100", "P 200", "P 400 with frequency converter",
		"Flow rate", "100 l/min", "200 l/min", "400 l/min",
		"Power", "0.75 kW", "1.5 kW", "3.0 kW",
		"Größe, Maße und Gewicht entnehmen Sie dem Typenschild. Français: l’appareil doit être installé à l’abri du gel. Polski: żółć gęślą jaźń. Ελληνικά: αντλία. Русский: насос. 日本語：ポンプの取扱説明書。",
		interval(1), interval(2),
		"Pump P series – operating manual",
		interval(3), interval(4),
		"Pump P series – operating manual",
		"3 Disposal",
		"Dispose of the pump and its packaging in accordance with local regulations.",
		"Pump P series – operating manual",
	})
}

func TestNoText(t *testing.T) {
	if _, err := Read(read(t, "protected.pdf")); !errors.Is(err, ErrEncrypted) {
		t.Fatalf("a protected file: %v", err)
	}
	if _, err := Read(read(t, "picture.pdf")); !errors.Is(err, ErrNoText) {
		t.Fatalf("a file of pictures: %v", err)
	}
	if _, err := Read([]byte("Just some text.")); err == nil || !strings.Contains(err.Error(), "it is not a PDF file") {
		t.Fatalf("a text file: %v", err)
	}
	if _, err := Read([]byte("%PDF-1.4\nnothing else")); err == nil {
		t.Fatal("a file with no objects is read")
	}
}

// build writes a PDF by hand: the objects in order, numbered from 1, with
// no table of objects, so the reader has to find them itself. The first
// object is the catalog.
func build(objects ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	for i, o := range objects {
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	b.WriteString("trailer\n<< /Root 1 0 R >>\n%%EOF\n")
	return b.Bytes()
}

func content(s string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
}

const cmap = `/CIDInit /ProcSet findresource begin 12 dict begin begincmap
1 begincodespacerange <0000> <FFFF> endcodespacerange
2 beginbfchar <0001> <0041> <0002> <00660069> endbfchar
1 beginbfrange <0010> <0012> <0061> endbfrange
1 beginbfrange <0020> <0021> [<00E9> <D83DDE00>] endbfrange
endcmap end end`

// What the fixtures do not have: a file without a table of objects, a
// font of two-byte codes with a map to text, kerning that is a space and
// kerning that is not, text a stretch says it stands for, a form drawn on
// the page, a page shown turned, glyph names, and an accent drawn before
// its letter.
func TestByHand(t *testing.T) {
	page := strings.Join([]string{
		"BT /F1 12 Tf 72 700 Td [(Hel) -20 (lo) -300 (wor) 15 (ld)] TJ ET",
		// Two-byte codes: A, fi, a, b, c, é, and an emoji.
		"BT /F2 12 Tf 72 680 Td <000100020010001100120020> Tj <0021> Tj ET",
		"/Span << /ActualText (Stra\\337e) >> BDC BT /F1 12 Tf 72 660 Td (Stra) Tj (sse) Tj ET EMC",
		"q 1 0 0 1 0 -60 cm /Fm1 Do Q",
		// Glyph names in the font's encoding: 1 is Zcaron, 2 is uni20AC.
		"BT /F3 12 Tf 72 620 Td (\\001ilina 5 \\002) Tj ET",
		// A circumflex drawn, then the letter under it.
		"BT /F1 12 Tf 72 600 Td (r) Tj 2 0 Td <C3> Tj 0 0 Td (ole) Tj ET",
		// Lines of a paragraph, moved to in three ways; é in the encoding of Windows.
		"BT /F3 12 Tf 14 TL 72 580 Td (Caf\\351 is the first line of a paragraph and) Tj 0 -14 Td (it runs on to the second line, and to the) Tj (third.) ' ET",
	}, "\n")
	turned := "BT /F1 12 Tf 0 1 -1 0 300 100 Tm (Read from the side.) Tj ET"
	data := build(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 10 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R /F2 6 0 R /F3 9 0 R >> /XObject << /Fm1 8 0 R >> >> >>",
		"<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>",
		content(page),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /StandardEncoding >>",
		"<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Some-Bold /Encoding /Identity-H /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /DW 500 /W [1 [600 700] 16 18 450] >>] /ToUnicode 7 0 R >>",
		content(cmap),
		"<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Length 49 >>\nstream\nBT /F1 12 Tf 72 700 Td (Drawn from a form.) Tj ET\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Custom /FirstChar 1 /Widths [600 600] /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [1 /Zcaron /uni20AC] >> >>",
		"<< /Type /Page /Parent 2 0 R /Rotate 90 /Contents 11 0 R >>",
		content(turned),
	)
	same(t, texts(t, data), []string{
		"Hello world",
		"Afiabcé😀",
		"Straße",
		"Drawn from a form.",
		"Žilina 5 €",
		"rôle",
		"Café is the first line of a paragraph and it runs on to the second line, and to the third.",
		"Read from the side.",
	})
	doc, _ := Read(data)
	if !doc.Paragraphs[1].Bold || doc.Paragraphs[0].Bold || doc.Paragraphs[7].Page != 2 {
		t.Fatalf("%+v", doc.Paragraphs)
	}
	if doc.Width != 612 || doc.Height != 792 {
		t.Fatalf("page size %v x %v", doc.Width, doc.Height)
	}

	// A font that does not say which characters it draws.
	blind := build(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		"<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		content("BT /F1 12 Tf 72 700 Td <0001000200030004> Tj ET"),
		"<< /Type /Font /Subtype /Type0 /BaseFont /Blind /Encoding /Identity-H /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 >>] >>",
	)
	if _, err := Read(blind); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a font without characters: %v", err)
	}
}

func TestStreamFilters(t *testing.T) {
	f := &file{}
	hex := &stream{dict{"Filter": []any{name("ASCIIHexDecode")}}, []byte("48 65 6C6c 6F>")}
	if got, err := decodeStream(f, hex); err != nil || string(got) != "Hello" {
		t.Fatalf("hex: %q %v", got, err)
	}
	a85 := &stream{dict{"Filter": name("ASCII85Decode")}, []byte("<~87cURD]i,\"Ebo80~>")}
	if got, err := decodeStream(f, a85); err != nil || string(got) != "Hello World!" {
		t.Fatalf("ascii85: %q %v", got, err)
	}
	rl := &stream{dict{"Filter": name("RunLengthDecode")}, []byte{2, 'a', 'b', 'c', 254, 'x', 128}}
	if got, err := decodeStream(f, rl); err != nil || string(got) != "abcxxx" {
		t.Fatalf("run length: %q %v", got, err)
	}
	// The PNG predictor "up" over two rows of three bytes.
	rows := []byte{2, 1, 2, 3, 2, 1, 1, 1}
	if got, err := unpredict(f, rows, dict{"Predictor": 12.0, "Columns": 3.0}); err != nil || !bytes.Equal(got, []byte{1, 2, 3, 2, 3, 4}) {
		t.Fatalf("predictor: %v %v", got, err)
	}
	if _, err := decodeStream(f, &stream{dict{"Filter": name("DCTDecode")}, nil}); err == nil {
		t.Fatal("a picture's filter is decoded")
	}
}

func TestToWord(t *testing.T) {
	data := read(t, "manual.pdf")
	word, err := ToWord(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := ToWord(data)
	if !bytes.Equal(word, again) {
		t.Fatal("the same PDF gives another document the second time")
	}
	zr, err := zip.NewReader(bytes.NewReader(word), int64(len(word)))
	if err != nil {
		t.Fatal(err)
	}
	var document string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			raw, _ := io.ReadAll(r)
			document = string(raw)
		}
	}
	for _, want := range []string{
		`<w:p><w:r><w:rPr><w:sz w:val="52"/><w:szCs w:val="52"/></w:rPr><w:t xml:space="preserve">Operating manual</w:t></w:r></w:p>`,
		`<w:rPr><w:b/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr><w:t xml:space="preserve">1 Safety</w:t>`,
		`<w:p><w:pPr><w:pageBreakBefore/></w:pPr><w:r><w:rPr><w:sz w:val="22"/>`,
		`the “F3” message`,
		`<w:pgSz w:w="12240" w:h="15840"/>`,
	} {
		if !strings.Contains(document, want) {
			t.Fatalf("the document lacks %s:\n%s", want, document)
		}
	}
	if strings.Count(document, "<w:pageBreakBefore/>") != 2 {
		t.Fatalf("page breaks: %d", strings.Count(document, "<w:pageBreakBefore/>"))
	}
}

// Damaged files are refused or read as far as they go, and never bring
// the reader down.
func TestDamaged(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for _, name := range []string{"columns.pdf", "columns-packed.pdf", "browser.pdf"} {
		whole := read(t, name)
		for i := 0; i < 150; i++ {
			data := append([]byte(nil), whole...)
			switch i % 3 {
			case 0:
				data = data[:rnd.Intn(len(data))]
			case 1:
				for j := 0; j < 20; j++ {
					data[rnd.Intn(len(data))] = byte(rnd.Intn(256))
				}
			case 2:
				at := rnd.Intn(len(data))
				data = append(data[:at], data[min(len(data), at+rnd.Intn(400)):]...)
			}
			Read(data)
		}
	}
	deep := []byte("%PDF-1.4\n1 0 obj\n" + strings.Repeat("[", 100000) + "\nendobj\ntrailer << /Root 1 0 R >>")
	if _, err := Read(deep); err == nil {
		t.Fatal("a file of nothing but nesting is read")
	}
}
