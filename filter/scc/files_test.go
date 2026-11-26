package scc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// rowsOf is a unit's rows as a viewer shows them: a row at each break,
// a mid-row code as a space, runs of spaces as one.
func rowsOf(u filter.Unit) []string {
	var out []string
	cur := ""
	for _, r := range u.Source {
		if r.Tag != nil {
			if r.Tag.Type == "break" {
				out = append(out, cur)
				cur = ""
			} else {
				cur += " "
			}
			continue
		}
		cur += r.Text
	}
	out = append(out, cur)
	for i := range out {
		out[i] = strings.Join(strings.Fields(strings.ReplaceAll(out[i], "\u00a0", " ")), " ")
	}
	return out
}

// swapCase stands in for a translation: every ASCII letter changes case,
// so each caption has text to write that differs from the source's.
func swapCase(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 128 && unicode.IsUpper(r):
			return unicode.ToLower(r)
		case r < 128 && unicode.IsLower(r):
			return unicode.ToUpper(r)
		}
		return r
	}, s)
}

// TestOtherToolsFiles reads the captions of pycaption's tests (files
// written to trip decoders up: restated addresses, rows past the last
// column, roll-up windows, paint-on, mid-row codes, both channels):
// each is read, written back unchanged without translations, and
// translated, read back with the translated text shown when the source's
// was (within a frame).
func TestOtherToolsFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/*/*.scc")
	if err != nil || len(files) < 100 {
		t.Fatalf("%d files: %v", len(files), err)
	}
	refused := map[string]bool{
		"sample_scc_frame_30.scc":              true, // frame 30 in a 30-frame time code
		"sample_scc_with_unknown_commands.scc": true, // "942x" is no byte pair
	}
	modes := map[int]int{}
	for _, p := range files {
		name := filepath.Base(p)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		ex, err := Extract(data, "en")
		if refused[name] {
			if err == nil {
				t.Errorf("%s is read", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if f, err := parse(data); err == nil {
			for _, c := range f.caps {
				modes[c.mode]++
			}
		}
		same, err := Merge(data, "en", "de", filter.Translations{})
		if err != nil || string(same) != string(data) {
			t.Errorf("%s: without translations: %v", name, err)
		}
		tr := filter.Translations{}
		want := ""
		for _, u := range ex.Units {
			var c content.Content
			for _, r := range u.Source {
				if r.Tag != nil {
					c = c.AppendTag(*r.Tag)
				} else {
					c = c.Append(swapCase(r.Text))
				}
			}
			tr[u.ID] = c
			want += strings.ReplaceAll(c.Plain(), " ", "")
		}
		out, err := Merge(data, "en", "de", tr)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		back, err := Extract(out, "de")
		if err != nil {
			t.Errorf("%s: read back: %v\n%s", name, err, out)
			continue
		}
		got := ""
		for _, u := range back.Units {
			got += strings.ReplaceAll(u.Source.Plain(), " ", "")
		}
		if got != want {
			t.Errorf("%s: read back %q, want %q\n%s", name, got, want, out)
		}
		switch {
		case len(back.Units) == len(ex.Units):
			for i := range ex.Units {
				if d := back.Units[i].Timing.Start - ex.Units[i].Timing.Start; d < -34 || d > 34 {
					t.Errorf("%s: caption %d shows at %d ms, was %d\n%s", name, i+1, back.Units[i].Timing.Start, ex.Units[i].Timing.Start, out)
				}
			}
		case len(back.Units) < len(ex.Units):
			t.Errorf("%s: %d captions read back of %d\n%s", name, len(back.Units), len(ex.Units), out)
		default:
			// Only a roll-up line longer than a row rolls on to the next.
			for _, u := range ex.Units {
				if !strings.HasPrefix(u.Notes[len(u.Notes)-1], "A line of roll-up") {
					t.Errorf("%s: %d captions read back of %d\n%s", name, len(back.Units), len(ex.Units), out)
					break
				}
			}
		}
	}
	if modes[popOn] == 0 || modes[paintOn] == 0 || modes[rollUp] == 0 {
		t.Errorf("captions by mode: %v", modes)
	}
}

// pycaptionRows are rows pycaption's tests expect of its files.
var pycaptionRows = []struct{ file, text string }{
	{"pop_on_line_starting_left_of_the_origin", "SAM:\nThe line below it"},
	{"pop_on_pac_overriding_the_line_start_before_text", "HELLO\nWORLD"},
	{"pac_within_a_tab_offset_of_an_overwrite", "ABCDEFGHWX  YZ"},
	{"pac_within_a_tab_offset_of_a_mid_row_code", "ABCDE XY"},
	{"same_row_pac_near_the_origin_after_text", "A yes"},
	{"same_row_pac_at_the_cursor", "Quyana, Atsaq."},
	{"pac_restating_the_position_after_text", "Quyana Atsaq."},
	{"tab_offset_resolving_behind_the_cursor", "ABWXYZIJ"},
	{"restated_origin_pac_refilling_a_long_line", "YZABCDEFGHIJKLMNOPQRSTUV"},
	{"roll_up_pac_restating_the_row_mid_row", "HE'S YOUNG AND WELL"},
	{"same_row_pac_ahead_of_the_cursor", "Ah  there you are."},
	{"same_row_pac_behind_the_cursor", "ABCDEFGHIJKLMNOPQRSTUVWXYZabWXYZ"},
	{"tab_offset_after_line_break", "AB\n  CDEF"},
	{"line_break_then_repositioning", "AB"},
	{"line_break_then_repositioning", "CD"},
	{"italics_still_open_at_repositioning", "♪ AB ♪"},
	{"extended_char_first_on_a_wrapped_line", "AB\nÁBC"},
	{"extended_char_first_at_a_new_position", "AB"},
	{"extended_char_first_at_a_new_position", "ÁBC"},
	{"doubled_pac_with_tab_offset", "AB"},
	{"blank_columns_abandoned_before_a_full_line", "AB\nABCDEFGHIJKLMNOPQRSTUVWXYZabcde"},
	{"tab_offset_part_way_through_a_line", "AB    CD"},
	{"style_command_between_the_text_and_a_refill_pac", "ABCDEFGHIJKLMNOPQRSTUVWXYZabWXYZ"},
	{"mid_row_code_after_a_pac_behind_the_cursor", "ABCD EF"},
	{"backspace_before_a_pac_ahead_of_the_cursor", "ABCDEFGÁÉÓ WX"},
}

func TestPycaptionReadings(t *testing.T) {
	for _, tc := range pycaptionRows {
		data, err := os.ReadFile("testdata/pycaption/sample_scc_" + tc.file + ".scc")
		if err != nil {
			t.Fatal(err)
		}
		ex, err := Extract(data, "en")
		if err != nil {
			t.Errorf("%s: %v", tc.file, err)
			continue
		}
		have := map[string]bool{}
		var all []string
		for _, u := range ex.Units {
			for _, r := range rowsOf(u) {
				r = strings.ReplaceAll(r, "’", "'")
				have[r] = true
				all = append(all, r)
			}
		}
		for _, w := range strings.Split(tc.text, "\n") {
			w = strings.Join(strings.Fields(strings.ReplaceAll(w, "\u00a0", " ")), " ")
			if !have[w] {
				t.Errorf("%s: row %q not read; rows %q", tc.file, w, all)
			}
		}
	}
}
