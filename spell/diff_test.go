package spell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAgainstHunspell compares checking and suggestions with Hunspell on
// real dictionaries. INTERLINEA_SPELL_DIFF names a directory with
// NAME.aff, NAME.dic and NAME.words (one word per line);
// INTERLINEA_HUNSPELL_CHECKER is a program linked with libhunspell that
// reads UTF-8 words from standard input and prints, per word, "1 word"
// or "0 word", then a tab and the suggestions joined by ", " when it is
// run with a third argument (spell/testdata/hunspell-check.cxx). It checks each
// word as a whole; "hunspell -l" would split it at characters that are
// not word characters. Dictionaries and word lists are not part of the
// repository.
func TestAgainstHunspell(t *testing.T) {
	dir, bin := os.Getenv("INTERLINEA_SPELL_DIFF"), os.Getenv("INTERLINEA_HUNSPELL_CHECKER")
	if dir == "" || bin == "" {
		t.Skip("set INTERLINEA_SPELL_DIFF and INTERLINEA_HUNSPELL_CHECKER")
	}
	suggest := os.Getenv("INTERLINEA_SPELL_SUGGEST") != ""
	lists, _ := filepath.Glob(filepath.Join(dir, "*.words"))
	for _, wl := range lists {
		name := strings.TrimSuffix(wl, ".words")
		if only := os.Getenv("INTERLINEA_SPELL_ONLY"); only != "" && filepath.Base(name) != only {
			continue
		}
		affData, err1 := os.ReadFile(name + ".aff")
		dicData, err2 := os.ReadFile(name + ".dic")
		data, err3 := os.ReadFile(wl)
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("%s: missing files", name)
		}
		start := time.Now()
		d, err := Load(affData, dicData)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		loadTime := time.Since(start)
		words := strings.Fields(string(data))
		ref := map[string]bool{}
		refSug := map[string][]string{}
		runChecker := func(ws []string, sug bool) {
			args := []string{name + ".aff", name + ".dic"}
			if sug {
				args = append(args, "-s")
			}
			cmd := exec.Command(bin, args...)
			cmd.Stdin = strings.NewReader(strings.Join(ws, "\n") + "\n")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s: checker: %v", name, err)
			}
			for _, l := range strings.Split(string(out), "\n") {
				if len(l) < 2 || l[0] == '?' {
					continue
				}
				w, sugs, _ := strings.Cut(l[2:], "\t")
				ref[w] = l[0] == '1'
				if sugs != "" {
					refSug[w] = strings.Split(sugs, ", ")
				}
			}
		}
		runChecker(words, false)
		start = time.Now()
		var falseOK, falseBad, misspelled []string
		for _, w := range words {
			want, known := ref[w]
			if !known {
				continue
			}
			ok := d.Spell(w)
			switch {
			case ok && !want:
				falseOK = append(falseOK, w)
			case !ok && want:
				falseBad = append(falseBad, w)
			case !ok:
				misspelled = append(misspelled, w)
			}
		}
		checkTime := time.Since(start)
		t.Logf("%s: %d words, %d entries (load %v, check %v): %d accepted that Hunspell rejects, %d rejected that Hunspell accepts",
			filepath.Base(name), len(words), d.Words(), loadTime.Round(time.Millisecond), checkTime.Round(time.Millisecond), len(falseOK), len(falseBad))
		show := func(label string, ws []string) {
			if len(ws) > 15 {
				ws = ws[:15]
			}
			if len(ws) > 0 {
				t.Logf("  %s: %s", label, strings.Join(ws, " "))
			}
		}
		show("accepted", falseOK)
		show("rejected", falseBad)
		if !suggest {
			continue
		}
		if len(misspelled) > 100 {
			misspelled = misspelled[:100]
		}
		runChecker(misspelled, true)
		same, top1, inRef := 0, 0, 0
		var diffs []string
		start = time.Now()
		for _, w := range misspelled {
			got, want := d.Suggest(w, 15), refSug[w]
			if strings.Join(got, ", ") == strings.Join(want, ", ") {
				same++
			} else if len(diffs) < 8 {
				diffs = append(diffs, w+": "+strings.Join(got, ", ")+" / hunspell: "+strings.Join(want, ", "))
			}
			if (len(got) == 0 && len(want) == 0) || (len(got) > 0 && len(want) > 0 && got[0] == want[0]) {
				top1++
			}
			if len(got) > 0 {
				for _, x := range want {
					if x == got[0] {
						inRef++
						break
					}
				}
			} else if len(want) == 0 {
				inRef++
			}
		}
		t.Logf("  suggestions for %d words (%v): %d identical lists, %d same first suggestion, %d first suggestion among Hunspell's",
			len(misspelled), time.Since(start).Round(time.Millisecond), same, top1, inRef)
		for _, x := range diffs {
			t.Logf("    %s", x)
		}
	}
}
