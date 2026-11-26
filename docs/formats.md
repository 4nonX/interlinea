# File formats

What Interlinea reads and writes, and how. Every format is read and
written the same way in the desktop app, the browser client, on the team
server and on the command line: the same Go code does it everywhere.

| Kind | Formats |
|---|---|
| Documents | Word (.docx), PowerPoint (.pptx), Excel (.xlsx), OpenDocument (.odt, .ods, .odp), web pages (.html), InDesign (.idml), XML, Markdown, PDF (the text), plain text |
| Software strings and subtitles | Java properties, JSON, YAML, gettext PO, .NET RESX, SubRip (.srt), WebVTT |
| Bilingual files | XLIFF 1.2 and 2.0 to 2.2, SDLXLIFF, MQXLIFF (also as .mqxlz), Phrase .mxliff, Wordfast Pro .txml and .txlf, TagEditor .ttx |
| Bilingual tables for review | Trados Studio review documents (.docx), memoQ two-column tables (.rtf, .docx), Déjà Vu external views (.rtf) |
| Packages | Trados project packages (.sdlppx, returned as .sdlrpx), memoQ handoff packages (.mqout, returned as .mqback) |
| Translation memories | TMX 1.4, Trados Studio memories (.sdltm, read), Wordfast memories (.txt, read and written) |
| Termbases | TBX 3 and TBX 2, MultiTerm XML, MultiTerm termbases (.sdltb, read), CSV, TSV, Excel |
| Glossaries | PDF, Word, a web page with search, plain text ([how they look](guide.md#glossaries)) |

A translated document keeps every part of the file that was not
translated byte for byte; a bilingual file gets only its changed targets
and states written. How matches, terms and checks work is in
[matching.md](matching.md).

## Documents

### Word

Formatting that changes inside a paragraph (bold, italic,
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
written. Text boxes (with their compatibility fallback), content controls,
comments, chart titles and labels (and the chart's embedded workbook) and
SmartArt text (and its drawing cache) are extracted too. Tags say what they
are (bold, colour, font, size, style, link and so on). Tracked changes in
a rewritten paragraph are accepted, with a warning; `-keep-revisions`
keeps them as tags instead.

### PowerPoint

Slides in the order of the show, each followed by its
charts, SmartArt and speaker notes; tables cell by cell. Slide masters
and layouts are not read (their prompts do not show in a show), comments
are kept as they are. A paragraph translated into a language of the
other direction is turned around, with its alignment.

### Excel

The text of cells, sheet by sheet along the rows; a text several
cells share is one segment. Comments, text boxes and chart titles
follow the cells of their sheet. Numbers, dates and formulas are left
alone, and so are sheet names, which formulas and charts refer to. A
workbook translated into a language of the other direction has its
sheets turned.

### Web pages

A run of text between block elements is a block; inline
elements are tags; the title, the description and keywords, and alt,
title, placeholder, aria-label and button labels are blocks of their
own. Scripts, styles, `translate="no"`, svg, frames and embedded
objects are left alone. The page is written back in its own encoding
(a character it does not have becomes a numeric reference), with the
target's `lang` and `dir` on the html element. Markup that is not well
formed is kept as it is.

### InDesign

Stories in the order of their frames on the pages; a
paragraph is a block; other character formats and links are pairs,
forced line breaks, page numbers, anchored objects, tables and footnote
references placeholders; cells and footnotes follow the paragraph that
holds them. Notes and tracked changes are kept as they are; a paragraph
whose text sits in XML structure tags is kept, with a warning. The
language InDesign checks spelling in is not changed; for Arabic and
Hebrew the paragraph direction is set, and the World-Ready composer has
to be chosen in InDesign. Only IDML is read: save an .indd as IDML in
InDesign first.

### PDF

Only the text is read. A PDF says where each piece of text stands
on a page and nothing of paragraphs, so those are found from the
places: lines from baselines, columns and table columns from the white
that runs between them, paragraphs from spacing, short lines, indents
and list marks. Columns are read one after the other; of a table each
cell is a paragraph, column by column. Running heads and page numbers
are text like any other. A paragraph is in the size of its font, and
bold or italic where all of it is; formatting within a paragraph,
pictures and the layout of the pages are not kept. The translation is
saved as a Word document, since a page cannot be rebuilt around text
of another length. A PDF that is protected, one that is pictures of
its pages (a scan without text recognition), and one whose fonts do not
say which characters they draw are not read, and say so.

### OpenDocument, XML and Markdown

Any XML file is read: an element that holds text is a paragraph, the
elements in it are tags, and `translate="no"` is left alone. OpenDocument
text, spreadsheets and presentations are read as paragraphs and headings,
with footnotes and text boxes as paragraphs of their own; cells of numbers
and formulas are left alone.

Markdown: headings, paragraphs, list items, quotes and table cells are
read, with emphasis, links, images, code in a line and HTML as tags; code
blocks, front matter and link definitions are left alone.

### Software strings and subtitles

Java properties, JSON, YAML, gettext PO, .NET RESX, SubRip and WebVTT:
one string is one unit. Placeholders (`%s`, `{0}`, `{{name}}`, `${name}`)
and markup are tags; keys, comments, times and everything outside the
strings stay byte for byte, and escapes and the encoding are written as
the file had them.

## Bilingual files

### XLIFF, SDLXLIFF and MQXLIFF

Segments keep their Trados, memoQ or XLIFF state, mapped to
Interlinea's new, draft, translated, reviewed, final and rejected (Trados
Not Translated, Draft, Translated, Approved Translation, Approved
Sign-off, Rejected; memoQ Not started, Pre-translated or Edited,
Confirmed, Reviewer 1 confirmed, Proofread, Rejected; XLIFF 1.2 and 2
`state`). Merge writes only the target of each changed segment and its
state: the Trados confirmation level in `sdl:seg-defs` (and origin
`interactive` for edited text), the memoQ `mq:status`, or the XLIFF
`state` (XLIFF 2 drafts and rejections as `subState`
`interlinea:draft` and `interlinea:rejected`). Locked segments (Trados
and memoQ locks) cannot change. Trados tracked changes in a segment stay
as tags; their deleted text is not translatable. Files in UTF-8 or UTF-16
are written back in the same encoding. Not read yet: `alt-trans`
suggestions in XLIFF 1.2 and the XLIFF 2 matches module.

### Phrase

Phrase (Memsource) files (.mxliff): Phrase's codes in the text (`{1}`,
`{1>` and `<1}`, `{b>` and the like) are tags, with what each stands for
as Phrase describes it; a segment is confirmed (`m:confirmed`) once
translated. In a file for review (`m:level` 2 and on) confirming is the
reviewer's: a translation not yet confirmed there is a translation, a
confirmed one reviewed. Locks and match values come from the file.

### Wordfast Pro

Wordfast Pro files (.txml): the codes (`<ut>`) are tags; a segment is a
draft while `unconfirmed="true"`, and from machine translation when
`gtmt="true"`. Wordfast Pro 5 files (.txlf) are XLIFF and read as such.

### TagEditor

Trados TagEditor files (.ttx), from Trados 2007 and earlier: each `<Tu>`
is a segment. Text not yet segmented in TagEditor is cut into sentences
as Interlinea cuts documents, and each sentence translated is written
back as a `<Tu>` of its own, as TagEditor writes it, with the codes that
close a paragraph in its last sentence. A sentence is not cut through
formatting TagEditor shows (`<df>`): where such formatting holds a
paragraph break, the sentence ends at it, where TagEditor would split the
formatting instead. A file that was never segmented names no target
language: it is the project's first target language, and the file
delivered names it.

### Bilingual tables for review

Bilingual tables for review in Word and RTF, each segment a table row:

- A Trados Studio review document (Export for bilingual review, .docx):
  the segment number and translation unit in the first cell, the status
  and match ("Draft (77%)"), source and target. Tags are text in the
  character style Tag (`<1>`, `</1>`, `<2/>`), locked content is in the
  style LockedContent, and Word comments in a row are the segment's
  comments. A Word document is read as one by its content, whatever its
  name.
- A memoQ two-column RTF or DOCX: the document and project in the first
  row, the languages by name in the header, then a segment in each row
  with its number and GUID, comment and status ("Pre-translated (99%)").
  memoQ's tags are text in the style mqInternal: `[1]` stands alone,
  `[2}` opens, `{3]` closes; the same code written twice is told apart by
  its place.
- A Déjà Vu RTF external view: the segment ID, source, target and
  comment; Déjà Vu's codes (`{00108}`) are tags.
- The files are read as their program wrote them or as Word saved them
  again. Only the target cells (and the status shown) are written; every
  other byte stays, so the program reads the file back by its segment
  IDs. Tags a reviewer typed without the tag style are the source's tags.
  A file that does not say its languages takes the project's.

Bilingual files of these programs keep their segments as the program
made them: they are not split or joined.

## Packages

### Trados project packages

A return package holds the package's project file with
`PackageType="ReturnPackage"` (every other byte as it was, so Trados
matches it to its project) and the package's SDLXLIFF files, the
translated ones replaced; memories, termbases and settings are not sent
back.

### memoQ handoff packages

The documents of a memoQ handoff package (.mqout) are read as memoQ XLIFF,
out of their archives (.mqxlz) where they are in one; the package's
memories (TMX) and term bases (CSV) stay in it and can be imported from
it by hand. The delivery package (.mqback) holds the documents as memoQ
XLIFF and DeliveryPackageInfo.xml, made from the handoff's
HandoffPackageInfo.xml the way delivery packages are put together by
hand: the HandoffPackageInfo element named HandoffDeliveryPackageInfo,
its UserName named User. memoQ publishes no description of these files,
and the delivery package has not been tried against memoQ: if memoQ
does not take it, save the documents one by one (Save XLIFF) and bring
them in with memoQ's import of bilingual files, which is documented.
A memoQ XLIFF archive on its own is a bilingual file and is saved back
as the archive it was.

## Translation memories

### Trados Studio memories

Trados Studio memories (.sdltm) are read wherever TMX is, in every
client: the units with their tags (paired as Trados pairs them, the same
tag of source and target given the same ID), who made and changed each and
when, how often it was used, and the memory's own fields (text, numbers,
dates and lists) as unit attributes. Trados's own field StructureContext
is left out. A new memory made from the file takes its name and languages.
Changes still in the memory's write-ahead log (a `.sdltm-wal` file beside
it, while Trados has it open) are not seen: close the memory in Trados
first.

### Wordfast memories

Wordfast memories (.txt, UTF-16 or Latin-1) are read with their dates,
users, usage counts and the five fields of one's own; Wordfast's escapes
(`&'92;` for a closing quote) become the characters, and its formatting
codes (`&tA;`) placeholders. A memory is written out as one too, as
Wordfast writes it (UTF-16, Windows line ends): its tags become Wordfast's
codes (a tag that came from Wordfast keeps its own; Wordfast has 52, and
tags beyond them are left out), and the first five fields of its units
that are not Interlinea's or TMX's own (`x-…`) are Wordfast's attribute
fields.

### All memory files

A memory file whose languages are the memory's the other way round is
read the other way round; one of other languages stores nothing, and the
import says so.

TMX `<sub>` content inside native codes is kept as part of the native code,
not as separately translatable text.

Import policy: by default an identical source/target pair is updated rather
than duplicated, and a different translation of the same source is kept as
a second unit. `-overwrite` replaces the most recent translation instead.

## Termbases

An entry is one concept with its terms per language, as in
MultiTerm, memoQ and TBX. Statuses map to TBX `administrativeStatus`
(approved = preferredTerm, proposed = admittedTerm, forbidden =
deprecatedTerm). Fields TBX-Basic has no category for are written as
`<note>[Name] value</note>` and read back as fields. MultiTerm XML is
written as UTF-16 with a byte order mark, as MultiTerm exports it.
Spreadsheets have one row per entry; the column mapping is guessed from
headers such as `German`, `de-DE`, `Status (en)` or `en Term Note`, and
synonyms share a cell (`;`, or `|` when a file uses that). A MultiTerm
definition (.xdt) is read for its languages only and is not written.
A MultiTerm termbase file (.sdltb) is read directly, with its entries,
fields, history and name, so a termbase needs no export from MultiTerm
first; it is not written, MultiTerm takes the XML export instead.
Imports into an existing termbase match on a shared term (in one or any
language), the entry ID, or nothing, then merge (only adds), overwrite,
skip or add.
