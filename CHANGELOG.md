# Changelog

What changed in each release of Interlinea. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the versions
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). A release
takes its notes from its section here.

## [Unreleased]

## [0.1.0] - 2026-11-24

The first release: the core, the team server, the web client and the
desktop app, with installers and a server image.

### Translating

- Projects from Word documents, plain text, Trados SDLXLIFF and memoQ
  MQXLIFF files, and from Trados project packages, with the translated
  files and return packages written back.
- An editor with translation memory matches, concordance search, found
  terms, formatting tags, spelling, quality checks, own rules and length
  limits. The best memory match goes into an empty segment, and a
  confirmed translation fills its repetitions, if wanted.
- Analysis of a project against its memories, by match band.
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
  deleted entry is brought back.

### Working together

- The team server: accounts, sign-in with a password or through an
  OpenID Connect provider (Entra ID, Google, Keycloak), projects,
  memories and termbases for a team, on SQLite or PostgreSQL.
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
  browser with its data kept there.
- The desktop app for Windows, macOS and Linux, laid out as the web
  client, with its own core for work on this computer and connections to
  team servers. Sign-ins are kept in the system's keychain.
- The command line tool for projects, memories, termbases and analysis.

### Installing

- A server image on the GitHub container registry, for amd64 and arm64.
- The server and the command line tool for Linux, macOS and Windows.
- The desktop app as a Windows installer, a macOS disk image and a Linux
  AppImage.

[Unreleased]: https://github.com/4nonX/interlinea/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/4nonX/interlinea/releases/tag/v0.1.0
