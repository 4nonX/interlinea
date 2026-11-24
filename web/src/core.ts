// The Interlinea core (Go) compiled to WebAssembly: the same Word filter the
// desktop app and the server use. Loaded on first use; about 1.3 MB
// compressed. Build it with `make wasm` in the repository root.

import {
  coreToRuns,
  plain,
  runsToCore,
  type Block,
  type CoreRun,
  type FileState,
  type Run,
  type Segment,
} from "./editor/model";

export { unwrap };

export type { Block };

/** A Hunspell dictionary found in a LibreOffice extension or ZIP file. */
export interface SpellBundle {
  name: string;
  locales: string[];
  aff: Uint8Array;
  dic: Uint8Array;
}

interface CoreUnit {
  id: string;
  block: string;
  source: CoreRun[];
  target?: CoreRun[];
  state?: FileState;
  locked?: boolean;
  match?: number;
  notes?: string[];
  /** Longest the translation may be, in characters (XLIFF maxwidth). */
  maxLength?: number;
}

interface CoreExtraction {
  units: CoreUnit[];
  blocks: Block[];
  warnings?: string[];
  format?: string;
  sourceLang?: string;
  targetLang?: string;
}

type Res<T> = { ok: true; value: T } | { ok: false; error: string };

export interface CoreApi {
  extract(bytes: Uint8Array, lang: string, options?: string): Res<string>;
  align(
    sourceName: string,
    source: Uint8Array,
    sourceLang: string,
    targetName: string,
    target: Uint8Array,
    targetLang: string,
  ): Res<string>;
  merge(
    bytes: Uint8Array,
    lang: string,
    target: string,
    translations: string,
    options?: string,
  ): Res<Uint8Array>;
  tmOpen(name: string, source: string, target: string): Res<boolean>;
  tmAdd(name: string, units: string, overwrite?: boolean): Res<number>;
  tmSetTarget(name: string, id: number, target: string, by: string): Res<string>;
  tmDelete(name: string, id: number): Res<boolean>;
  tmLookup(
    name: string,
    source: string,
    min: number,
    limit: number,
    penalty?: number,
  ): Res<string>;
  tmConcordance(
    name: string,
    query: string,
    inTarget: boolean,
    limit: number,
  ): Res<string>;
  tmCount(name: string): Res<number>;
  tmImportTMX(name: string, bytes: Uint8Array): Res<string>;
  tmExportTMX(name: string): Res<Uint8Array>;
  // Termbase functions: see termio.ts.
  xliffDetect(bytes: Uint8Array): Res<string>;
  xliffExtract(bytes: Uint8Array): Res<string>;
  xliffMerge(bytes: Uint8Array, updates: string): Res<Uint8Array>;
  archiveRead(bytes: Uint8Array): Res<Uint8Array>;
  archiveWrite(bytes: Uint8Array, document: Uint8Array): Res<Uint8Array>;
  packageOpen(bytes: Uint8Array, name: string): Res<string>;
  packageRead(bytes: Uint8Array, name: string, path: string): Res<Uint8Array>;
  packageReturn(
    bytes: Uint8Array,
    name: string,
    translated: Record<string, Uint8Array>,
  ): Res<Uint8Array>;
  qaKinds(): Res<string>;
  spellArchive(bytes: Uint8Array): Res<SpellBundle[]>;
  spellLoad(
    name: string,
    aff: Uint8Array,
    dic: Uint8Array,
    extraWords: string,
  ): Res<string>;
  spellDrop(name: string): Res<boolean>;
  qaCheck(segments: string, options: string): Res<string>;
  projectDefaults(): Res<string>;
  projectValidate(settings: string, catalog: string): Res<string>;
  projectTemplateValidate(template: string, catalog: string): Res<string>;
  analyze(files: string, memories: string, options: string): Res<string>;
  pretranslate(
    files: string,
    memories: string,
    prepare: string,
    rule: string,
  ): Res<string>;
}

function unwrap<T>(r: Res<T>): T {
  if (!r.ok) throw new Error(r.error);
  return r.value;
}

/** The loaded core, or null while it is still loading. */
export function core(): CoreApi | null {
  return window.interlineaCore ?? null;
}

/** A translation memory unit as the core stores it. */
export interface MemUnit {
  id?: number;
  source: CoreRun[];
  target: CoreRun[];
  createdBy?: string;
  createdAt?: string;
  changedBy?: string;
  changedAt?: string;
  attributes?: Record<string, string>;
  score?: number;
}

/** Two documents with their segments paired, as the core aligns them. */
export interface Alignment {
  source: CoreRun[][];
  target: CoreRun[][];
  pairs: { sourceSegments: number[]; targetSegments: number[]; score: number }[];
}

/** Pairs the segments of a document with those of its translation. */
export function alignDocuments(
  c: CoreApi,
  source: { name: string; bytes: Uint8Array; lang: string },
  target: { name: string; bytes: Uint8Array; lang: string },
): Alignment {
  return JSON.parse(
    unwrap(c.align(source.name, source.bytes, source.lang, target.name, target.bytes, target.lang)),
  ) as Alignment;
}

export const tm = {
  open(c: CoreApi, name: string, source: string, target: string) {
    unwrap(c.tmOpen(name, source, target));
  },
  /** With overwrite, a unit replaces the other translations of its source. */
  add(c: CoreApi, name: string, units: MemUnit[], overwrite = false) {
    return unwrap(c.tmAdd(name, JSON.stringify(units), overwrite));
  },
  /** Changes a unit's translation; the rest of the unit stays. */
  setTarget(c: CoreApi, name: string, id: number, target: CoreRun[], by: string): MemUnit {
    return JSON.parse(unwrap(c.tmSetTarget(name, id, JSON.stringify(target), by))) as MemUnit;
  },
  remove(c: CoreApi, name: string, id: number) {
    unwrap(c.tmDelete(name, id));
  },
  /** Matches of at least min percent; scores lowered by the memory's penalty. */
  lookup(
    c: CoreApi,
    name: string,
    source: CoreRun[],
    min: number,
    limit: number,
    penalty = 0,
  ): MemUnit[] {
    return JSON.parse(
      unwrap(c.tmLookup(name, JSON.stringify(source), min, limit, penalty)),
    ) as MemUnit[];
  },
  concordance(
    c: CoreApi,
    name: string,
    query: string,
    inTarget: boolean,
    limit: number,
  ): MemUnit[] {
    return JSON.parse(
      unwrap(c.tmConcordance(name, query, inTarget, limit)),
    ) as MemUnit[];
  },
  count(c: CoreApi, name: string): number {
    return unwrap(c.tmCount(name));
  },
  importTMX(
    c: CoreApi,
    name: string,
    bytes: Uint8Array,
  ): { read: number; stored: number; skipped: number } {
    return JSON.parse(unwrap(c.tmImportTMX(name, bytes)));
  },
  exportTMX(c: CoreApi, name: string): Uint8Array {
    return unwrap(c.tmExportTMX(name));
  },
};

declare global {
  interface Window {
    interlineaCore?: CoreApi;
    interlineaCoreReady?: () => void;
    Go?: new () => {
      importObject: WebAssembly.Imports;
      run(instance: WebAssembly.Instance): Promise<void>;
    };
  }
}

let loading: Promise<CoreApi> | null = null;

function base(): string {
  return new URL("./", document.baseURI).href;
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`Could not load ${src}`));
    document.head.append(s);
  });
}

export function loadCore(): Promise<CoreApi> {
  if (window.interlineaCore) return Promise.resolve(window.interlineaCore);
  loading ??= (async () => {
    if (!window.Go) await loadScript(`${base()}core/wasm_exec.js`);
    const res = await fetch(`${base()}core/interlinea.wasm`);
    if (!res.ok)
      throw new Error(
        `The core is missing (${res.status}). Run "make wasm" in the repository.`,
      );
    // instantiateStreaming needs the application/wasm type, which not every
    // host sends; compiling from the bytes works everywhere.
    const bytes = await res.arrayBuffer();
    const go = new window.Go!();
    const ready = new Promise<void>(
      (resolve) => (window.interlineaCoreReady = resolve),
    );
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    void go.run(instance);
    await ready;
    return window.interlineaCore!;
  })().catch((e) => {
    loading = null;
    throw e;
  });
  return loading;
}

export interface Extracted {
  segments: Segment[];
  blocks: Block[];
  /** Things the filter changes or cannot translate (tracked changes, for example). */
  warnings: string[];
}

/** The document formats the core's filters read, by file name. */
export type DocumentFormat = "docx" | "pptx" | "xlsx" | "html" | "idml";

const DOCUMENT_FORMATS: [RegExp, DocumentFormat][] = [
  [/\.(docx|docm)$/i, "docx"],
  [/\.(pptx|pptm|ppsx)$/i, "pptx"],
  [/\.(xlsx|xlsm)$/i, "xlsx"],
  [/\.(html?|xhtml)$/i, "html"],
  [/\.idml$/i, "idml"],
];

/** The format of a document one of the core's filters reads; null for any other file. */
export function documentFormat(name: string): DocumentFormat | null {
  return DOCUMENT_FORMATS.find(([re]) => re.test(name))?.[1] ?? null;
}

/** What a document format is called. */
export const DOCUMENT_FORMAT_NAME: Record<DocumentFormat, string> = {
  docx: "DOCX document",
  pptx: "PPTX presentation",
  xlsx: "XLSX workbook",
  html: "Web page",
  idml: "IDML document",
};

/** The type a translated document is offered for download as. */
export const DOCUMENT_TYPE: Record<DocumentFormat, string> = {
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  html: "text/html",
  idml: "application/vnd.adobe.indesign-idml-package",
};

/**
 * Extracts the units of a document as editor segments: a Word document,
 * a presentation, a workbook, a web page or an InDesign document, by the
 * file's name.
 */
export async function extractDocx(
  bytes: Uint8Array,
  lang: string,
  name = "document.docx",
): Promise<Extracted> {
  const core = await loadCore();
  const r = core.extract(
    bytes,
    lang,
    JSON.stringify({ format: documentFormat(name) ?? "docx" }),
  );
  if (!r.ok) throw new Error(r.error);
  const ex = JSON.parse(r.value) as CoreExtraction;
  return {
    segments: ex.units.map((u) => ({
      id: u.id,
      block: u.block,
      source: coreToRuns(u.source),
      target: [],
      status: "new" as const,
    })),
    blocks: ex.blocks,
    warnings: ex.warnings ?? [],
  };
}

/**
 * Writes the translated document. Segments without a translation keep their
 * source text, as in other CAT tools.
 */
export async function mergeDocx(
  bytes: Uint8Array,
  lang: string,
  target: string,
  segments: Segment[],
  name = "document.docx",
): Promise<Uint8Array> {
  const core = await loadCore();
  const tr: Record<string, CoreRun[]> = {};
  for (const s of segments) {
    if (
      s.target.length &&
      s.target.some((r) => typeof r !== "string" || r.trim())
    )
      tr[s.id] = runsToCore(s.target);
  }
  const r = core.merge(
    bytes,
    lang,
    target,
    JSON.stringify(tr),
    JSON.stringify({ format: documentFormat(name) ?? "docx" }),
  );
  if (!r.ok) throw new Error(r.error);
  return r.value;
}

/** File names the XLIFF filter reads: XLIFF 1.2 and 2, Trados SDLXLIFF, memoQ MQXLIFF, also in its archive. */
export const XLIFF_FILE = /\.(xlf|xliff|sdlxliff|mqxliff|mqxlz)$/i;

/** Whether the bytes are a ZIP file: a memoQ XLIFF in its archive (.mqxlz). */
function archived(bytes: Uint8Array): boolean {
  return bytes.length > 3 && bytes[0] === 0x50 && bytes[1] === 0x4b && bytes[2] === 0x03 && bytes[3] === 0x04;
}
export const XLIFF_TYPE = "application/xliff+xml";

/** What a bilingual file holds besides its segments. */
export interface Bilingual extends Extracted {
  /** The variant: "xliff-1.2", "sdlxliff", "mqxliff" or "xliff-2". */
  format: string;
  sourceLang: string;
  targetLang: string;
}

export const XLIFF_FORMAT_NAME: Record<string, string> = {
  "xliff-1.2": "XLIFF 1.2",
  "xliff-2": "XLIFF 2",
  sdlxliff: "SDLXLIFF",
  mqxliff: "MQXLIFF",
};

// A segment's state in the file becomes the editor's: confirmed for
// translated and later, draft for drafts and rejections.
function editorStatus(
  state: FileState | undefined,
  target: Run[],
): Segment["status"] {
  switch (state) {
    case "translated":
    case "reviewed":
    case "final":
      return "confirmed";
    case "draft":
    case "rejected":
      return "draft";
    default:
      return plain(target).trim() ? "draft" : "new";
  }
}

/** Reads a bilingual XLIFF file with its translations and states. */
export async function extractXliff(bytes: Uint8Array): Promise<Bilingual> {
  const core = await loadCore();
  const doc = archived(bytes) ? unwrap(core.archiveRead(bytes)) : bytes;
  const ex = JSON.parse(unwrap(core.xliffExtract(doc))) as CoreExtraction;
  return {
    format: ex.format ?? "xliff-1.2",
    sourceLang: ex.sourceLang ?? "",
    targetLang: ex.targetLang ?? "",
    blocks: ex.blocks,
    warnings: ex.warnings ?? [],
    segments: ex.units.map((u): Segment => {
      const target = u.target ? coreToRuns(u.target) : [];
      return {
        id: u.id,
        block: u.block,
        source: coreToRuns(u.source),
        target,
        status: editorStatus(u.state, target),
        locked: u.locked || undefined,
        notes: u.notes,
        maxLength: u.maxLength || undefined,
        file: {
          state: u.state ?? "new",
          target: JSON.stringify(u.target ?? []),
          match: u.match || undefined,
        },
      };
    }),
  };
}

/**
 * The state to write for a segment: a confirmed segment is translated,
 * unless the file had it reviewed or signed off and the translation did not
 * change; a draft stays rejected if it was and did not change.
 */
function fileState(s: Segment): FileState {
  const unchanged =
    s.file !== undefined &&
    JSON.stringify(runsToCore(s.target)) === s.file.target;
  if (s.status === "confirmed")
    return unchanged &&
      (s.file?.state === "reviewed" || s.file?.state === "final")
      ? s.file.state
      : "translated";
  if (s.status === "draft")
    return unchanged && s.file?.state === "rejected" ? "rejected" : "draft";
  return plain(s.target).trim() ? "draft" : "new";
}

/** Writes translations and states into the original XLIFF file. Locked segments stay as they are. */
export async function mergeXliff(
  bytes: Uint8Array,
  segments: Segment[],
): Promise<Uint8Array> {
  const core = await loadCore();
  const updates: Record<string, { target: CoreRun[]; state: FileState }> = {};
  for (const s of segments) {
    if (!s.locked)
      updates[s.id] = { target: runsToCore(s.target), state: fileState(s) };
  }
  if (!archived(bytes))
    return unwrap(core.xliffMerge(bytes, JSON.stringify(updates)));
  // A memoQ XLIFF in its archive goes back into it.
  const merged = unwrap(
    core.xliffMerge(unwrap(core.archiveRead(bytes)), JSON.stringify(updates)),
  );
  return unwrap(core.archiveWrite(bytes, merged));
}

/** Packages of bilingual files: Trados project packages and memoQ handoff packages. */
export const SDL_PACKAGE_FILE = /\.(sdlppx|mqout)$/i;

/** The name of the package that goes back: .sdlrpx for Trados, .mqback for memoQ. */
export function returnPackageName(name: string): string {
  return /\.mqout$/i.test(name)
    ? name.replace(/\.mqout$/i, "") + ".mqback"
    : name.replace(/\.sdlppx$/i, "") + ".sdlrpx";
}

/** What a package is called, by its name. */
export function packageKind(name: string): string {
  return /\.mqout$/i.test(name) ? "handoff package" : "project package";
}
export const SDL_RETURN_TYPE = "application/octet-stream";

export interface SdlPackage {
  name: string;
  type: string;
  guid: string;
  sourceLang: string;
  targetLangs: string[];
  files: { path: string; lang: string; target: boolean; size: number }[];
  other: number;
}

export async function openSdlPackage(
  bytes: Uint8Array,
  name: string,
): Promise<SdlPackage> {
  const core = await loadCore();
  return JSON.parse(unwrap(core.packageOpen(bytes, name))) as SdlPackage;
}

/** A bilingual file of a package; a memoQ document comes out of its archive. */
export async function readSdlPackage(
  bytes: Uint8Array,
  name: string,
  path: string,
): Promise<Uint8Array> {
  const core = await loadCore();
  return unwrap(core.packageRead(bytes, name, path));
}

/** A return package (.sdlrpx, .mqback) with the translated files in place of the package's. */
export async function returnSdlPackage(
  bytes: Uint8Array,
  name: string,
  translated: Record<string, Uint8Array>,
): Promise<Uint8Array> {
  const core = await loadCore();
  return unwrap(core.packageReturn(bytes, name, translated));
}

/** Offers bytes as a download. */
export function download(bytes: Uint8Array, name: string, type: string) {
  const url = URL.createObjectURL(new Blob([new Uint8Array(bytes)], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

export const DOCX_TYPE =
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
