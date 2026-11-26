---
title: Where your work is kept
description: The desktop app's data folder, the browser client's storage, and how to back both up.
---

Interlinea needs no account and sends nothing anywhere unless you sign in
to a team server or turn on a translation service with your own key.

## Desktop app

Projects, translations, termbases, memories, templates and dictionaries
are saved as you work, in a data folder:

| System | Folder |
|---|---|
| Windows | `%APPDATA%\Interlinea\Interlinea\data` |
| macOS | `~/Library/Application Support/Interlinea/Interlinea/data` |
| Linux | `~/.local/share/Interlinea/Interlinea/data` |

**Settings** shows the folder and has a button to open it. To keep the
data somewhere else, start the app with the environment variable
`INTERLINEA_DATA` set to that folder.

**Backing up:** quit Interlinea, then copy the whole data folder. To
restore, quit Interlinea and put the copy back in its place.

## Browser client

The browser client keeps your work in the browser you use (IndexedDB),
on your computer. It is there again on the next visit in the same browser,
but not in another browser or on another computer. Clearing the browser's
site data deletes it.

To keep a copy, export what matters: memories as TMX, termbases as TBX,
and the translated files themselves. **Settings** shows whether saving
works in this browser and can delete everything kept there.

## Team server

On a team server the work is on the server. Its administrator backs it up
as [running a team server](/docs/manual/running/#backups) describes.
