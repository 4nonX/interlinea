# Changelog

What changed in each release of Interlinea. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the versions
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). A release
takes its notes from its section here.

## [Unreleased]

## [0.2.0] - 2026-11-25

Termbases made from scratch, with fields of every kind; glossaries to
read and print, as PDF, Word, a web page or text; and a desktop app whose
layout is checked by its tests and shows a real preview.

### Terminology

- A termbase is made in steps: its name and kind, its languages from a
  list with a search, its fields, and a review before it is made. A
  master starts from a ready-made structure or from another master's
  fields; a project termbase shows what it gets from its masters and may
  have fields of its own.
- Fields take text, a value from a list, a number, a date, or yes or no,
  each with its own control in an entry. The values of a list are typed
  one a line. Fields are added in the termbase settings as well as from
  an entry.
- A field a project termbase has and its master lacks reaches the master
  with the first approved entry that uses it, with its kind and values.
- The team server says which part of a termbase's definition it refuses
  and why.
- Glossaries to read and print: a termbase is exported as a PDF, a Word
  document, a web page or plain text, laid out as a dictionary (headwords
  in two columns, each language on a line under them, initial letters,
  the first and last headword of each page in its head) or as a table
  with a column for each language.
- Grammar as dictionaries set it: part of speech, gender and number in
  short italics after each term, with a list of the abbreviations used.
- PDF files are made by the core itself, with their fonts embedded:
  Latin, Greek, Cyrillic, Arabic and Hebrew come with the app; Chinese,
  Japanese, Korean and other scripts take the fonts installed on the
  computer or server. Arabic letters join and right-to-left text runs as
  it should. The letters of a glossary are the bookmarks of its PDF.
- An export chooses its languages, entries, term statuses and fields,
  can keep only entries with a field value or changed since a day, and
  applies the same to the exchange formats. Export settings are kept with
  a termbase by name, for everyone who exports it; a few ready-made ones
  come with every termbase.

### Desktop app

- The editor's preview shows the document as it is laid out, in the
  target language only.
- Lists look like lists, choices open on a click, and text no longer runs
  out of its box at any scaling.
- Every window is checked by the tests for broken layout, at 100% and
  150% scaling, at several window sizes and in both themes: text cut off,
  parts lying over each other, buttons narrower than their labels, boxes
  too low for a line. The app writes what it finds to its log when
  INTERLINEA_LAYOUT_CHECK is set.
- Dialogs scroll when they are taller than the screen, and grow when what
  they show changes. The licence is shown in full; the analysis and
  memories windows of the editor are built like the others; long choices
  wrap.

## [0.1.0] - 2026-11-25

The first release: the core, the team server, the web client and the
desktop app, with installers and a server image.

Interlinea is free software under the GNU Affero General Public License,
version 3.

### Translating

- Projects from Word, PowerPoint and Excel documents, OpenDocument text,
  spreadsheets and presentations, web pages, InDesign documents (IDML),
  XML, Markdown, software strings (Java properties, JSON, YAML, gettext
  PO, .NET RESX), subtitles (SRT, WebVTT), plain text, XLIFF, Trados
  SDLXLIFF and memoQ MQXLIFF files, and from Trados project packages and
  memoQ handoff packages, with the translated files and return packages
  written back. In software strings a placeholder is a tag and the
  string's key is shown with it.
- PDF files: their text is read without any other program, with lines,
  columns and paragraphs found from where the text stands on the pages,
  and translated into a Word document. A protected PDF, a scan without
  text and a PDF whose fonts do not name their characters say why they
  are not read.
- An editor with translation memory matches, concordance search, found
  terms, formatting tags, spelling, quality checks, own rules and length
  limits. The best memory match goes into an empty segment, and a
  confirmed translation fills its repetitions, if wanted.
- Match scores that hold in every listed language: a word in another
  form, a spelling with other marks, a number in other digits and
  punctuation in another form each cost what they are worth, and a short
  segment finds its match in another form.
- The segments shown in the editor are narrowed to those that hold a
  text, in the source or the translation, besides to those still to do
  or with warnings.
- Comments on segments, for the others working on a project: shown
  with the segment, counted in its row, kept on the team server and
  seen there as they are written. They do not go into the document,
  and leave the translation and its version alone.
- Find and replace in the translations of a file, with matching case
  and whole words: the places are counted before anything changes, tags
  and locked segments stay, and a segment that changes is a draft again.
- A segment shows the match it was translated from beside it. An empty
  segment shows none: its matches are in the results when it is opened.
- Analysis of a project against its memories, by match band.
- Alignment: a document and its translation, done before, are paired
  sentence by sentence and shown side by side with a line from each
  sentence to its translation. Lines are drawn and taken away by hand,
  and what is paired goes into a memory. A line paired automatically is
  dotted; one drawn or confirmed is solid and stays when the rest is
  paired again around it.
  Changes to the lines are undone and done again. Documents are aligned
  from the Memories page into a memory chosen there or made for them, or
  from an open memory into that one.
- Translation memories with TMX in and out. The units of a memory are
  searched, added, changed with their formatting tags kept, and deleted.
- Languages: number formats and digits of every listed script, sentence
  marks, word forms, detection and segmentation; right-to-left segments
  and Word output; Thai, Lao, Khmer and Burmese counted and matched by
  character.

### Segments

- Any segment can be split at any place, and any two segments of a paragraph joined, as often as wanted: a joined segment is split again, a part of one sentence is joined with the next sentence (Alt+Shift+S, Alt+Shift+J). Split where two segments were joined, they stand apart as the file has them. The translated file has the translations of the parts one after the other where the sentences stood, with the formatting of all of them. Only the language at hand is cut differently.
- This holds for documents, plain text and plain XLIFF files (in XLIFF within a unit of the file). Files of Trados and memoQ, and their packages, keep their segments: those programs keep their own record of every segment, and the editor says so.
- Plain text files are read by the same core as every other document, in the browser as on the desktop: a line is a paragraph, cut by the rules of the source language, and the translated text can be saved from the editor.
- A project can have segmentation rules of its own: regular expressions for the text before and after a place, where a sentence ends or does not. They come before the rules for the language and cut the documents and text files added to the project; templates carry them.

### Review

- Scored review (LQA): a reviewer marks issues on a segment, each with a kind (accuracy, fluency, terminology, style, locale conventions, formatting, other) and a severity (minor 1, major 5, critical 25 penalty points). The editor shows the file's score, 100 less the penalty points in percent of the source words, and the review of the whole project is saved as a CSV table. On a team server the issues are there for everyone and arrive while others work; marking one leaves the translation and its version alone.

### Grammar

- Grammar checking with a service of your own that answers the check
  request of the LanguageTool HTTP API, such as a LanguageTool server you
  run yourself. A service is added in the settings, and a project names
  the one that reads its translations.
- The open segment's translation is read a moment after the last change
  and when it is confirmed. What the service finds is listed with the
  checks, with replacements that go into the translation at a click, and
  is kept with the confirmed segment.
- Spelling stays with the project's dictionary. The quality report of a
  file from the API has the service's findings.

### Typing

- While a translation is typed, the terms found for the segment and the words of its memory matches are offered after two letters. Tab or Enter takes one, Escape closes the list. It can be turned off in the settings.

### Other programs

- An MCP endpoint (`/mcp`) on the team server and the desktop's core, for
  AI assistants and other programs that speak the Model Context
  Protocol: projects, segments with their tags, memory and concordance
  search, terms, saving translations and comments, each with the rights
  of the person whose token the program carries.
- Access tokens for programs, made and ended by their owner in the
  settings.
- Translation services of one's own: any machine translation engine or
  language model behind an address that answers a small JSON contract is
  asked for a suggestion in the editor, with the segment's tags kept. A
  team server asks its services itself and keeps their keys.

### Terminology

- Termbases with fields at entry, language and term level, picklists,
  term statuses, search, duplicates to merge, and a history of every
  entry.
- Import from TBX, MultiTerm XML, MultiTerm termbase files (.sdltb), CSV,
  TSV and Excel, with the columns and languages of a spreadsheet mapped
  and a look at what the import does before it is done. Export to TBX,
  MultiTerm XML, CSV, TSV and Excel.
- Master and project termbases: a project termbase shows its masters'
  entries, changes one for the project only or leaves it out, and submits
  its new and changed entries to a master for review.
- Review of what a project submits: three-way merge, a conflict decided
  for one side by name, approval of all entries without conflicts,
  postponing and rejecting with a note for the author, and undoing a
  merge. A master's rules say what merges without review: entries it has
  already, fields that need no approval, and trusted authors.
- An earlier version of an entry is restored from its history, and a
  deleted entry is brought back. Termbases the web client keeps in the
  browser have the same as those on the team server: versions, deleted
  entries, the masters' entries under a project termbase, and the
  master's review rules in the merge review.

### Working together

- The team server, first of all for a team's terminology: its termbases
  in one place, with review of what projects add and change. Projects
  and memories are shared there as well. Accounts sign in with a
  password or through an OpenID Connect provider (Entra ID, Google,
  Keycloak); the server keeps its data in SQLite or PostgreSQL.
- Roles as sets of the permissions read, suggest, edit, approve and
  manage: five come with the server, and administrators define others.
  Roles are given on single projects, memories and termbases or on all
  of a kind.
- Saving checks the version read: a change made by someone else in the
  meantime is shown, to keep one's own or take theirs.
- Live changes: what others save, who has which segment open, and work
  given to translators and reviewers.
- Reviewers are given parts of a merge request and see their part alone.
- A backup command, a health check, and an API reference written from
  the server's source.

### The apps

- The web client, served by the team server or used by itself in the
  browser with its data kept there. It starts empty; a sample project is
  added by `?sample` in the address.
- The desktop app for Windows, macOS and Linux, laid out as the web
  client, with its own core for work on this computer and connections to
  team servers. Sign-ins are kept in the system's keychain.
- The command line tool for projects, memories, termbases and analysis.
- The desktop app's New project, project and Settings pages have the
  layout and wording of the browser client: a project is set up in five
  steps with the match analysis before it is made, and its settings are
  panels down one page, saved together.
- The project page of both apps analyses the files and saves the count
  as CSV. Settings in the desktop app also hold the editor's choices,
  the wording of the terminology, project templates and spelling
  dictionaries.
- The interface names file formats by their extensions and compares
  itself to no other program.

### Installing

- A server image on the GitHub container registry, for amd64 and arm64.
- The server and the command line tool for Linux, macOS and Windows.
- The desktop app as a Windows installer, a macOS disk image and a Linux
  AppImage.

### Upgrades

- The data of each release is kept and opened by every later version in
  a test, and the steps of each store's schema can no longer be changed
  once recorded.

[Unreleased]: https://github.com/4nonX/interlinea/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/4nonX/interlinea/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/4nonX/interlinea/releases/tag/v0.1.0
