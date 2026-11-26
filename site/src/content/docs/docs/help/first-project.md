---
title: Translate your first file
description: From a document or a Trados package to the translated file, in six steps.
---

This walks through one file from start to delivery. The
[manual](/docs/manual/guide/) has every option on the way.

## 1. Make a project

On **Projects**, choose **New project** and add the files: documents,
whole folders, bilingual files (SDLXLIFF, MQXLIFF, XLIFF) or a Trados
package (.sdlppx), which becomes one project with all its files.
Interlinea detects the source language; set the target languages.

## 2. Add memories and termbases

Choose the memories to look up and update for each target language, and
the termbases to use. To bring in what you already have, import a TMX
file, a Trados Studio memory (.sdltm) or a Wordfast memory, and a TBX,
MultiTerm or Excel termbase. Termbases for the same client are suggested.

## 3. Look at the analysis

Before the project is made, the analysis counts the words by match band,
weighted as you set it, for a quote. It is on the project's page later too.

## 4. Translate

Creating the project opens the editor. The keys are the ones you know
from other tools:

| Key | Does |
|---|---|
| <kbd>Ctrl</kbd>+<kbd>Enter</kbd> | confirms the segment, fills its repetitions and goes to the next |
| <kbd>Ctrl</kbd>+<kbd>1</kbd> to <kbd>9</kbd> | inserts the numbered result |
| <kbd>Ctrl</kbd>+<kbd>K</kbd> | searches the memory for the selected text |
| <kbd>F9</kbd> | inserts the next tag |

Confirmed segments go into the project's memories. Checks run as you
type; a warning stays on its segment until you fix or ignore it.

## 5. Preview

**Preview** shows the translated document with its headings, tables and
footnotes. A click in it opens that segment.

## 6. Save the result

- A document: **Save translated document** writes it with its original
  layout.
- A bilingual file: **Save XLIFF** writes it back under its own name,
  with only the changed segments and states.
- A Trados package: **Save return package** writes one .sdlrpx with every
  file translated.

Before you send it, see [check a file before delivery](/docs/help/check-before-delivery/).
