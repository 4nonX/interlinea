# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

The browser client is web and is the reference for the interface. The
desktop client is a native Qt 6 application on Windows, macOS and Linux;
what the web client shows and does, the desktop client shows and does the
same way.

## Stack

React 19 + TypeScript + Vite for the browser client (`web/`). Go core and
server (this repository). Desktop client: Qt 6 Quick (QML) and C++20
(`desktop/`).

## Users

- Primary: professional translators. First user is the project owner, a
  freelance and sworn translator (German/English, legal domain). Works on
  contracts, court and certified documents, alone, often under deadline.
- Next: former and current students and lecturers of SDI Munich (teaching
  and first professional years).
- Later: agencies and in-house language teams using a self-hosted or hosted
  server.

## Product Purpose

Interlinea is an open-source translation workspace: CAT editor, terminology
(termbases), translation memory and project workflow in one tool. It exists
because the established tools (Trados Studio with MultiTerm above all) make
setup and terminology work database-first and clunky. Success means a
translator can drop a document, have a working project with the right
termbases in under a minute, and keep terminology consistent without ever
managing a database schema.

## Positioning

- Translation first, database hidden: termbases are created and structured
  as a side effect of translating; fields ("information types") grow with use
  instead of being defined up front.
- Git-like terminology: a project termbase works like a branch of one or more
  master termbases, with history per entry, updates from the master, and a
  merge review on completion.
- Local or server: the same termbases and memories work locally or on a
  self-hosted (or hosted) server.
- Open source, accessible, cross-platform.

## Operating Context

- Projects start from a source document; the source language is detected
  automatically, with manual override and fallback.
- Several user-defined master termbases (per client, per domain, per
  language set); a project uses one or more of them.
- Termbases can live locally or on a server; a project may mix both.
- Interoperability with Trados (SDLXLIFF, packages, MultiTerm/TBX exports)
  is a requirement, because agencies send Trados material.

## Capabilities and Constraints

- Built and released (0.6.0): the editor with memories, termbases, checks,
  spelling and machine translation; projects with analysis, assignments
  and review; document, subtitle, software and bilingual formats, Trados
  and memoQ packages back as packages; termbases with project termbases,
  review and merge; printed glossaries; alignment; the command line; the
  desktop app, the browser client and the team server with sign-in,
  roles, a live feed, an API and an MCP endpoint.
- Terminology model: concept-oriented (concept, language, term), TBX
  compatible underneath.
- Git vocabulary is switchable: plain translator words by default
  ("project changes", "merge into master", "history", "update from master"),
  literal git terms (branch, commit, merge, pull, diff, revert) as an expert
  setting. Same mechanics either way.
- Licence: GNU AGPL 3.0 only (LICENSE, NOTICE).
- Server projects are worked on live on the server: translations are saved
  there as they are typed and confirmed, and a translation that can't be
  saved stays marked "Not saved" on its segment. There is no offline copy
  of a server project; local projects work without a network.
- Not yet: signed Windows installers and a notarized macOS app; memoQ
  delivery packages tried in memoQ itself.

## Brand Commitments

- Name: Interlinea.
- Never use em dashes in UI copy or docs.

## Evidence on Hand

No customers, testimonials or benchmarks exist yet. Do not invent any.
Screenshots of the app are in docs/screenshots, made by the tests. The
website is interlinea.dev, built from site/.

## Product Principles

1. The work comes first; setup and data structure stay out of the way until
   asked for.
2. Simple by default, powerful on demand: every advanced control has a
   sensible default and is never required.
3. Nothing is lost: every terminology change is attributable and reversible.
4. Local and server are the same product, not two modes with different rules.
5. Accessible to every translator, including blind and low-vision
   professionals.

## Accessibility & Inclusion

WCAG 2.2 AA as a floor. Full keyboard operation, screen readers (NVDA, JAWS,
VoiceOver, Orca), braille displays (correct `lang` on source and target
text), magnification software (real focus and caret), Windows high contrast
/ forced colors, reduced motion. Multiscript text: CJK input methods, RTL and
bidirectional text.
