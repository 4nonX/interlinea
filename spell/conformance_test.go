package spell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestHunspellSuite runs Hunspell's own test suite (the tests directory
// of github.com/hunspell/hunspell) when INTERLINEA_HUNSPELL_TESTS points
// to it. The suite is not part of this repository: it is under the
// MPL/GPL/LGPL, and it is a check of conformance, not of this code's own
// behaviour, which the other tests cover.
func TestHunspellSuite(t *testing.T) {
	dir := os.Getenv("INTERLINEA_HUNSPELL_TESTS")
	if dir == "" {
		t.Skip("set INTERLINEA_HUNSPELL_TESTS to Hunspell's tests directory")
	}
	affs, _ := filepath.Glob(filepath.Join(dir, "*.aff"))
	sort.Strings(affs)
	var failed []string
	total, passed := 0, 0
	for _, af := range affs {
		name := strings.TrimSuffix(af, ".aff")
		if only := os.Getenv("INTERLINEA_HUNSPELL_ONLY"); only != "" && filepath.Base(name) != only {
			continue
		}
		start := time.Now()
		defer func(n string) {
			if el := time.Since(start); el > 2*time.Second {
				t.Logf("%s took %v", n, el)
			}
		}(filepath.Base(name))
		affData, err := os.ReadFile(af)
		if err != nil {
			t.Fatal(err)
		}
		dicData, err := os.ReadFile(name + ".dic")
		if err != nil {
			continue
		}
		d, err := Load(affData, dicData)
		if err != nil {
			failed = append(failed, filepath.Base(name)+": load: "+err.Error())
			continue
		}
		readWords := func(ext string) []string {
			data, err := os.ReadFile(name + ext)
			if err != nil {
				return nil
			}
			var out []string
			for _, l := range bytes.Split(data, []byte("\n")) {
				l = bytes.TrimRight(l, "\r")
				l = bytes.TrimPrefix(l, []byte("\xef\xbb\xbf"))
				if len(bytes.TrimSpace(l)) == 0 {
					continue
				}
				out = append(out, string(l))
			}
			return out
		}
		ok := true
		var bad []string
		for _, w := range readWords(".good") {
			if !d.Spell(w) {
				ok = false
				bad = append(bad, "rejects "+w)
			}
		}
		wrong := readWords(".wrong")
		for _, w := range wrong {
			if d.Spell(w) {
				ok = false
				bad = append(bad, "accepts "+w)
			}
		}
		sugs := readWords(".sug")
		if bin := os.Getenv("INTERLINEA_HUNSPELL_BIN"); bin != "" && len(wrong) > 0 {
			// The installed Hunspell is the reference: the .sug files
			// follow Hunspell's development version.
			sugs = hunspellSuggest(t, bin, name, wrong)
		}
		if sugs != nil {
			var got []string
			for _, w := range wrong {
				if s := d.Suggest(w, 15); len(s) > 0 {
					got = append(got, strings.Join(s, ", "))
				}
			}
			if strings.Join(got, "\n") != strings.Join(sugs, "\n") {
				ok = false
				bad = append(bad, "suggestions\n    got:  "+strings.Join(got, " | ")+"\n    want: "+strings.Join(sugs, " | "))
			}
		}
		total++
		if ok {
			passed++
		} else {
			failed = append(failed, filepath.Base(name)+": "+strings.Join(bad, "; "))
		}
	}
	t.Logf("%d of %d Hunspell tests pass", passed, total)
	for _, f := range failed {
		t.Log(f)
	}
}

// hunspellSuggest runs "hunspell -a" and returns its suggestion lines for
// the words, in the .sug format (only words with suggestions).
func hunspellSuggest(t *testing.T, bin, dict string, words []string) []string {
	cmd := exec.Command(bin, "-i", "UTF-8", "-d", dict, "-a")
	cmd.Stdin = strings.NewReader(strings.Join(words, "\n") + "\n")
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("%s: hunspell: %v", dict, err)
		return nil
	}
	var sugs []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "& ") {
			if _, after, ok := strings.Cut(l, ": "); ok {
				sugs = append(sugs, after)
			}
		}
	}
	return sugs
}
