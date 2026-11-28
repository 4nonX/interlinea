# Changelog

What changed in each release of Interlinea. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the versions
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). A release
takes its notes from its section here.

## [Unreleased]

## [0.6.0] - 2026-11-28

### Added

- Light and dark in both clients: Settings, Appearance chooses light,
  dark, or the system's setting (the default). The browser client, light
  only until now, has the desktop app's dark colours; the desktop app,
  which followed the system only, can now be set to either. The change is
  shown at once.

### Changed

- Desktop app: the interface is now built in Qt Quick, in Interlinea's
  own design, and looks like the browser client on Windows, macOS and
  Linux alike. Every page came over with all it did: projects and New
  project, the project page, the editor with its panel, preview and
  dialogs, memories with alignment, terminology with merge review,
  accounts and settings. It has a menu bar (File, Edit, View, Help) with
  the system's shortcuts and native file dialogs; documents named when
  Interlinea starts, handed over by a second start or opened from the
  system (macOS) begin a project, and the window keeps its place and size.
- Desktop app: narrow windows. The editor's bar puts the filter and the
  search on a line of their own, Terminology puts the termbases over the
  entries, and an account's details and the role form wrap; the roles
  table scrolls sideways where it can't.
- Terminology in the browser client: an entry's row and the head of its
  details show each language with all its terms, in the termbase's
  order, where they showed one term and a count; forbidden terms are
  struck through.
- New termbase: the fields are no longer one of three ready-made
  structures. Definitions, notes, subject field, grammar, context and
  usage, and the date last checked are groups ticked in any combination,
  each putting its fields in the list (and taking them out again), where
  they can be changed and fields of one's own added; the fields of a
  termbase there is are added to them, not put in their place. In the
  desktop app and the browser client.
- Segments are joined across paragraphs too, as languages divide a text
  into paragraphs differently (and a PDF's lines often come in as
  paragraphs of their own). The translation is written in the first
  paragraph, the later ones without what was joined; in Word and
  PowerPoint with its formatting and fields. Split where the paragraphs
  met, the segments stand apart again. XLIFF stays joined only within a
  unit.

### Fixed

- Desktop app: the lists of a choice (the languages above all) opened
  empty: their rows asked for what a list with named fields does not
  give, and stayed without text. They show the choice's own text for
  each row now.
- Desktop app: the box where a segment is split showed no cursor (Qt
  draws none in a text that cannot be changed); a blinking line now shows
  where it will be split.
- Desktop app: in the editor's list of translation memories, the buttons
  of a memory covered its number of units; they go onto a second line
  where the column is narrow.
- Desktop app in a small window (its least, 800 by 520, as on a laptop at
  150 % scaling) and with the wider letters of Windows: rows of fields
  and buttons ran out of their panel. Such rows now put their parts one
  under another where they do not fit (the search over a memory's units,
  the row that makes an access token, the box to start a project, which
  all four boxes for dropping files use); the buttons' column of a table
  gives way to the others, its buttons going onto more lines; the line
  under a term entry that says when it last changed wraps; tabs wider
  than their pane scroll sideways and bring the chosen one into view; in
  Terminology the entry keeps more of a narrow window.
- Desktop app on macOS: settings saved by Interlinea 0.4 or earlier come
  along again when it starts for the first time; the move to Qt Quick had
  left this out.
- Site: on phones the rounded corner of the download cards cut into the
  Windows, macOS and Linux icons.
- Browser client in German and Korean: no more English on the screen.
  The names, descriptions and messages of the checks, the rows of an
  analysis, the descriptions of tags in the editor ("Footnote reference",
  "Bold") and the history of termbases on a team server come from the
  core and are now translated where they are shown. About 80 places that
  put sentences together from pieces ("Konflikts", "Projekt-Termbanks",
  "Tag 1 end, Bold", "Segment 8 of 38") are whole translated sentences.
  Shortcuts use the German key names (Strg, Umschalt, Einfg). A new
  termbase's standard fields and their values are named in the interface
  language. Numbers in the messages of the checks are each in quotes, as
  a German number has commas of its own.
- Terminology: a field can be removed where an entry shows it, by those
  who manage the termbase, after saying how many values go with it. Until
  now that was only possible in the termbase's settings.

### Tests

- The desktop tests work the Qt Quick window as a person does: parts are
  found by name, buttons clicked, text typed key by key and notices read,
  and a layout check looks for text that is cut off or runs out of its
  row. They found and fixed: comments and review issues of the open
  segment showing only after moving on, a new translation service's form
  without the service's name, and a missing message when the core does
  not start.

## [0.5.1] - 2026-11-27

### Fixed

- New project: a target language picked in the list and not added with
  Add left the project without a target language, so it could not go on
  to the memories. The language is now added on continuing, and a line
  under the list says so; with no target yet the list reads "Choose a
  language…". In the desktop app and the browser client.

## [0.5.0] - 2026-11-27

### Alignment

- Where a translation has its sections in another order than the
  document, the review moves them into the document's order: a handle
  beside each segment drags it, Alt+Up and Alt+Down move it one place, and
  Shift+click on handles marks several to move together. The sentences
  are paired again in the new order around the pairs already confirmed,
  and a move is undone like a change of the lines. In the browser client
  and the desktop app.
- `POST /api/align` and the core's align function take the order each
  side is read in.

### Desktop app

- The app's ID is dev.interlinea.Interlinea, on the project's own
  domain: the bundle ID on macOS, the desktop entry on Linux and the name
  sign-ins are kept under in the keychain. Sign-ins and, on macOS, the
  settings kept under the old ID come along on the first start.
- Interlinea.exe carries its product name and version in its file
  properties.

### Releases

- The release workflow signs the Windows programs through SignPath and
  signs and notarizes the macOS app with a Developer ID once those are set
  up ([docs/signing.md](docs/signing.md)); until then it builds them
  unsigned as before. The release notes describe the first-start warning
  only for files that are not signed.
- interlinea.dev: the downloads, help pages and the documentation, built
  from `site/` and `docs/`.

### Changed

- Files are checked out exactly as they are kept, whatever
  `core.autocrlf` says.
- README: the first start on macOS 15 goes through Privacy & Security,
  Open Anyway.
- SECURITY.md says how to report a security problem privately; issue
  forms for bugs and for files back in Trados, memoQ and other tools.

## [0.4.0] - 2026-11-26

### Subtitles

- More subtitle formats: YouTube (.sbv), Advanced SubStation Alpha and
  SubStation Alpha (.ass, .ssa), MicroDVD and SubViewer (.sub), Timed Text
  (TTML, DFXP, Apple iTT, EBU-TT and IMSC, as .ttml, .dfxp, .itt or .xml)
  and EBU STL (.stl), the binary format broadcasters exchange subtitles in,
  for teletext and open subtitles.
- One subtitle is one segment with its times shown; its line breaks and
  markup are tags. EBU STL subtitles are written back in their character
  set, in extension blocks where a translation is longer, moved up or down
  to keep their last row in place, with the header updated.
- SubRip and the other text subtitles made on Windows (Windows-1252) are
  read; a translation with letters that encoding lacks is saved in UTF-8.
- Scenarist closed captions (.scc, CEA-608) in all three modes. Pop-on
  captions are segments with their times; a translation is loaded again
  with its rows placed and aligned, long rows wrapped at 32 columns, and
  its time code moved so the caption still shows when it did. Each roll-up
  line is a segment, written back in place of the line's letters, moved
  left or rolled on to a next line where it is longer; paint-on captions
  are painted back in place. The reader is tested against the 142 caption
  files of pycaption's tests, written to trip decoders up.
- Subtitle checks: characters in a line, lines in a subtitle and reading
  speed (characters per second), with limits in each project's settings
  (42, 2 and 17 by default). The editor counts them under a subtitle as it
  is typed. Closed captions keep to their 32 columns.
- EBU STL: the programme and episode titles of the header are translated
  into its translated titles. A translated title in letters the header's
  code page lacks is reported with the letters, not left out. The reader is
  tested against the 53 files of ttconv's tests (IRT and Sandflow); a
  subtitle written in extension blocks is read whole.
- The browser client keeps the notes of document strings (keys, subtitle
  times) as the desktop app and the server do.

### The interface in German and Korean

- Both apps can be shown in English, German or Korean, chosen under
  Settings; by default they follow the language of the browser or the
  system. The browser client switches at once, the desktop app when it
  starts again. Dates, numbers and lists follow the interface language.
- The translations are one catalog per language, shared by the browser
  client and the desktop app (`i18n/`). CI fails when a string of either
  app has no translation, and the desktop tests show every page in German
  and Korean through the layout check.

### Spelling dictionaries included

- Spelling dictionaries for 22 languages and regions come with the desktop
  app, the team server and the web app: English (US, UK), German (Germany,
  Austria, Switzerland), French, Spanish (Spain, Mexico), Italian,
  Portuguese (Portugal, Brazil), Dutch, Polish, Russian, Swedish, Danish,
  Norwegian Bokmål, Czech, Korean, Ukrainian, Hungarian and Romanian, from
  the LibreOffice dictionaries with their authors' licences.
- A new project spells each target language with the dictionary that fits
  it, in the desktop app too. Your own dictionaries are added as before; one
  under the name of an included dictionary takes its place.
- The server takes the folder from `-dictionaries` (`INTERLINEA_DICTIONARIES`),
  or finds it next to the program or in the web app's folder. `make
  dictionaries` fetches and packs them from source.
- Hunspell dictionaries with numeric flags are read as Hunspell reads them
  (a flag 0, a flag with text after its number), so the Danish and Turkish
  dictionaries load.

### Translation services

- DeepL, Google Cloud Translation, Microsoft Translator, OpenAI (and any
  service that speaks its API, such as Mistral or a model run at home) and
  Anthropic's Claude are built in: choose one in Settings, paste the key
  of your own account, and the editor asks it for suggestions. A language
  model's model is chosen from the list the service gives for the key.
  Tags are kept. A service of your own at an address still works as
  before.
- On a team server the server asks the services, so keys stay there; on
  your computer the desktop app's core does. The web app on its own asks
  them from the browser, except DeepL, which takes no requests from a web
  page.

## [0.3.0] - 2026-11-26

### Bilingual files

- Bilingual tables for review (Word and RTF) open as the bilingual files they
  are, in the desktop app, the browser client, on the team server and on
  the command line: Trados Studio review documents (.docx), memoQ
  two-column RTF and DOCX, and Déjà Vu RTF external views. Tags, locked
  content, comments, states and match values are read; translations go
  back into the target cells, and the status shown with them, with every
  other byte as it was, so Studio, memoQ and Déjà Vu read the file back.
  Their segments are not split or joined.
- A Word document is taken for a review table by its content, so a Trados
  review document renamed by hand is still one.
- Phrase (Memsource) files (.mxliff) open with their translations, states,
  locks and match values; Phrase's codes in the text are tags, and a file
  for review counts confirming as the reviewer's.
- Wordfast Pro files (.txml) open with their translations and drafts, and
  are written back as Wordfast writes them. Wordfast Pro 5 files (.txlf)
  are XLIFF.
- Trados TagEditor files (.ttx) open with their segments; text TagEditor
  had not segmented is cut into sentences and written back as segments of
  their own. A file that names no target language is for the project's
  first.
- Bilingual files of these programs keep their segments as made.

### Memories

- Trados Studio memories (.sdltm) and Wordfast memories (.txt) are
  imported wherever TMX is: the Memories page, a project's memories and the
  editor's, in the desktop app, the browser client and on the team server,
  and on the command line (`tm import`, `lookup`, `concordance`, `analyze`).
  Tags, who made and changed a unit and when, how often it was used, and a
  memory's own fields come with the units.
- A new memory made from one of these files takes the name and languages
  the file gives.
- A memory file in the memory's languages the other way round is read the
  other way round; one of other languages stores nothing, and the import
  says why.
- Memories are exported as Wordfast memories too, beside TMX: UTF-16 text
  as Wordfast writes it, with dates, users, usage counts, up to five fields
  and the tags as Wordfast's codes. On the Memories page, a project's
  memories in the editor, on the team server, and with `tm export -format
  wordfast`.
- The team server's memory page and the editor's list of memories in the
  desktop app import every memory file the other places do.

### Changed

- The apps name file formats by their extension (.sdlxliff, .mxliff,
  .txml, .ttx, .sdltm, .txt), and the review tables by what they are
  (review table, two-column table, external view).
- The README is a short overview; using the apps, the file formats,
  matching and checks, the command line, the team server and development
  each have a document of their own in `docs/`, with new screenshots.

### Fixed

- A termbase keeps spellings that differ only in case as terms of their
  own, as MultiTerm does: a forbidden "Mistjet" beside the preferred
  "MistJet" is no longer dropped on import, may be added by hand, and the
  term check reports it only where it is written so.
- Terms marked "not allowed", as in Trados's own sample termbase, and the
  other usual words for it in English, German, French, Spanish and Italian,
  are read as forbidden instead of proposed.
- A Trados segment from a memory or machine translation that is changed
  keeps where it came from in an `sdl:prev-origin`, as Trados writes it.
- In the desktop app, a change to a termbase's settings that was saved
  while the page was being read again (after another change came in) is
  announced and shown; its notice was dropped.
- A PDF file whose text has broken coordinates no longer ends the
  program (on a team server, the whole server) while its reading order is
  worked out.
- Importing a spreadsheet or CSV file whose header has a cell of many
  words no longer takes seconds to minutes.

### Security

- Someone who manages a project may make it use only the memories and
  termbases they may use themselves: read on what it looks things up in,
  edit on a memory it stores translations in and on the master new terms
  go into. Before, a manager of one project could add any memory or
  termbase of the server to it and read it, or write into it, through the
  project. Those who make projects (manage on all projects) still choose
  among all memories and masters, as the catalog offers them. Taking an
  existing termbase as a project's own termbase, which makes the
  project's managers manage it, needs manage on that termbase.
- Linking a sign-in of the identity provider to an account needs the
  account to confirm it: the provider's answer brings a one-time code
  (`#interlinea-link=`) that the account sends to
  `POST /api/me/identities/confirm`. Before, someone could begin a link
  and get another person to sign in at the provider, and that person's
  sign-in was linked to the first one's account.
- The web app's pages carry a Content-Security-Policy that runs only the
  app's own scripts and lets no other site frame them. A server serving
  HTTPS itself sends Strict-Transport-Security.
- Archives (Word, Excel, PowerPoint, InDesign and OpenDocument files,
  packages, dictionaries) with more than 100,000 entries, or that unpack
  to more than 1 GB, are refused before they are unpacked; a PDF's streams
  decode to at most 1 GB together. A small file can no longer fill a
  server's memory.
- Requests to a translation or grammar service follow redirects only on
  its own host, so its secret header goes nowhere else.
- Spelling suggestions are made for words of up to 100 characters: the
  time they take grows with the square of a word's length, and one word
  of thousands of characters kept a server busy for seconds.
- A word with ten or more dashes (or other break points) is checked as it
  is, as Hunspell does, instead of split every way: thirty dashes took
  minutes and gigabytes.

### Tests

- Files written by Trados Studio, MultiTerm and memoQ, from public
  repositories, are read, translated and written back in the tests: nine
  SDLXLIFF files, two MultiTerm termbases, four memoQ XLIFF files and a
  memoQ archive. Where each comes from is in
  `internal/realworld/testdata/README.md`.
- Fuzz tests for every reader of files people bring: bilingual files,
  documents in every format, PDF, memories, termbases, project and handoff
  packages, spelling dictionaries and RTF. `go test` runs them on the
  sample files; `go test -fuzz=FuzzExtract ./filter/xliff` (and so on)
  goes on with inputs of its own.
- Every route of the server is called without signing in, and by someone
  without grants on things others made; only the routes for everyone
  answer.
- The web app's end-to-end tests run under the server's
  Content-Security-Policy and fail on anything it blocks.

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

[Unreleased]: https://github.com/4nonX/interlinea/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/4nonX/interlinea/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/4nonX/interlinea/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/4nonX/interlinea/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/4nonX/interlinea/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/4nonX/interlinea/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/4nonX/interlinea/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/4nonX/interlinea/releases/tag/v0.1.0
