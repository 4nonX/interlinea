package langdetect

// stopwords are frequent function words of each language written with
// spaces. Running text in a language is full of them; they rarely occur
// together in another.
var stopwords = map[string][]string{
	"de-DE": {"der", "die", "das", "und", "ist", "nicht", "mit", "von", "dem", "den", "des", "ein", "eine", "zu", "für", "auf", "im", "bei", "nach", "sich", "wird", "werden", "oder", "auch", "über"},
	"en-GB": {"the", "and", "of", "to", "in", "is", "for", "on", "with", "by", "this", "that", "shall", "be", "or", "any", "may", "not", "are", "as", "an", "each", "between", "which"},
	"fr-FR": {"le", "la", "les", "et", "de", "des", "du", "un", "une", "est", "à", "en", "il", "pour", "dans", "par", "sur", "au", "aux", "qui", "que", "ne", "pas", "ce", "cette", "ces", "son", "sa", "avec", "sont", "être", "doit", "entre", "présent"},
	"es-ES": {"el", "los", "las", "y", "del", "que", "por", "con", "para", "una", "es", "se", "al", "su", "como", "entre", "este", "está"},
	"it-IT": {"lo", "gli", "e", "di", "del", "della", "che", "per", "con", "un", "una", "è", "non", "sono", "tra", "nel", "alla", "questo", "essere"},
	"nl-NL": {"de", "het", "een", "en", "van", "in", "is", "op", "te", "dat", "met", "voor", "niet", "zijn", "door", "aan", "bij", "wordt", "tussen"},
	"pl-PL": {"i", "w", "z", "na", "się", "do", "nie", "że", "jest", "to", "przez", "od", "dla", "lub", "oraz", "jego", "po", "być"},
}
