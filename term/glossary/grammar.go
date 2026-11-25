package glossary

import (
	"sort"
	"strings"

	"github.com/4nonX/interlinea/term"
)

// What a term's grammar fields say, set as a dictionary sets it: short,
// in italics, after the term ("Kaution f", "kündigen v"), and explained in
// a list of abbreviations at the start.

type grammarKind int

const (
	notGrammar grammarKind = iota
	partOfSpeech
	gender
	number
)

// grammarField tells whether a field (by its name, in English, German,
// French or Spanish, as termbases name them) holds a term's grammar.
func grammarField(name string) grammarKind {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.NewReplacer("_", " ", "-", " ").Replace(n)
	switch n {
	case "part of speech", "partofspeech", "pos", "word class", "wordclass", "grammar", "grammatical category",
		"wortart", "grammatik", "catégorie grammaticale", "categorie grammaticale", "nature", "categoría gramatical", "categoria gramatical":
		return partOfSpeech
	case "gender", "grammatical gender", "genus", "geschlecht", "genre", "género", "genero":
		return gender
	case "number", "grammatical number", "numerus", "nombre", "número", "numero":
		return number
	}
	return notGrammar
}

type abbrev struct {
	short, long string
}

// The values people give, as dictionaries abbreviate them.
var grammarValues = map[grammarKind]map[string]abbrev{
	partOfSpeech: {
		"noun": {"n", "noun"}, "n": {"n", "noun"}, "substantive": {"n", "noun"}, "substantiv": {"n", "noun"}, "nom": {"n", "noun"}, "sustantivo": {"n", "noun"},
		"verb": {"v", "verb"}, "v": {"v", "verb"}, "vb": {"v", "verb"}, "verbe": {"v", "verb"}, "verbo": {"v", "verb"},
		"adjective": {"adj", "adjective"}, "adj": {"adj", "adjective"}, "adjektiv": {"adj", "adjective"}, "adjectif": {"adj", "adjective"}, "adjetivo": {"adj", "adjective"},
		"adverb": {"adv", "adverb"}, "adv": {"adv", "adverb"}, "adverbe": {"adv", "adverb"}, "adverbio": {"adv", "adverb"},
		"proper noun": {"prop n", "proper noun"}, "propernoun": {"prop n", "proper noun"}, "eigenname": {"prop n", "proper noun"}, "nom propre": {"prop n", "proper noun"},
		"pronoun": {"pron", "pronoun"}, "preposition": {"prep", "preposition"}, "conjunction": {"conj", "conjunction"},
		"phrase": {"phr", "phrase"}, "abbreviation": {"abbr", "abbreviation"}, "acronym": {"abbr", "abbreviation"},
	},
	gender: {
		"masculine": {"m", "masculine"}, "m": {"m", "masculine"}, "maskulinum": {"m", "masculine"}, "männlich": {"m", "masculine"}, "masculin": {"m", "masculine"}, "masculino": {"m", "masculine"},
		"feminine": {"f", "feminine"}, "f": {"f", "feminine"}, "femininum": {"f", "feminine"}, "weiblich": {"f", "feminine"}, "féminin": {"f", "feminine"}, "femenino": {"f", "feminine"},
		"neuter": {"nt", "neuter"}, "n": {"nt", "neuter"}, "nt": {"nt", "neuter"}, "neutrum": {"nt", "neuter"}, "sächlich": {"nt", "neuter"}, "neutre": {"nt", "neuter"}, "neutro": {"nt", "neuter"},
		"common": {"c", "common gender"}, "c": {"c", "common gender"}, "utrum": {"c", "common gender"},
		"other": {"", ""},
	},
	number: {
		"plural": {"pl", "plural"}, "pl": {"pl", "plural"}, "pluriel": {"pl", "plural"}, "mehrzahl": {"pl", "plural"},
		"singular": {"", ""}, "sg": {"", ""}, "singulier": {"", ""}, "einzahl": {"", ""},
		"uncountable": {"no pl", "no plural"}, "mass noun": {"no pl", "no plural"},
	},
}

// grammar gives a term's grammar as a dictionary writes it, and the
// abbreviations used.
func (g *Glossary) grammar(vs []term.Value) (string, []abbrev) {
	var pos, gen, num *abbrev
	var other []string
	for _, n := range g.TermFields {
		kind := grammarField(n)
		if kind == notGrammar {
			continue
		}
		v := strings.TrimSpace(Value(vs, n))
		if v == "" {
			continue
		}
		a, known := grammarValues[kind][strings.ToLower(v)]
		if !known {
			// A value without an abbreviation is set as it is.
			other = append(other, v)
			continue
		}
		if a.short == "" {
			continue
		}
		switch kind {
		case partOfSpeech:
			pos = &a
		case gender:
			gen = &a
		case number:
			num = &a
		}
	}
	var parts []string
	var used []abbrev
	// A gender says it is a noun: "f", not "n f".
	if pos != nil && !(gen != nil && pos.short == "n") {
		parts = append(parts, pos.short)
		used = append(used, *pos)
	}
	if gen != nil {
		parts = append(parts, gen.short)
		used = append(used, *gen)
	}
	if num != nil {
		parts = append(parts, num.short)
		used = append(used, *num)
	}
	parts = append(parts, other...)
	return strings.Join(parts, " "), used
}

// Abbreviations lists the grammar abbreviations the glossary uses, sorted,
// as "f feminine".
func (g *Glossary) Abbreviations() []string {
	seen := map[string]string{}
	add := func(vs []term.Value) {
		_, used := g.grammar(vs)
		for _, a := range used {
			seen[a.short] = a.long
		}
	}
	for _, s := range g.Sections {
		for _, a := range s.Articles {
			for _, e := range a.Entries {
				for _, l := range e.Languages {
					for _, t := range l.Terms {
						add(t.Fields)
					}
				}
			}
		}
	}
	var out []string
	for short, long := range seen {
		out = append(out, short+" "+long)
	}
	sort.Strings(out)
	return out
}
