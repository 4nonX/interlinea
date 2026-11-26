# Matches, terms, spelling and checks

How Interlinea scores memory matches, finds terms in a segment, checks
spelling and runs its quality checks, and how fast it does so. The file
formats are in [formats.md](formats.md).

## Match scores

100 means identical text and identical tag structure (tag IDs
may differ). Below that the score is an edit distance over words, numbers,
punctuation and tags, where a difference costs by its degree: another case
or other marks on the same word (an accent left out, Arabic with or
without its short vowels) a fraction of a word; another form of a word
(an ending, a suffix, a Korean particle) by the share of its letters that
differ; another number a quarter of a word, and the same number in other
digits next to nothing; punctuation of the same kind in another form
(curly for straight quotes, a full-width comma) a fraction of a mark;
another word a whole word; tags half. Chinese, Japanese, Thai, Lao, Khmer
and Burmese are compared by character. A short segment is also found by
the beginnings of its words, so a heading in the plural finds its
singular. The score is tested on a sample of every listed language.

## Term recognition

Term recognition ignores case and tolerates inflection without a
dictionary: endings added to a term's last word (Kaution, Kautionen;
Mieter, Mieters; deposit, deposits), German umlaut plurals (Vertrag,
Verträge) and German adjective endings before the noun (ordentliche
Kündigung, der ordentlichen Kündigung). Endings are only added, never
swapped, so Miete does not match Mieter and Kauf does not match Käufer.
Irregular forms, changed stems and terms inside compounds are not found.
Korean allows particles after a term; Chinese, Japanese and Thai match as
substrings. Where matches overlap the longest wins; a term in several
termbases is shown once per termbase. Terms are looked up in the exact
language variant first, then any variant of the language. The check
reports a recognized term with no allowed translation in the target
(inflected target forms count) and every forbidden term in the target.

## Segmentation

SRX rules use Go's RE2 engine: Unicode classes work, look-around and
back-references do not and are reported as errors when a rule file is loaded.

## Spelling

The `spell` package follows Hunspell 1.7.2. On 6,000 generated
words per language (dictionary words with endings, compounds, typing
errors, capitalizations) it agrees with Hunspell on every word for English
(US and GB), French, Spanish, Italian and Brazilian Portuguese; it differs on
3 of 10,000 German words, 2 of 6,000 Dutch and 2 of 6,000 Korean words, and
on 30 of 6,000 Hungarian words, where Hunspell has rules of its own for
Hungarian compounds and hyphenation that are not implemented. Its first
suggestion is Hunspell's first in 95 to 97 of 100 misspellings for English,
German, Spanish and Italian and 86 for French; the lists differ more often
in order and length. A suggestion takes about 0.1 s for German (Hunspell
itself takes longer). Of Hunspell's own test suite 171 of 182 cases pass;
the rest are suggestion lists from Hunspell's development version, the
Hungarian rules and morphological analysis, which `spell` does not do.
PHONE rules (en_GB) are applied in the simplified form of spylls, not with
every option of Hunspell's phonetic code. To repeat the comparison, see the
comments of `spell/diff_test.go` and `spell/conformance_test.go`.

Words of more than 100 characters get no suggestions, and a word with ten
or more dashes is checked as it is, not split at each of them: both would
otherwise take seconds or more.

## Quality checks

The number check reads numbers with the separators of each language
(1.250,00 in German is 1,250.00 in English; de-CH writes 1'250.50), so a
number copied unchanged into the wrong format is reported. Numbers written
as words are not compared.

### Subtitles

Subtitles and captions are checked against the project's limits, which its
settings set under Checks: the characters in a line (42 by default), the
lines of a subtitle (2) and the reading speed in characters per second
(17), as streaming services and most broadcasters ask for adult audiences.
A line ends at a line break of the file (a tag) or one typed in. The
reading speed counts the characters of the lines, spaces among them, over
the time the subtitle is shown. Closed captions (SCC) hold at most 32
characters a line whatever the project says. A subtitle the translator
split has no time of its own and is not checked for speed; two joined are
checked over the time of both. The editor shows the longest line, the lines
and the reading speed under the translation as it is typed, what is over
the limit in red.

## Performance

Measured on a Linux machine with two cores: importing 50,000 memory units
takes about 22 s, a fuzzy lookup against 10,000 units about 20 ms. A
command-line command that opens a database spends about 0.8 s at start-up
compiling the embedded SQLite; the desktop app's core and the team server
pay this once, when they start.
