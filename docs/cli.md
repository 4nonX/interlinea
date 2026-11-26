# Command line

`interlinea` does on the command line what the apps do: memories,
termbases, documents, bilingual files, packages, analysis and checks. It
comes with the team server's download, and installs with Go:

```
go install github.com/4nonX/interlinea/cmd/interlinea@latest
```

`interlinea -h` lists the commands, and `interlinea <command> -h` the
options of each.

## Translation memories


```
interlinea tm create -name Mietrecht -src de-DE -tgt en-GB
interlinea tm import -name Mietrecht memory.tmx
interlinea tm import -name Mietrecht -overwrite -primary other.tmx
interlinea tm import -name Mietrecht client.sdltm
interlinea tm import -name Mietrecht wordfast-tm.txt
interlinea tm list
interlinea tm lookup -name Mietrecht "Der Mieter haftet für Schäden."
interlinea tm concordance -name Mietrecht -target "leased property"
interlinea tm export -name Mietrecht -o mietrecht.tmx
interlinea tm export -name Mietrecht -format wordfast -o mietrecht.txt
interlinea tm delete -name Mietrecht -yes
```

Memories live in `Interlinea/memories.db` in the user configuration directory
(`%AppData%` on Windows, `~/Library/Application Support` on macOS,
`~/.config` on Linux). `-db` or the `INTERLINEA_DB` environment variable
selects another file.

## Termbases

```
interlinea termbase create -name "Kanzlei Weber" -langs de-DE,en-GB
interlinea termbase import -name "Kanzlei Weber" -dry-run client.tbx
interlinea termbase import -name "Kanzlei Weber" -match term -on merge client.tbx
interlinea termbase import -name "Glossar" -create -mapping mapping.json glossary.xlsx
interlinea termbase search -name "Kanzlei Weber" -lang de -status approved miet
interlinea termbase lookup -name "Kanzlei Weber" -lang de Kaution
interlinea termbase show -name "Kanzlei Weber" c12
interlinea termbase history -name "Kanzlei Weber" c12
interlinea termbase duplicates -name "Kanzlei Weber" -lang de
interlinea termbase merge -name "Kanzlei Weber" c12 c40
interlinea termbase recognize -name "Projekt,Kanzlei Weber" -src de-DE -tgt en-GB "Die Kaution ..."
interlinea termbase export -name "Kanzlei Weber" weber.tbx
```

Termbases live in `Interlinea/termbases.db` next to the memories
(`-db` or `INTERLINEA_TERMDB` for another file). Every change keeps the
previous version with who made it and why; deleted entries are kept as a
record. Entries keep the IDs of the file they came from when those are
free, otherwise they are numbered c1, c2 and so on, which are valid TBX IDs,
so a TBX export imported back with `-match id` finds them again.

## Documents, segments and matches

```
interlinea segment -lang de-DE contract.txt
interlinea segment -lang en -srx my-rules.srx -json notes.txt
interlinea tmx-info memory.tmx
interlinea lookup -tmx memory.tmx -tgt en-GB "Der Mieter haftet für Schäden."
interlinea lookup -tmx memory.tmx -tgt en-GB -min 60 -n 10 -json "..."
interlinea concordance -tmx memory.tmx -tgt en-GB Mietsache
interlinea concordance -tmx memory.tmx -tgt en-GB -target "leased property"
interlinea score "Der Mieter haftet." "Der Vermieter haftet."
interlinea extract -lang de-DE lease.docx          # also .pptx, .xlsx, .html, .idml
interlinea extract -lang de-DE -json lease.docx > units.json
interlinea merge -lang de-DE -target en-GB -o lease_en-GB.docx lease.docx translations.json
```

`translations.json` maps unit IDs to their translation, either a plain string
or the run array `extract -json` prints (with tags as `{"tag":{"kind":"open","id":"1"}}`).
Units without a translation keep their source text.

`segment` treats blank lines as paragraph boundaries and prints one segment
per line. `lookup` and `concordance` load the memory file (TMX, a Trados
Studio memory or a Wordfast memory) into memory; `-src` overrides the file's
source language and `-primary` also accepts variants that match by primary
language only (`de-AT` for `de-DE`).

## Bilingual files

Bilingual XLIFF files, from Trados, memoQ, Phrase or any XLIFF tool:

```
interlinea xliff info Mietvertrag.docx.sdlxliff
interlinea xliff extract -state new Mietvertrag.docx.sdlxliff
interlinea xliff extract -json Mietvertrag.mqxliff > segments.json
interlinea xliff merge -o Mietvertrag.docx.sdlxliff Mietvertrag.docx.sdlxliff updates.json
```

`updates.json` maps segment IDs to `{"target": ..., "state": ...}` (target as
a plain string or the run array `extract -json` prints) or to a plain string,
which also sets the state to translated.

## Packages

Trados project packages:

```
interlinea package info "Mietvertrag Hofmann.sdlppx"
interlinea package extract -o work "Mietvertrag Hofmann.sdlppx"
# translate work/en-GB/*.sdlxliff, then:
interlinea package return -o "Mietvertrag Hofmann.sdlrpx" "Mietvertrag Hofmann.sdlppx" work
```

memoQ handoff packages:

```
interlinea package info Hofmann.mqout
interlinea package extract -o work Hofmann.mqout
interlinea package return -o Hofmann.mqback Hofmann.mqout work
```

## Analysis and word counts

The basis for quotes:

```
interlinea analyze -src de-DE Mietvertrag.docx                          # word count only
interlinea analyze -src de-DE -tgt en-GB -tmx client.tmx -tm Mietrecht \
    -weights context=0,repetition=10,cross-file=10,100=30,95=60,85=60,75=100 \
    -format html -o analysis.html Mietvertrag.docx Anlage.docx
interlinea analyze -context both work/*.sdlxliff                       # memoQ's context rule
```

Bands are `locked`, `context`, `repetition`, `cross-file`, `100`, `95`,
`85`, `75`, `50` and `new`. A band without a weight counts fully, except
locked segments (0). Context matches need memory units that record their
neighbouring segments, as memoQ's TMX does (`x-context-pre`,
`x-context-post`); Trados stores a hash of the context that other tools
cannot read, so Trados memories give 100% matches instead.

## Quality checks and spelling

The exit code is 1 when errors are found, so a script or CI job can stop:

```
interlinea qa -checks                                    # what is checked, on or off by default
interlinea qa -dict ~/dictionaries/de_DE_frami.oxt -termbase Mietrecht \
    -rules our-rules.json -format html -o qa.html Mietvertrag.docx.sdlxliff
interlinea qa -disable capitalization -enable length-ratio,inconsistent-source work/*.sdlxliff
interlinea spell -dict /usr/share/hunspell/de_DE "Kaution Mietvertrg"
```

`our-rules.json` is a list of rules: `{"name": "Euro sign", "source": "EUR",
"target": "€", "mode": "must"}`; `mode` is `must`, `must-not` or `count` (the
translation matches `target` as often as the source matches `source`), with
an optional `severity` (`error` or `warning`) and `message`. `-words` names a
file of words that are always right (one per line).

## Termbase files

```
interlinea term info glossary.xlsx
interlinea term convert glossary.xlsx glossary.tbx            # TBX 3
interlinea term convert -to tbx2 client.tbx client_v2.tbx
interlinea term convert export.xml export.csv                  # MultiTerm XML to CSV
interlinea term mapping glossary.csv > mapping.json            # edit, then:
interlinea term convert -mapping mapping.json glossary.csv glossary.tbx
interlinea term duplicates -lang de client.tbx
interlinea term recognize -src de-DE -tgt en-GB -target "The operating costs." client.tbx "Die Betriebskosten."
```
