# Languages

Interlinea works with any language that has a BCP 47 tag (`de-DE`, `ko-KR`,
`sr-Latn-RS`). The languages below are the ones both apps offer by name. The
list lives in one place, `langtag/languages.go`; `go run ./langtag/gen`
writes it into the browser client (`prototype/src/languages.gen.ts`) and the
desktop app (`desktop/src/languagelist.h`), and CI fails when either copy is
out of date. A language outside the list is still accepted: files that name
it open as usual, and the desktop app takes a typed tag.

## What depends on the language

| Part | What it does per language |
|---|---|
| Sentence splitting | Abbreviations of 23 languages; the full stops and question marks of Arabic, Urdu, Hindi, Armenian, Amharic, Burmese, Khmer, Greek, Chinese and Japanese. Thai and Lao write no full stop, so their paragraphs stay whole. |
| Word counts | By words, or by characters for Chinese, Japanese, Thai, Lao, Khmer and Burmese, with the signs written on a character counted with it. |
| Memories | Exact and fuzzy matches and search work the same however a file stores accents (composed or decomposed) and in either case, in every script. Simplified and Traditional Chinese, and Serbian in Latin and Cyrillic letters, are kept apart. |
| Terms | Found in the forms they take in running text: endings, prefixes written together with the word (Arabic, Hebrew), suffix chains (Turkish, Finnish, Hungarian), optional vowel marks. Rules name endings; they do not analyse the language, so irregular forms are missed. |
| Checks | Numbers are read with each language's separators and in each script's digits; sentence-final marks are compared across scripts; French spacing follows France or Canada. |
| Detection | About 70 languages are told apart from a page of text. Close relatives (Croatian, Bosnian and Serbian in Latin letters; Danish and Norwegian; Indonesian and Malay) may be taken for one another, and the apps ask to confirm when unsure. |
| Word files | The translated text is marked with the target language where Word expects it, and a paragraph translated into a right-to-left language is laid out from the right. |
| Editors | Segments in a right-to-left language are shown and typed from the right. |

## How it is checked

`internal/langmatrix` holds a short text, a term with one of its forms and a
number for every listed language, and runs each through all of the parts
above. A language added to the list without a sample fails that test. The
samples say the same in every language and were written for this purpose;
they are not a substitute for a native speaker's review of real documents.

## Adding a language

1. Add it to `langtag/languages.go`, in the order of the English names.
2. Add its sample to `internal/langmatrix/samples_test.go`.
3. Run `go run ./langtag/gen` and `go test ./internal/langmatrix`.
4. Where a check fails, the language needs a rule of its own in the part
   that failed.
