# Interlinea

An open-source translation workspace: CAT editor, terminology, translation
memory and project workflow in one tool, translation first.

This repository holds the Go core. The native desktop client (Qt 6), the
server mode and the browser client will build on it.

## Status

Milestone 1: core foundation, plus the first file filter (Word).

| Package | What it does |
|---|---|
| `content` | Segment model: text interleaved with inline tags (open, close, standalone), JSON encoding, slicing at segment boundaries |
| `langtag` | BCP 47 normalization and matching (`de_de` to `de-DE`, primary-language matches) |
| `segment` | SRX 2.0 parser and segmenter, built-in rules for German, English, CJK and a generic fallback |
| `fuzzy` | Weighted token edit distance and match percentages, aware of tags, case and punctuation |
| `tmx` | Streaming TMX 1.4b reader and writer (UTF-8, UTF-16, ISO-8859-1, Windows-1252), inline markup mapped to tags |
| `tm` | Translation memory model, the `Memory` interface, an in-memory backend, TMX import and export |
| `tm/sqlitetm` | Persistent backend: any number of memories in one SQLite file, term index for fuzzy candidates, FTS5 trigram index for concordance |
| `tm/tmtest` | Conformance suite every `tm.Memory` backend must pass |
| `filter` | What file filters hand over: units with inline tags and the document structure they sit in |
| `filter/docx` | Word filter: extracts body, footnotes, endnotes, headers and footers; rebuilds the file from translations, keeping every byte it did not translate |
| `cmd/interlinea` | Command-line interface to all of the above |
| `cmd/interlinea-wasm` | The filters compiled to WebAssembly for the browser prototype |

The SQLite driver is pure Go (SQLite compiled to WebAssembly), so builds need
no C compiler on any platform.

## Build

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
make wasm     # the core for the browser prototype (prototype/public/core)
make sample   # regenerate the sample Word document and its extraction
```

## Command line

Stored translation memories:

```
interlinea tm create -name Mietrecht -src de-DE -tgt en-GB
interlinea tm import -name Mietrecht memory.tmx
interlinea tm import -name Mietrecht -overwrite -primary other.tmx
interlinea tm list
interlinea tm lookup -name Mietrecht "Der Mieter haftet für Schäden."
interlinea tm concordance -name Mietrecht -target "leased property"
interlinea tm export -name Mietrecht -o mietrecht.tmx
interlinea tm delete -name Mietrecht -yes
```

Memories live in `Interlinea/memories.db` in the user configuration directory
(`%AppData%` on Windows, `~/Library/Application Support` on macOS,
`~/.config` on Linux). `-db` or the `INTERLINEA_DB` environment variable
selects another file.

Other commands:

```
interlinea segment -lang de-DE contract.txt
interlinea segment -lang en -srx my-rules.srx -json notes.txt
interlinea tmx-info memory.tmx
interlinea lookup -tmx memory.tmx -tgt en-GB "Der Mieter haftet für Schäden."
interlinea lookup -tmx memory.tmx -tgt en-GB -min 60 -n 10 -json "..."
interlinea concordance -tmx memory.tmx -tgt en-GB Mietsache
interlinea concordance -tmx memory.tmx -tgt en-GB -target "leased property"
interlinea score "Der Mieter haftet." "Der Vermieter haftet."
interlinea extract -lang de-DE lease.docx
interlinea extract -lang de-DE -json lease.docx > units.json
interlinea merge -lang de-DE -target en-GB -o lease_en-GB.docx lease.docx translations.json
```

`translations.json` maps unit IDs to their translation, either a plain string
or the run array `extract -json` prints (with tags as `{"tag":{"kind":"open","id":"1"}}`).
Units without a translation keep their source text.

`segment` treats blank lines as paragraph boundaries and prints one segment
per line. `lookup` and `concordance` load the TMX file into memory; `-src`
overrides the header's source language and `-primary` also accepts variants
that match by primary language only (`de-AT` for `de-DE`).

## Notes on behaviour

- Word documents: formatting that changes inside a paragraph (bold, italic,
  underline, other run formatting, links) becomes paired tags; tabs, line
  breaks, fields, images, footnote references and bookmarks become
  placeholders that carry the original markup. Formatting of a whole
  paragraph is not tagged, and tags at the paragraph edges (a footnote mark
  at the end, a bookmark at the start) stay out of the units. A sentence
  break inside a tag pair is not used, so tags never cross units. On merge,
  only paragraphs with units are rewritten; styles, numbering, images,
  sections and all other parts are copied byte for byte. Rewritten text gets
  the target language (`w:val`, or `w:eastAsia` / `w:bidi` for the scripts
  Word keeps separately). A translation that leaves a span open has it closed
  at the end of the unit; a placeholder left out of a translation is not
  written. Not yet extracted: text boxes, content controls, charts, SmartArt
  and comments (all kept as they are). Tracked changes in a rewritten
  paragraph are accepted.
- Match scores: 100 means identical text and identical tag structure (tag IDs
  may differ). Case-only and punctuation differences cost little, a changed
  word costs a full token, tags count half.
- SRX rules use Go's RE2 engine: Unicode classes work, look-around and
  back-references do not and are reported as errors when a rule file is loaded.
- TMX `<sub>` content inside native codes is kept as part of the native code,
  not as separately translatable text.
- Import policy: by default an identical source/target pair is updated rather
  than duplicated, and a different translation of the same source is kept as
  a second unit. `-overwrite` replaces the most recent translation instead.
- Performance (measured on a 2-core Linux VM): importing 50,000 units takes
  about 22 s, a fuzzy lookup against 10,000 units about 20 ms. Commands that
  open a database spend about 0.8 s at start-up compiling the embedded SQLite;
  a long-running process (the future desktop app or server) pays this once.

## Prototype

`prototype/` is a clickable React prototype (mock data) of project creation,
termbase setup, the translation editor, terminology management and the
git-like merge review.

```
make wasm            # once, and after changes to the Go filters
cd prototype
npm install
npm run dev
```

Sample documents and a deep link per sample (`#/new/sample/0` to `4`) show
language detection, including the fallback for files without readable text.
Word documents (.docx) you choose go through the core's Word filter, running
in the browser as WebAssembly: the units keep their formatting as tags, and
"Save translated document" writes the .docx with the original layout.

Creating a project opens it in the editor (`#/editor/p-hofmann` for the
sample lease, a real Word file in `public/samples`). It follows the
conventions Trados Studio, memoQ and Phrase share: the best match goes into
an empty segment when it opens, Ctrl+Enter confirms and fills repetitions,
Ctrl+1 to 9 insert numbered results, checks run again on confirm and stay on
the segment as warnings until fixed or ignored, Ctrl+K searches the memory,
and F9 inserts the next tag. Match scores look as in Trados: CM for context
matches, the percentage coloured by band, outlined once edited; Analysis
counts segments and words per band (also shown before a project is
created). Preview shows the translated document with its headings, tables
and footnotes. Terms added while translating show up in the merge review.
Settings holds the editor options and switches between plain wording and git
terms. `PRODUCT.md` and `DESIGN.md` describe the product and the visual
system.

## Design skills

UI and UX work on the desktop and browser clients uses agent skills pinned by
commit in `skills-lock.json`:

- `frontend-design` (anthropics/skills): visual direction and typography
- `impeccable` (pbakaus/impeccable): design critique, polish, accessibility, design systems
- `design-taste-frontend` (Leonxlnx/taste-skill): layout, components, copy
- emilkowalski/skills: motion and interaction (`animate`, `choreograph`,
  `review-animations`, `remove-animations`, `animation-performance`,
  `gestures`, `break-ui` and others)

They are not committed (about 42 MB); restore them with

```
npx skills experimental_install
```

## License

Not chosen yet.
