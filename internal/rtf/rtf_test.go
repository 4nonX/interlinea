package rtf

import (
	"strings"
	"testing"
)

func cellTexts(t *testing.T, d *Doc) [][]string {
	t.Helper()
	var out [][]string
	for _, r := range d.Rows {
		var row []string
		for _, c := range r.Cells {
			row = append(row, c.Text())
		}
		out = append(out, row)
	}
	return out
}

func TestReadTable(t *testing.T) {
	src := `{\rtf1\ansi\ansicpg1252\uc1\deff0{\fonttbl{\f0\fswiss\fcharset0 Arial;}{\f1\froman\fcharset238 Times CE;}}` +
		`{\stylesheet{\s0 Normal;}{\*\cs15 \additive \cf13 mqInternal;}}` + "\r\n" +
		`{\info{\title Not text}}` +
		`\trowd\cellx1000\cellx2000\pard\intbl {Caf\'e9 \u8364?\cell}{\f1 \'9a\cell}\row` + "\n" +
		`\trowd\cellx1000\cellx2000\pard\intbl {{\cs15 [1\}}x{\*\bkmkstart b}y\line z\tab w\cell }{\uc2\u-10179\'3f\'3f\u-8704\'3f\'3f!\~\_\-\cell}\row}`
	d, err := Read([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := cellTexts(t, d)
	want := [][]string{{"Caf\u00e9 \u20ac", "\u0161"}, {"[1}xy\nz\tw", "\U0001f600!\u00a0\u2011\u00ad"}}
	if len(got) != len(want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	for i := range want {
		if strings.Join(got[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d: %q, want %q", i, got[i], want[i])
		}
	}
	if d.Styles["mqInternal"] != 15 {
		t.Errorf("styles %v", d.Styles)
	}
	runs := d.Rows[1].Cells[0].Runs
	if runs[0].Style != 15 || runs[0].Text != "[1}" || !strings.Contains(runs[0].Prefix, `\cs15`) {
		t.Errorf("tag run %+v", runs[0])
	}
	if runs[1].Style != -1 {
		t.Errorf("text run %+v", runs[1])
	}
}

func TestDoubleByte(t *testing.T) {
	// Shift-JIS: 日本 is 93 fa 96 7b; the second byte of 本 is a plain "{"
	// in the file only when escaped, so both are written as \'hh here.
	src := `{\rtf1\ansi\ansicpg932{\fonttbl{\f0\fcharset128 MS Mincho;}}\trowd\cellx1\pard\intbl {\f0\'93\'fa\'96\'7b}\cell\row}`
	d, err := Read([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Rows[0].Cells[0].Text(); got != "\u65e5\u672c" {
		t.Errorf("got %q", got)
	}
}

func TestClearCell(t *testing.T) {
	src := `{\rtf1\ansi\trowd\cellx1\cellx2\pard\intbl {\b A\cell}{\lang1031 old {\cs15 [1]} text\u233?\par more{\*\bkmkstart k}\cell }\row}`
	d, err := Read([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	c := d.Rows[0].Cells[1]
	cleared := string(d.ClearCell(c))
	if strings.Contains(cleared, "old") || strings.Contains(cleared, "text") || strings.Contains(cleared, "more") || strings.Contains(cleared, `\u233`) || strings.Contains(cleared, `\par`) {
		t.Errorf("text left: %q", cleared)
	}
	for _, keep := range []string{`\lang1031`, `{\cs15 }`, `{\*\bkmkstart k}`} {
		if !strings.Contains(cleared, keep) {
			t.Errorf("%q lost from %q", keep, cleared)
		}
	}
	out := src[:c.Start] + cleared + `{\uc1 ` + Escape("Neu {\u00f6}\n\U0001f600") + `}` + src[c.End:]
	d2, err := Read([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := d2.Rows[0].Cells[1].Text(); got != "Neu {\u00f6}\n\U0001f600" {
		t.Errorf("written %q from %q", got, out)
	}
	if got := d2.Rows[0].Cells[0].Text(); got != "A" {
		t.Errorf("first cell %q", got)
	}
}

func TestNotRTF(t *testing.T) {
	if _, err := Read([]byte("<xml/>")); err != ErrNotRTF {
		t.Errorf("err %v", err)
	}
}
