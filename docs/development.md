# Development

How Interlinea is built and tested, and what is where. The repository
holds the Go core (file formats, memories, termbases, checks, analysis),
the team server, the desktop app (Qt 6, C++20) and the browser client
(React and TypeScript, with the core compiled to WebAssembly).

| Folder | What it is |
|---|---|
| `cmd/`, the other Go folders | the core, the team server, the desktop app's core, the command line and the WebAssembly build |
| `desktop/` | the desktop app ([desktop.md](desktop.md)) |
| `web/` | the browser client |
| `docs/` | this documentation; `docs/api.md` and `server/openapi.json` are made from the server's source |

## The Go core

Requires Go 1.24 or newer. No C compiler is needed.

```
go build -o bin/ ./cmd/...
go test ./...
```

Install the command line tool directly:

```
go install github.com/4nonX/interlinea/cmd/interlinea@latest
```

With `make`:

```
make all      # gofmt check, vet, tests, build
make race     # tests with the race detector
make bench    # benchmarks
make cross    # binaries for Linux, macOS and Windows (amd64, arm64)
make wasm     # the core for the browser client (web/public/core)
make dictionaries  # the included spelling dictionaries (web/public/dictionaries)
make sample   # regenerate the sample Word document and its extraction
```

The SQLite driver is pure Go (SQLite compiled to WebAssembly), so builds
need no C compiler on any platform.

The packages:

| Package | What it does |
|---|---|
| `content` | Segment model: text interleaved with inline tags (open, close, standalone), JSON encoding, slicing at segment boundaries |
| `langtag` | BCP 47 normalization and matching (`de_de` to `de-DE`, primary-language matches) |
| `segment` | SRX 2.0 parser and segmenter, built-in rules for German, English, CJK and a generic fallback |
| `fuzzy` | Match percentages: a weighted edit distance over words, numbers, punctuation and tags, aware of case, marks, word forms, the digits and punctuation of every script, and scripts written without spaces |
| `tmx` | Streaming TMX 1.4b reader and writer (UTF-8, UTF-16, ISO-8859-1, Windows-1252), inline markup mapped to tags |
| `tm` | Translation memory model, the `Memory` interface, an in-memory backend, TMX import and export, import of Trados Studio memories (.sdltm), Wordfast memories (.txt) in and out, penalties for less trusted memories |
| `tm/tmdb` | Persistent backend: any number of memories in a SQLite file (desktop) or PostgreSQL (server); term index for fuzzy candidates, trigram index for concordance, unit versions that refuse stale edits, change log for syncing |
| `tm/tmtest` | Conformance suite every `tm.Memory` backend must pass |
| `align` | Alignment of a document with its translation into pairs for a memory: sentence lengths after Gale and Church, numbers as anchors in any notation, paragraphs kept where both documents have the same ones; pairs of one to one, two to one, one to two, two to two, and segments left out |
| `term/termdb` | Persistent termbases: any number in one SQLite file, every version of every entry, edits on stale copies refused, term index for lookups, FTS5 trigram search, imports in one transaction |
| `internal/sqlitedb` | Opens the SQLite stores with shared settings (WAL, busy timeout) and migrates their schema |
| `internal/sqlitefile` | Reads the tables of an SQLite file from its bytes without an SQLite library (for Trados memories, in the browser core too) |
| `internal/jet` | Reads the tables of a Microsoft Access (Jet 4) database, which is what a MultiTerm termbase (.sdltb) is |
| `internal/sqlstore` | What the stores share to run on SQLite and on PostgreSQL: queries written once and rebound, transactions, schema migrations |
| `internal/sqlstore/schemahistory` | Keeps the steps of a store's schema from changing once they exist |
| `internal/pgdriver` | Opens PostgreSQL databases for the server's stores |
| `internal/xmlraw` | Reads XML into a tree that keeps the original bytes of every node, so filters rewrite only what they change |
| `internal/rtf` | Reads RTF as the tables of text bilingual review files are, every token keeping its place in the file |
| `internal/textnorm` | Makes text comparable across composed and decomposed accents and upper and lower case in every script |
| `internal/unzip` | Opens the archives people bring within limits: entries, total unpacked size, size of each entry |
| `internal/langmatrix` | Runs every language the apps list through every part of the core that depends on the language |
| `internal/realworld` | Tests the readers on files Trados, MultiTerm and memoQ wrote |
| `internal/fuzzseed` | Gives the fuzz tests the repository's sample files as their first inputs |
| `filter/docx/docxtest`, `filter/formats/formatstest` | Build small, valid documents of every format for tests and samples |
| `filter` | What file filters hand over: units with inline tags and the document structure they sit in |
| `filter/docx` | Word filter: extracts body, footnotes, endnotes, comments, headers and footers, text boxes, content controls, chart text and SmartArt; rebuilds the file from translations, keeping every byte it did not translate |
| `filter/pptx` | PowerPoint filter: slides in the order of the show with their tables, charts and SmartArt, then each slide's speaker notes (shares the reading of DrawingML with the Word filter) |
| `filter/xlsx` | Excel filter: cells sheet by sheet, a shared text once; comments, text boxes and chart titles; numbers, dates and formulas left alone |
| `filter/html` | Web page filter: runs of text between block elements with inline elements as tags; the title, description and the attributes people read; scripts, styles and translate="no" left alone; any encoding |
| `filter/textfmt` | Software strings and text subtitles: Java properties, JSON, YAML, gettext PO, .NET RESX, SubRip, WebVTT, YouTube SBV, ASS/SSA, MicroDVD and SubViewer. One string is one unit; placeholders (%s, {0}, {{name}}, ${name}) and markup are tags; keys, comments, times and everything outside the strings stay byte for byte; escapes and encoding written as the file had them |
| `filter/ttml` | Timed Text subtitles (TTML, DFXP, iTT, EBU-TT, IMSC): a paragraph of the body is one unit, line breaks and spans are tags, the root's xml:lang is set to the target |
| `filter/ebustl` | EBU STL (Tech 3264), binary broadcast subtitles: ISO 6937 and ISO 8859-5 to 8 code tables, teletext and open subtitle control codes as tags, extension blocks, vertical position and header fields rewritten |
| `filter/scc` | Scenarist closed captions (CEA-608): pop-on, roll-up and paint-on captions read as a decoder keeps the screen, with special and extended characters; translated captions written back in their mode, rows placed and aligned, time codes moved so captions show when they did |
| `filter/xmlfmt` | Any XML (an element that holds text is a paragraph, the elements in it are tags, translate="no" is left alone) and OpenDocument text, spreadsheets and presentations (paragraphs and headings with footnotes and text boxes read as paragraphs of their own; cells of numbers and formulas left alone) |
| `filter/markdown` | Markdown: headings, paragraphs, list items, quotes and table cells, with emphasis, links, images, code in a line and HTML as tags; code blocks, front matter and link definitions left alone |
| `filter/plaintext` | Plain text: each line a paragraph cut into sentences; blank lines, line ends and spaces between sentences kept; UTF-8 or UTF-16 with a byte order mark, written back as it was read |
| `filter/idml` | InDesign IDML filter: stories in page order, paragraphs with character formats and links as tags, tables and footnotes after the paragraph that holds them |
| `filter/pdf` | Reads the text of PDF files, without any other program: the objects of the file (also packed in streams, also with a broken table of objects), fonts with their maps to text, encodings and glyph names, and the text shown on each page with where it stands. Lines, columns and paragraphs are found from the places, a word divided at the end of a line is whole again, and the text is handed on as a Word document, which is what the translation is saved as |
| `filter/para` | What the document filters share: a paragraph's content cut into sentence units, and translations put back with their tags matched and nested |
| `filter/formats` | The formats by file name, and one way to extract and merge any document |
| `filter/xliff` | Bilingual files: XLIFF 1.2, Trados SDLXLIFF, memoQ MQXLIFF, Phrase MXLIFF, XLIFF 2.0 to 2.2, Wordfast Pro TXML, Trados TagEditor TTX, and the review tables of Trados (DOCX), memoQ (RTF, DOCX) and Déjà Vu (RTF); segments with targets, states, locks, match values and notes; writes translations and states back, changing only those bytes |
| `filter/sdlpackage` | Trados project packages (.sdlppx): languages and bilingual files; return packages (.sdlrpx) with the translated files |
| `filter/mqpackage` | memoQ handoff packages (.mqout) and memoQ XLIFF archives (.mqxlz): the documents as memoQ XLIFF; delivery packages (.mqback) |
| `term` | Termbase model (concept entries, languages, terms, statuses, fields at entry, language and term level); reads and writes TBX 3, TBX 2, MultiTerm XML, CSV, TSV and Excel, and reads MultiTerm termbase files (.sdltb); import planning (merge, overwrite, skip, add), duplicate detection, term recognition and terminology checks |
| `term/glossary` | A termbase as a glossary to read and print: a dictionary sorted by one language or a table with a column per language, as PDF, Word, a web page or plain text |
| `typeset` | Lays out text in pages and writes PDF with the fonts embedded (only the glyphs used): several scripts in one document, joining scripts such as Arabic, right-to-left text, line breaking |
| `typeset/sfnt` | Reads TrueType and OpenType fonts as far as a PDF needs them: glyphs, widths, metrics and names, substitutions for joining scripts, subsets to embed |
| `typeset/fonts` | The fonts documents are typeset in: Noto Sans (Latin, Greek, Cyrillic), Noto Sans Arabic and Hebrew, under the SIL Open Font License; other scripts take installed fonts |
| `analysis` | Word counts and analysis against memories as in Trados and memoQ: locked, context matches (101%, segment before or before and after), repetitions within and across files, 100%, fuzzy bands, new; segments, words, characters, tags and placeables per file and in total; weighted word counts for quotes; reports in HTML, CSV and JSON |
| `qa` | Quality checks as in Trados, memoQ and Xbench: empty and untranslated segments, tags (missing, extra, nesting), numbers read with each language's separators, end punctuation, spacing (double, leading and trailing, before punctuation, French no-break spaces), brackets, repeated words, capitalization, length limits and ratio, consistency across segments, terminology, spelling, rules of your own (regular expressions); reports in HTML, CSV and JSON |
| `spell` | Hunspell spelling in Go: reads LibreOffice, Firefox and system dictionaries (.aff/.dic, .oxt extensions), affixes, compounds, all flag types and character sets, suggestions in Hunspell's order |
| `project` | Project setups: languages, translatable and reference files, memories per target language (lookup, concordance, update, penalty), terminology (project termbase over masters, directly into a master, own), context rule, QA switches, preparation; validation that names the field of every problem; templates; pre-translation from memories as the analysis counts matches |
| `project/projectdb` | Persistent projects in a SQLite file or PostgreSQL: settings, files with their bytes, segments per file and target language with versions that refuse stale saves, target languages added or removed later, pre-translation applied safely, templates, assignments of files, languages or segment ranges to translators and reviewers with due dates (one of each per segment), who has which segment open, change log for syncing |
| `auth/authdb` | Accounts with Argon2id passwords, OpenID Connect identities, sign-in tokens (only their hashes stored), roles as sets of permissions (read, suggest, edit, approve, manage: five built-in roles, each a permission and the ones before it, and roles administrators define) on projects, memories, termbases or everything; the last administrator is kept; change log; SQLite or PostgreSQL |
| `auth/oidc` | OpenID Connect sign-in (Entra ID, Google, Keycloak and others): discovery, authorization code with PKCE, ID token checks (RS256, PS256, ES256 against the provider's keys, issuer, audience, expiry, nonce) |
| `server` | The team server's HTTP API: sign-in with passwords or an identity provider, accounts and roles, projects with their files, segments, assignments and presence, memories, termbases with their review workflow, a live feed of changes, and an MCP endpoint for assistants ([integration.md](integration.md)) |
| `cmd/interlinea-server` | The team server: SQLite files or PostgreSQL, the web app at `/`, TLS or a reverse proxy |
| `cmd/interlinea-core` | The desktop app's core: the same API on the person's own computer, on the loopback interface, with one account and a token handed to the app at start ([desktop.md](desktop.md)) |
| `langdetect` | Guesses a document's language: scripts by their share of the letters, Latin-script languages by function words; says when the text is too short or mixed to tell |
| `cmd/interlinea-apidoc` | Writes the API reference (`server/openapi.json`, `docs/api.md`) from the server's source |
| `langtag/gen` | Writes the list of languages the apps offer into the browser client and the desktop app |
| `cmd/interlinea` | Command-line interface to all of the above |
| `cmd/interlinea-wasm` | The filters, memories, termbase formats, QA, analysis, project validation and pre-translation compiled to WebAssembly for the browser client |

The SQLite driver is pure Go (SQLite compiled to WebAssembly), so builds need
no C compiler on any platform.

## Desktop app

`desktop/` is the native app. It starts its own core (`interlinea-core`),
which keeps the person's work in SQLite files in the app's data folder, and
connects to team servers; sign-ins are kept in the system's keychain.

```
cmake -S desktop -B build/desktop      # needs Qt 6.8 or later and Go
cmake --build build/desktop
ctest --test-dir build/desktop         # starts the core; QT_QPA_PLATFORM=offscreen is set
```

The build puts `interlinea-core` next to the app, where the app starts it.
`INTERLINEA_CORE` and `INTERLINEA_DATA` point the app at another core or
data folder.

## Browser client

```
make wasm            # once, and after changes to the Go filters
make dictionaries    # once: the spelling dictionaries that come with the app
cd web
npm install
npm run dev
```

It starts empty. Your work is saved in the browser (IndexedDB) as you go:
projects with their files, translations, termbases, memories, templates and
settings are there again on the next visit. Settings shows whether saving
works in this browser and can delete everything kept there; `?reset` in the
address does the same on load.

To look around before bringing your own files, add `?sample` to the
address: it adds a sample project (a tenancy agreement of an invented law
firm, a real Word file in `web/samples`) with its master termbase, a
project termbase that has changes to review and merge, and a translation
memory. What you already have stays.

Browser tests drive the production build in headless Chrome (found through
`$CHROME` or on the PATH) and run in CI:

```
npm run build
npm run e2e
```

With `INTERLINEA_SERVER` set to a built `interlinea-server`, they also start
the team server with the build as its web app and test working on it (CI
always does).

## Before a change goes in

CI runs all of this on every push; run what a change touches first:

```
gofmt -l .                       # prints nothing
go vet ./...
go run ./langtag/gen -check      # the language tables are up to date
go run ./i18n/gen -check         # every interface string has its German and Korean
go run ./cmd/interlinea-apidoc   # docs/api.md and server/openapi.json
go test ./...
cd web && npx tsc --noEmit -p . && npm run build && npm run e2e
ctest --test-dir build/desktop
```

The PostgreSQL tests run with a database to use:

```
INTERLINEA_TEST_POSTGRES='postgres://user@127.0.0.1:5432/interlinea?sslmode=disable' \
  go test -tags postgres ./term/termdb ./tm/tmdb ./project/projectdb ./auth/authdb ./server
```

The desktop tests open every page at several window sizes, and run once
more at a scale of 1.5 (the `_150` tests). They fail on any text cut off,
button too narrow for its label or part reaching out of its place;
`INTERLINEA_LAYOUT_CHECK=1` marks such problems in the running app.

## The interface in other languages

Both apps are written in English and shown in English, German or Korean,
as the person chooses under Settings (by default the language of the
browser or the system). The translations are in `i18n/de.json` and
`i18n/ko.json`, one catalog per language shared by the two apps: each
string is looked up by its English text.

- In the browser client a string is marked with `tl("…")`, or `tlx("…")`
  where a placeholder holds a link or other markup (`src/i18n.ts`).
  Placeholders are named: `tl("Saved {name}", { name })`. Counts go
  through `plural(n, "file", "files")`, looked up as "{n} file" and
  "{n} files", so a language puts the number where it belongs. Dates and
  numbers go through `fmtDate`, `fmtNum`, `fmtDec` and `listOf`, which
  follow the interface language.
- In the desktop app strings are marked with `tr()` as usual in Qt; a
  catalog translator (`src/interfacelanguage.cpp`) looks them up.
- A sentence is one string, never put together from pieces, so it can be
  translated as a whole. `-check` refuses a string that starts or ends in
  the middle of a sentence (", due", "; {n} skipped") or has a placeholder
  that does not say what goes in it (`{x}`).
- The core's texts the apps show (the checks and their messages, the
  rows of an analysis, how a tag of a file is described, the history of
  a termbase on the server) stay English in the core and in the files it
  writes. They are marked with `i18n.N("…")`, and a part that changes is
  a named placeholder filled with `i18n.Fill`: `i18n.Fill(i18n.N("Font
  {value}"), "value", name)`. The catalogs list them like the apps'
  strings. The browser client shows them through `tlCore()`, which
  recognizes a filled text by its marked one, translates the parts that
  are not in quotes and writes numbers as the interface language does.
- Key names in shortcuts go through `keyName("Ctrl+Enter")`: German
  keyboards print Strg, Umschalt, Einfg and Entf.

After changing text, `go run ./i18n/gen` adds the new strings to both
catalogs, with no translation, and takes out those gone; translate what it
added. `go run ./i18n/gen -todo de` lists what German still lacks, and
`-check` (run in CI) fails on a missing translation or a translation
without the placeholders of its string. `tst_interfacelanguage` shows every
desktop page in German and Korean through the layout check, and the browser
test `e2e/languages.mjs` opens every page of the browser client and each of
its tabs in German and Korean and fails on a word left in English or a word
glued together from a translated word and an English ending.

## Fuzzing

Every reader of files people bring has a fuzz test: bilingual files,
documents in every format, PDF, memories, termbases, project and handoff
packages, spelling dictionaries and RTF. `go test` runs them on the
repository's sample files. To go on with inputs of the fuzzer's own:

```
go test -run '^$' -fuzz '^FuzzExtract$' -fuzztime 30m ./filter/xliff
go test -run '^$' -fuzz '^FuzzExtract$' -fuzztime 30m ./filter/formats
go test -run '^$' -fuzz '^FuzzToWord$' -fuzztime 30m ./filter/pdf
go test -run '^$' -fuzz '^FuzzReadFile$' -fuzztime 30m ./tm
go test -run '^$' -fuzz '^FuzzRead$' -fuzztime 30m ./term
go test -run '^$' -fuzz '^FuzzOpen$' -fuzztime 30m ./filter/sdlpackage
go test -run '^$' -fuzz '^FuzzOpen$' -fuzztime 30m ./filter/mqpackage
go test -run '^$' -fuzz '^FuzzLoad$' -fuzztime 30m ./spell
go test -run '^$' -fuzz '^FuzzReadArchive$' -fuzztime 30m ./spell
go test -run '^$' -fuzz '^FuzzTokenize$' -fuzztime 30m ./internal/rtf
```

An input that fails is written to the package's `testdata/fuzz`; commit
it with the fix, and every `go test` runs it from then on. An input that
takes longer than ten seconds counts as failing too.

## Screenshots

The screenshots in `docs/screenshots` are taken from the builds, so they
can be taken again after a change:

- the browser client's (`editor.png`, `terminology.png`,
  `glossary-export.png`, `merge-review.png`, `projects.png`,
  `memories.png`): `cd web && npm run build && npm run screenshots`, on
  the sample project;
- the desktop app's (`desktop-*.png`): the desktop tests save every page
  they reach to `INTERLINEA_SCREENSHOTS=<folder>`, with
  `INTERLINEA_THEME=dark` for the dark variant;
- the glossary page (`glossary-page.png`):
  `INTERLINEA_GLOSSARY_OUT=<folder> go test -run TestSamplePages ./term/glossary`
  writes sample glossaries, and `pdftoppm -r 110 -png -f 1 -l 1` makes
  the picture of the first page;
- the two the README sets side by side are cut from these:
  `convert glossary-export.png -crop 920x726+255+87 +repage -bordercolor '#d0d7de' -border 1 glossary-dialog.png`
  and
  `convert glossary-page.png -crop 910x655+0+0 +repage -bordercolor '#d0d7de' -border 1 glossary-page-top.png`.
