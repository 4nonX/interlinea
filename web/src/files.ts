// Files chosen or dropped by the user, with the path inside a folder kept,
// so a project's files keep the structure they came in.

export interface ChosenFile {
  file: File;
  /** Path relative to the chosen or dropped folder; the name for a single file. */
  path?: string;
}

/** Files from a file input; a folder input gives each file its relative path. */
export function chosenFiles(list: FileList): ChosenFile[] {
  return [...list].map((file) => ({
    file,
    path: file.webkitRelativePath || undefined,
  }));
}

async function walk(e: FileSystemEntry, out: ChosenFile[]): Promise<void> {
  if (e.isFile) {
    const file = await new Promise<File>((ok, fail) =>
      (e as FileSystemFileEntry).file(ok, fail),
    );
    out.push({ file, path: e.fullPath.replace(/^\//, "") });
    return;
  }
  if (!e.isDirectory) return;
  const reader = (e as FileSystemDirectoryEntry).createReader();
  // readEntries returns the entries in batches until an empty one.
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((ok, fail) =>
      reader.readEntries(ok, fail),
    );
    if (!batch.length) break;
    for (const child of batch) await walk(child, out);
  }
}

/** Files and folders dropped on the page, folders walked recursively. */
export async function droppedFiles(dt: DataTransfer): Promise<ChosenFile[]> {
  const entries = [...dt.items]
    .map((i) => i.webkitGetAsEntry())
    .filter((e): e is FileSystemEntry => !!e);
  if (!entries.length) return [...dt.files].map((file) => ({ file }));
  const out: ChosenFile[] = [];
  for (const e of entries) await walk(e, out);
  return out.map((f) => (f.path === f.file.name ? { file: f.file } : f));
}
