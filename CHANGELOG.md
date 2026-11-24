# Changelog

What changed in each release of Interlinea. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the versions
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). A release
takes its notes from its section here.

## [Unreleased]

What the first release will hold: the core, the team server, the web
client and the desktop app, with installers and a server image.

### Translating

- Projects from Word, PowerPoint and Excel documents, web pages,
  InDesign documents (IDML), plain text, XLIFF, Trados SDLXLIFF and memoQ
  MQXLIFF files, and from Trados project packages and memoQ handoff
  packages, with the translated files and return packages written back.
- An editor with translation memory matches, concordance search, found
  terms, formatting tags, spelling, quality checks, own rules and length
  limits. The best memory match goes into an empty segment, and a
  confirmed translation fills its repetitions, if wanted.
- Match scores that hold in every listed language: a word in another
  form, a spelling with other marks, a number in other digits and
  punctuation in another form each cost what they are worth, and a short
  segment finds its match in another form.
- The best match of every segment is shown beside it in the editor before
  anything is inserted: context, exact and fuzzy matches and repetitions.
- Analysis of a project against its memories, by match band.
- Alignment: a document and its translation, done before, are paired
  sentence by sentence and shown side by side with a line from each
  sentence to its translation. Lines are drawn and taken away by hand,
  and what is paired goes into a memory. A line paired automatically is
  dotted; one drawn or confirmed is solid and stays when the rest is
  paired again around it.
- Translation memories with TMX in and out. The units of a memory are
  searched, added, changed with their formatting tags kept, and deleted.
- Languages: number formats and digits of every listed script, sentence
  marks, word forms, detection and segmentation; right-to-left segments
  and Word output; Thai, Lao, Khmer and Burmese counted and matched by
  character.

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

[Unreleased]: https://github.com/4nonX/interlinea/commits/main
