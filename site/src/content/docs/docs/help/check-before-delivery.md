---
title: Check a file before delivery
description: What to look at before a translated file or a return package goes back to a client.
---

Interlinea writes files back the way the program that made them expects:
everything that was not translated stays byte for byte, and a bilingual
file gets only its changed targets and states. Interlinea is new, though,
and your client's tools are the ones that count. Until you have seen your
kind of file go through once, check it before it leaves.

## Every file

1. Run the project's checks once more on the whole file and look at what
   is left: empty targets, tags, numbers, terms and consistency.
2. Open **Preview** and read the document through, or open the saved file
   in the program it belongs to (Word, InDesign, a subtitle player).
3. Keep the file you were sent. If anything is wrong, it is what a bug
   report needs.

## Trados

A return package (.sdlrpx) holds the package's project file and its
SDLXLIFF files, the translated ones replaced. If you or a colleague has
Trados Studio, open the return package there once: it should match the
project and show your segments with their states.

For a single SDLXLIFF, open it in Trados Studio or import it into the
project it came from.

## memoQ

The delivery package (.mqback) is built the way delivery packages are put
together by hand, because memoQ publishes no description of the format.
**It has not been tried against memoQ yet.** Until it has, the safe way
is to save each document with **Save XLIFF** and send the MQXLIFF files:
memoQ's import of bilingual files takes them, and that is documented.

If your client confirms that a delivery package worked, or did not, please
[say so on GitHub](https://github.com/4nonX/interlinea/issues/new?template=round-trip.yml);
the same form takes reports of any other file back in its program.

## Subtitles

Play the translated file with the video in a player or subtitle editor and
watch the timing of a few long lines. Subtitles longer than the original
are moved or wrapped as [the formats page](/docs/manual/formats/)
describes for each format.
