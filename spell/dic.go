package spell

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
)

type entry struct {
	stem  string
	flags flagSet
	cap   capType
	// alt are spellings from "ph:" fields: common misspellings that
	// lead to this word in suggestions.
	alt []string
}

// Dictionary is a loaded Hunspell dictionary. It is safe for concurrent
// use once loaded.
type Dictionary struct {
	a     *aff
	words map[string][]*entry
	lower map[string][]*entry
	list  []*entry
	// ngram are the entries n-gram suggestions consider; lowered their
	// stems in lower case, made on the first suggestion.
	ngram     []*entry
	lowerOnce sync.Once
	lowered   [][]rune
}

func (d *Dictionary) loweredStems() [][]rune {
	d.lowerOnce.Do(func() {
		d.lowered = make([][]rune, len(d.ngram))
		for i, e := range d.ngram {
			d.lowered[i] = []rune(d.a.lowerOne(e.stem))
		}
	})
	return d.lowered
}

// Load reads a dictionary from the contents of its .aff and .dic files.
func Load(affData, dicData []byte) (*Dictionary, error) {
	a, err := parseAff(affData)
	if err != nil {
		return nil, err
	}
	d := &Dictionary{a: a, words: map[string][]*entry{}, lower: map[string][]*entry{}}
	lines := splitLines(dicData)
	for i, raw := range lines {
		if i == 0 && isCountLine(raw) {
			continue
		}
		if err := d.addLine(raw, i+1); err != nil {
			return nil, err
		}
	}
	bad := flagSet{a.forbidden, a.nosuggest, a.onlyInCompound}
	for _, e := range d.list {
		skip := false
		for _, f := range bad {
			if e.flags.has(f) {
				skip = true
			}
		}
		if !skip {
			d.ngram = append(d.ngram, e)
		}
	}
	return d, nil
}

func isCountLine(b []byte) bool {
	f := fieldsOf(b)
	return len(f) > 0 && isDigits(f[0])
}

func (d *Dictionary) addLine(raw []byte, no int) error {
	a := d.a
	// The word and its flags end at a tab, or at a space before a
	// morphological field ("po:noun"); a word may contain spaces.
	end := len(raw)
	if t := bytes.IndexByte(raw, '\t'); t >= 0 {
		end = t
	}
	for k := 0; k+3 < end; k++ {
		if raw[k] == ' ' && isFieldName(raw[k+1:]) {
			end = k
			break
		}
	}
	head := bytes.TrimRight(raw[:end], " ")
	morph := raw[end:]
	if len(head) == 0 {
		return nil
	}
	// A slash at the start is part of the word ("/usr").
	word, flags := head, []byte(nil)
	for k := 1; k < len(head); k++ {
		if head[k] == '/' && head[k-1] != '\\' {
			word, flags = head[:k], head[k+1:]
			break
		}
	}
	stem := strings.ReplaceAll(a.decode(word), `\/`, "/")
	stem = a.stripIgnored(stem)
	if stem == "" {
		return nil
	}
	fs, err := a.parseFlags(flags)
	if err != nil {
		return fmt.Errorf("spell: .dic line %d: %v", no, err)
	}
	e := &entry{stem: stem, flags: fs, cap: a.guess(stem)}
	for _, m := range fieldsOf(morph) {
		if !bytes.HasPrefix(m, []byte("ph:")) || len(m) <= 3 {
			continue
		}
		p := a.decode(m[3:])
		switch {
		case strings.HasSuffix(p, "*") && len(p) > 1:
			// "ph:prity*" for "pretty": the stem without its last
			// letter replaces the pattern without its last two.
			pr := []rune(p)
			sr := []rune(stem)
			if len(pr) >= 2 && len(sr) >= 1 {
				a.rep = append(a.rep, repPattern{from: string(pr[:len(pr)-2]), to: string(sr[:len(sr)-1])})
			}
		case strings.Contains(p, "->"):
			from, to, _ := strings.Cut(p, "->")
			if from != "" {
				a.rep = append(a.rep, repPattern{from: from, to: to})
			}
		default:
			a.rep = append(a.rep, repPattern{from: p, to: stem})
			e.alt = append(e.alt, p)
		}
	}
	d.words[stem] = append(d.words[stem], e)
	if e.cap != capNo {
		for _, l := range a.lower(stem) {
			d.lower[l] = append(d.lower[l], e)
		}
	} else {
		d.lower[stem] = append(d.lower[stem], e)
	}
	d.list = append(d.list, e)
	return nil
}

func isFieldName(b []byte) bool {
	isLetter := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
	return len(b) >= 3 && isLetter(b[0]) && isLetter(b[1]) && b[2] == ':'
}

// Words is the number of entries in the dictionary.
func (d *Dictionary) Words() int { return len(d.list) }

// Lang is the language the .aff file names (LANG), if any.
func (d *Dictionary) Lang() string { return d.a.lang }

// WordChars lists the characters the dictionary counts as part of a word
// besides letters (WORDCHARS), for tokenizing text.
func (d *Dictionary) WordChars() string { return d.a.wordchars }

// Add adds a word to the dictionary without affixes, as a user
// dictionary does. Add is not safe for use while other goroutines check
// words.
func (d *Dictionary) Add(word string) {
	word = strings.TrimSpace(word)
	if word == "" {
		return
	}
	e := &entry{stem: word, cap: d.a.guess(word)}
	d.words[word] = append(d.words[word], e)
	for _, l := range d.a.lower(word) {
		d.lower[l] = append(d.lower[l], e)
	}
	d.list = append(d.list, e)
	d.ngram = append(d.ngram, e)
	if d.lowered != nil {
		d.lowered = append(d.lowered, []rune(d.a.lowerOne(word)))
	}
}

func (d *Dictionary) homonyms(stem string) []*entry { return d.words[stem] }

func (d *Dictionary) hasFlag(stem string, f flag, all bool) bool {
	if f == 0 {
		return false
	}
	hs := d.words[stem]
	if len(hs) == 0 {
		return false
	}
	for _, h := range hs {
		if h.flags.has(f) != all {
			return !all
		}
	}
	return all
}
