// The Interlinea core (Go) compiled to WebAssembly: the same Word filter the
// desktop app and the server use. Loaded on first use; about 1.3 MB
// compressed. Build it with `make wasm` in the repository root.

import {
  coreToRuns,
  plain,
  runsToCore,
  runsToCoreWhole,
  type Block,
  type CoreRun,
  type FileState,
  type Run,
  type Segment,
  type Timing,
} from "./editor/model";
import { tl } from "./i18n";
import { plural } from "./format";

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
  /** Subtitles: when the subtitle is shown. */
  timing?: Timing;
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
    fixed?: string,
    order?: string,
  ): Res<string>;
  merge(
    bytes: Uint8Array,
    lang: string,
    target: string,
    segments: string,
    options?: string,
  ): Res<Uint8Array>;
  resegment(how: string, segments: string, index: number, before: string): Res<string>;
  segmentation(segments: string, rules: string, lang: string): Res<string>;
  review(files: string): Res<string>;
  reviewCSV(files: string): Res<string>;
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
  tmInspect(bytes: Uint8Array): Res<string>;
  tmExportTMX(name: string): Res<Uint8Array>;
  tmExportWordfast(name: string): Res<Uint8Array>;
  // Translation services (package mt): the core builds each request and
  // reads each answer; the page sends the request.
  mtServices(): Res<string>;
  mtTranslate(settings: string, src: string, tgt: string, segments: string): Res<string>;
  mtAnswer(settings: string, segments: string, status: number, body: string): Res<string>;
  mtModels(settings: string): Res<string>;
  mtModelsAnswer(settings: string, status: number, body: string): Res<string>;
  mtCheck(settings: string): Res<string>;
  // Termbase functions: see termio.ts.
  xliffDetect(bytes: Uint8Array): Res<string>;
  xliffExtract(bytes: Uint8Array): Res<string>;
  xliffMerge(bytes: Uint8Array, updates: string, targetLang?: string): Res<Uint8Array>;
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
  pairs: { sourceSegments: number[]; targetSegments: number[]; score: number; fixed?: boolean }[];
}

/**
 * Pairs the segments of a document with those of its translation. Pairs
 * given as fixed stay, and the rest is paired around them.
 */
export function alignDocuments(
  c: CoreApi,
  source: { name: string; bytes: Uint8Array; lang: string },
  target: { name: string; bytes: Uint8Array; lang: string },
  fixed: { source: number[]; target: number[] }[] = [],
  /** The order each side is read in, by the segments' positions in the documents; none keeps the documents' order. */
  order?: { source: number[]; target: number[] },
): Alignment {
  return JSON.parse(
    unwrap(c.align(source.name, source.bytes, source.lang, target.name, target.bytes, target.lang, fixed.length ? JSON.stringify(fixed) : "", order ? JSON.stringify(order) : "")),
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
  importTMX(c: CoreApi, name: string, bytes: Uint8Array): MemoryImport {
    return JSON.parse(unwrap(c.tmImportTMX(name, bytes))) as MemoryImport;
  },
  /** Reads a memory file someone chose: TMX, Trados Studio or Wordfast. */
  importFile(c: CoreApi, name: string, bytes: Uint8Array): MemoryImport {
    return JSON.parse(unwrap(c.tmImportTMX(name, bytes))) as MemoryImport;
  },
  /** What a memory file holds, without storing it. */
  inspectFile(c: CoreApi, bytes: Uint8Array): MemoryFileInfo {
    return JSON.parse(unwrap(c.tmInspect(bytes))) as MemoryFileInfo;
  },
  exportTMX(c: CoreApi, name: string): Uint8Array {
    return unwrap(c.tmExportTMX(name));
  },
  /** The memory as a Wordfast memory (UTF-16 text). */
  exportWordfast(c: CoreApi, name: string): Uint8Array {
    return unwrap(c.tmExportWordfast(name));
  },
};

/** What an import of a memory file did. */
export interface MemoryImport {
  format: string;
  read: number;
  stored: number;
  skipped: number;
  /** Why nothing was stored, when the file is for other languages. */
  note?: string;
}

/** What a memory file holds: its format and what it says of itself. */
export interface MemoryFileInfo {
  format: string;
  formatName: string;
  name?: string;
  sourceLang?: string;
  targetLang?: string;
  description?: string;
  units: number;
  skipped: number;
  warnings: string[];
}

/** The memory files people can choose, for a file input's accept. */
export const MEMORY_FILES =
  ".tmx,.sdltm,.txt,application/x-tmx+xml,text/xml,text/plain";

/** The type of a Wordfast memory, for a download. */
export const WORDFAST_TYPE = "text/plain;charset=utf-16";

/** A memory's name as a file name, without what file systems refuse. */
export function memoryFileName(name: string, ext: string): string {
  return `${name.replace(/[^\p{L}\p{N}]+/gu, "_")}.${ext}`;
}

/** The memory files people can choose, in words. */
export const MEMORY_FILES_NOTE =
  tl("A memory file: .tmx, .sdltm or .txt.");

/** What an import says it did, for a notice. */
export function importNotice(st: MemoryImport, into: string, from?: string): string {
  if (st.stored === 0 && st.note)
    return from
      ? tl("Nothing was imported from {from}. {note}", { from, note: st.note })
      : tl("Nothing was imported. {note}", { note: st.note });
  const units = plural(st.stored, "unit", "units");
  const skipped = st.skipped ? tl("; {n} without both languages skipped", { n: st.skipped }) : "";
  return from
    ? tl("Imported {units} from {from} into {into}{skipped}", { units, from, into, skipped })
    : tl("Imported {units} into {into}{skipped}", { units, into, skipped });
}

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

// The fonts a PDF is set in, fetched once, when one is made.
let fontFiles: Promise<Uint8Array[]> | null = null;
export function pdfFonts(): Promise<Uint8Array[]> {
  const names = [
    "NotoSans-Regular.ttf",
    "NotoSans-Bold.ttf",
    "NotoSans-Italic.ttf",
    "NotoSans-BoldItalic.ttf",
    "NotoSansArabic-Regular.ttf",
    "NotoSansArabic-Bold.ttf",
    "NotoSansHebrew-Regular.ttf",
    "NotoSansHebrew-Bold.ttf",
  ];
  fontFiles ??= Promise.all(
    names.map(async (n) => {
      const r = await fetch(`${base()}core/fonts/${n}`);
      if (!r.ok) throw new Error(tl("The font {n} could not be loaded.", { n }));
      return new Uint8Array(await r.arrayBuffer());
    }),
  ).catch((e) => {
    fontFiles = null;
    throw e;
  });
  return fontFiles;
}

function base(): string {
  return new URL("./", document.baseURI).href;
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(tl("Could not load {src}", { src })));
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
        tl("The core is missing ({status}). Run \"make wasm\" in the repository.", { status: res.status }),
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
export type DocumentFormat =
  | "docx"
  | "pptx"
  | "xlsx"
  | "html"
  | "idml"
  | "odt"
  | "ods"
  | "odp"
  | "xml"
  | "md"
  | "properties"
  | "json"
  | "yaml"
  | "po"
  | "resx"
  | "srt"
  | "vtt"
  | "sbv"
  | "ass"
  | "sub"
  | "ttml"
  | "stl"
  | "scc"
  | "txt"
  | "pdf";

const DOCUMENT_FORMATS: [RegExp, DocumentFormat][] = [
  [/\.(docx|docm)$/i, "docx"],
  [/\.(pptx|pptm|ppsx)$/i, "pptx"],
  [/\.(xlsx|xlsm)$/i, "xlsx"],
  [/\.(html?|xhtml)$/i, "html"],
  [/\.idml$/i, "idml"],
  [/\.odt$/i, "odt"],
  [/\.ods$/i, "ods"],
  [/\.odp$/i, "odp"],
  [/\.xml$/i, "xml"],
  [/\.(md|markdown)$/i, "md"],
  [/\.properties$/i, "properties"],
  [/\.json$/i, "json"],
  [/\.ya?ml$/i, "yaml"],
  [/\.pot?$/i, "po"],
  [/\.resx$/i, "resx"],
  [/\.srt$/i, "srt"],
  [/\.vtt$/i, "vtt"],
  [/\.sbv$/i, "sbv"],
  [/\.(ass|ssa)$/i, "ass"],
  [/\.sub$/i, "sub"],
  [/\.(ttml|dfxp|itt)$/i, "ttml"],
  [/\.stl$/i, "stl"],
  [/\.scc$/i, "scc"],
  [/\.(txt|csv|tsv)$/i, "txt"],
  [/\.pdf$/i, "pdf"],
];

/** The format of a document one of the core's filters reads; null for any other file. */
export function documentFormat(name: string): DocumentFormat | null {
  return DOCUMENT_FORMATS.find(([re]) => re.test(name))?.[1] ?? null;
}

/** What a document format is called. */
export const DOCUMENT_FORMAT_NAME: Record<DocumentFormat, string> = {
  docx: tl("DOCX document"),
  pptx: tl("PPTX presentation"),
  xlsx: tl("XLSX workbook"),
  html: tl("Web page"),
  idml: tl("IDML document"),
  odt: tl("ODT document"),
  ods: tl("ODS spreadsheet"),
  odp: tl("ODP presentation"),
  xml: tl("XML file"),
  md: tl("Markdown file"),
  properties: tl("Properties file"),
  json: tl("JSON file"),
  yaml: tl("YAML file"),
  po: tl("PO file"),
  resx: tl("RESX file"),
  srt: tl("SRT subtitles"),
  vtt: tl("WebVTT subtitles"),
  sbv: tl("SBV subtitles"),
  ass: tl("ASS subtitles"),
  sub: tl("SUB subtitles"),
  ttml: tl("TTML subtitles"),
  stl: tl("EBU STL subtitles"),
  scc: tl("SCC captions"),
  txt: tl("Text file"),
  pdf: tl("PDF document"),
};

/**
 * The name a document's translation is saved under: the file's own with
 * the language in it. A PDF's translation is the DOCX document its text
 * was read into.
 */
export function translatedName(name: string, target: string): string {
  const own = name.split("/").pop() ?? name;
  const dot = own.lastIndexOf(".");
  const ext = documentFormat(own) === "pdf" ? ".docx" : own.slice(dot);
  return `${own.slice(0, dot)}_${target}${ext}`;
}

/** What a document format keeps of the file besides its text. */
export const DOCUMENT_FORMAT_KEEPS: Record<DocumentFormat, string> = {
  docx: tl("formatting kept"),
  pptx: tl("formatting kept"),
  xlsx: tl("formatting kept"),
  html: tl("markup kept"),
  idml: tl("layout kept"),
  odt: tl("formatting kept"),
  ods: tl("formatting kept"),
  odp: tl("formatting kept"),
  xml: tl("markup kept"),
  md: tl("markup kept"),
  properties: tl("keys and placeholders kept"),
  json: tl("keys and placeholders kept"),
  yaml: tl("keys and placeholders kept"),
  po: tl("keys and placeholders kept"),
  resx: tl("keys and placeholders kept"),
  srt: tl("times kept"),
  vtt: tl("times kept"),
  sbv: tl("times kept"),
  ass: tl("times and styles kept"),
  sub: tl("frames kept"),
  ttml: tl("times and styles kept"),
  stl: tl("times and positions kept"),
  scc: tl("times and positions kept"),
  txt: tl("line breaks kept"),
  pdf: tl("text only, saved as a DOCX document"),
};

/** The type a translated document is offered for download as. */
export const DOCUMENT_TYPE: Record<DocumentFormat, string> = {
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  html: "text/html",
  idml: "application/vnd.adobe.indesign-idml-package",
  odt: "application/vnd.oasis.opendocument.text",
  ods: "application/vnd.oasis.opendocument.spreadsheet",
  odp: "application/vnd.oasis.opendocument.presentation",
  xml: "application/xml",
  md: "text/markdown",
  properties: "text/plain",
  json: "application/json",
  yaml: "application/yaml",
  po: "text/x-gettext-translation",
  resx: "application/xml",
  srt: "application/x-subrip",
  vtt: "text/vtt",
  sbv: "text/plain",
  ass: "text/x-ssa",
  sub: "text/plain",
  ttml: "application/ttml+xml",
  stl: "application/octet-stream",
  scc: "text/plain",
  txt: "text/plain",
  pdf: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
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
      ...(u.notes?.length ? { notes: u.notes } : {}),
      ...(u.maxLength ? { maxLength: u.maxLength } : {}),
      ...(u.timing ? { timing: u.timing } : {}),
    })),
    blocks: ex.blocks,
    warnings: ex.warnings ?? [],
  };
}

/** A segment as the core takes it, with how it was split or joined. */
function coreSegment(s: Segment) {
  return {
    id: s.id,
    source: runsToCoreWhole(s.source),
    target: runsToCoreWhole(s.target),
    status: s.status,
    locked: !!s.locked && !s.lockedReason,
    block: s.block,
    maxLength: s.maxLength,
    timing: s.timing,
    notes: s.notes,
    comments: s.comments,
    issues: s.issues,
    file: s.file,
    split: s.split,
    joined: s.joined?.map((j) => ({ id: j.id, source: runsToCoreWhole(j.source) })),
  };
}

/** A segment as the core gives it back. */
interface CoreCut {
  id: string;
  source: CoreRun[];
  target: CoreRun[] | null;
  status: Segment["status"];
  block?: string;
  maxLength?: number;
  timing?: Timing;
  notes?: string[];
  comments?: Segment["comments"];
  issues?: Segment["issues"];
  split?: Segment["split"];
  joined?: { id: string; source: CoreRun[] }[];
}

function segmentOfCut(x: CoreCut, rev?: number): Segment {
  return {
    rev,
    id: x.id,
    source: coreToRuns(x.source),
    target: coreToRuns(x.target ?? []),
    status: x.status,
    block: x.block,
    maxLength: x.maxLength || undefined,
    timing: x.timing ?? undefined,
    notes: x.notes?.length ? x.notes : undefined,
    comments: x.comments?.length ? x.comments : undefined,
    issues: x.issues?.length ? x.issues : undefined,
    split: x.split ?? undefined,
    joined: x.joined?.length ? x.joined.map((j) => ({ id: j.id, source: coreToRuns(j.source) })) : undefined,
  };
}

/**
 * A document's segments cut as the project's own segmentation rules say:
 * joined where a rule says a sentence does not end, split where one says
 * it does. Segments the rules leave alone are the same objects.
 */
export async function applySegmentation(segments: Segment[], rules: { break: boolean; before?: string; after?: string }[], lang: string): Promise<Segment[]> {
  const core = await loadCore();
  const r = core.segmentation(JSON.stringify(segments.map(coreSegment)), JSON.stringify(rules), lang);
  if (!r.ok) throw new Error(r.error);
  const had = new Map(segments.map((x) => [x.id, x]));
  return (JSON.parse(r.value) as CoreCut[]).map((x) => (!x.split && !x.joined?.length && had.get(x.id)) || segmentOfCut(x));
}

/** The score of segments from the issues a reviewer marked on them. */
export interface Quality {
  words: number;
  segments: number;
  marked: number;
  issues: number;
  minor: number;
  major: number;
  critical: number;
  categories: Record<string, number>;
  penalty: number;
  perThousand: number;
  score: number;
}

/** The scored review of files: each one's score, and all together. */
export interface Review {
  files: ({ file: string; lang: string } & Quality)[];
  total: Quality;
}

type Reviewed = { name: string; lang: string; segments: Segment[] }[];
const reviewed = (files: Reviewed) => JSON.stringify(files.map((f) => ({ name: f.name, lang: f.lang, segments: f.segments.map(coreSegment) })));

/** Scores files in their target languages by the issues marked on their segments. */
export async function reviewOf(files: Reviewed): Promise<Review> {
  const core = await loadCore();
  return JSON.parse(unwrap(core.review(reviewed(files)))) as Review;
}

/** Every issue of the files as a CSV table, with its segment's number, source and translation. */
export async function reviewCsv(files: Reviewed): Promise<string> {
  const core = await loadCore();
  return unwrap(core.reviewCSV(reviewed(files)));
}

/** How a file's segments are cut differently. */
export type Resegment = "split" | "join";

/** What a segment is, to tell which segments an operation left alone. */
function cutKey(id: string, source: string, split: Segment["split"], joined: { id: string }[] | undefined): string {
  return JSON.stringify([id, source, split?.unit ?? "", split?.space ?? "", (joined ?? []).map((j) => j.id)]);
}

/**
 * Splits segment index where before, the start of its source, ends, or
 * joins it with the one after it. The answer is the file's segments as
 * they are then: those the change left alone are the same objects. Other
 * parts of the same stretch of the file may be named anew. What the
 * segments do not allow is thrown, as the sentence to show.
 */
export async function resegment(how: Resegment, segments: Segment[], index: number, before: Run[] = []): Promise<Segment[]> {
  const core = await loadCore();
  const r = core.resegment(how, JSON.stringify(segments.map(coreSegment)), index, JSON.stringify(runsToCoreWhole(before)));
  if (!r.ok) throw new Error(r.error);
  const out = JSON.parse(r.value) as CoreCut[];
  const was = segments.map((x) => cutKey(x.id, plain(x.source), x.split, x.joined) + JSON.stringify(plain(x.target)));
  const is = out.map((x) => cutKey(x.id, plain(coreToRuns(x.source)), x.split ?? undefined, x.joined) + JSON.stringify(plain(coreToRuns(x.target ?? []))));
  let lo = 0;
  while (lo < was.length && lo < is.length && was[lo] === is[lo]) lo++;
  let hiOld = was.length;
  let hiNew = is.length;
  while (hiOld > lo && hiNew > lo && was[hiOld - 1] === is[hiNew - 1]) {
    hiOld--;
    hiNew--;
  }
  const gone = segments.slice(lo, hiOld);
  // Counted on, so an open editor shows what the segment is now.
  const rev = Math.max(0, ...gone.map((x) => x.rev ?? 0)) + 1;
  const fresh = out.slice(lo, hiNew).map((x) => {
    const seg = segmentOfCut(x, rev);
    // A segment that only got another name keeps where its translation came from.
    const same = gone.find((g) => plain(g.source) === plain(seg.source) && plain(g.target) === plain(seg.target));
    if (same) {
      seg.origin = same.origin;
      seg.warnings = same.warnings;
    }
    return seg;
  });
  return [...segments.slice(0, lo), ...fresh, ...segments.slice(hiOld)];
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
  const r = core.merge(
    bytes,
    lang,
    target,
    JSON.stringify(segments.map(coreSegment)),
    JSON.stringify({ format: documentFormat(name) ?? "docx" }),
  );
  if (!r.ok) throw new Error(r.error);
  return r.value;
}

/**
 * File names the XLIFF filter reads: XLIFF 1.2 and 2, Trados SDLXLIFF, memoQ MQXLIFF, also in its archive,
 * Phrase, Wordfast, TagEditor, and the RTF tables for review of memoQ and Déjà Vu.
 * A Word document can be a table for review too: see bilingualDocx.
 */
export const XLIFF_FILE = /\.(xlf|xliff|sdlxliff|mqxliff|mxliff|txlf|txml|ttx|mqxlz|rtf)$/i;

/** Whether the bytes are a ZIP file: a memoQ XLIFF in its archive (.mqxlz), or a Word document. */
function archived(bytes: Uint8Array): boolean {
  return bytes.length > 3 && bytes[0] === 0x50 && bytes[1] === 0x4b && bytes[2] === 0x03 && bytes[3] === 0x04;
}
export const XLIFF_TYPE = "application/xliff+xml";

/** The type a bilingual file is saved as: a table for review is a Word document or RTF. */
export function bilingualType(name: string): string {
  if (/\.docx$/i.test(name)) return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
  if (/\.rtf$/i.test(name)) return "application/rtf";
  return XLIFF_TYPE;
}

/** The bilingual file in the bytes: a memoQ archive's XLIFF, or the bytes themselves. */
function bilingualBytes(core: CoreApi, bytes: Uint8Array): Uint8Array {
  if (!archived(bytes) || isBilingual(core, bytes)) return bytes;
  return unwrap(core.archiveRead(bytes));
}

/** Whether a Word document is a bilingual table for review: a Trados review document or a memoQ two-column DOCX. */
export async function bilingualDocx(bytes: Uint8Array): Promise<boolean> {
  if (!archived(bytes)) return false;
  return isBilingual(await loadCore(), bytes);
}

/** Whether the core takes the bytes for a bilingual file: it names a format, and names none ("") for others. */
function isBilingual(core: CoreApi, bytes: Uint8Array): boolean {
  const r = core.xliffDetect(bytes);
  return r.ok && r.value !== "";
}

/** What a bilingual file holds besides its segments. */
export interface Bilingual extends Extracted {
  /**
   * The variant: "xliff-1.2", "sdlxliff", "mqxliff", "mxliff" (Phrase), "txml" (Wordfast Pro), "ttx" (TagEditor),
   * "xliff-2", or a table for review: "sdlreview" (Trados), "mqrtf" and "mqdocx" (memoQ), "dvrtf" (Déjà Vu).
   */
  format: string;
  sourceLang: string;
  targetLang: string;
}

export const XLIFF_FORMAT_NAME: Record<string, string> = {
  "xliff-1.2": tl("XLIFF 1.2"),
  "xliff-2": tl("XLIFF 2"),
  sdlxliff: ".sdlxliff",
  mqxliff: ".mqxliff",
  mxliff: ".mxliff",
  txml: ".txml",
  ttx: ".ttx",
  sdlreview: tl("Review table (.docx)"),
  mqrtf: tl("Two-column table (.rtf)"),
  mqdocx: tl("Two-column table (.docx)"),
  dvrtf: tl("External view (.rtf)"),
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

/** Bilingual formats whose program keeps its own record of every segment. */
const FIXED_FORMATS = new Set(["sdlxliff", "mqxliff", "mxliff", "txml", "ttx", "sdlreview", "mqrtf", "mqdocx", "dvrtf"]);

/** Reads a bilingual XLIFF file with its translations and states. */
export async function extractXliff(bytes: Uint8Array): Promise<Bilingual> {
  const core = await loadCore();
  const doc = bilingualBytes(core, bytes);
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
        timing: u.timing ?? undefined,
        file: {
          fixed: FIXED_FORMATS.has(ex.format ?? "") || undefined,
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

/**
 * Whether a bilingual file comes from a program that keeps its own record
 * of every segment (Trados, memoQ, Phrase, Wordfast, TagEditor, Déjà Vu), so its segments are not split or joined.
 */
export async function keepsItsSegments(bytes: Uint8Array): Promise<boolean> {
  const core = await loadCore();
  const doc = bilingualBytes(core, bytes);
  const r = core.xliffDetect(doc);
  return !r.ok || FIXED_FORMATS.has(r.value);
}

/** Writes translations and states into the original XLIFF file. Locked segments stay as they are. */
export async function mergeXliff(
  bytes: Uint8Array,
  segments: Segment[],
  /** The language a file that names none (an unsegmented TagEditor file) is translated into. */
  targetLang?: string,
): Promise<Uint8Array> {
  const core = await loadCore();
  // Each segment with its state in the file; the core writes segments
  // that were split or joined as the file has them.
  const updates = segments.map((s) => ({ ...coreSegment(s), target: runsToCore(s.target), locked: !!s.locked, file: undefined, state: fileState(s) }));
  if (bilingualBytes(core, bytes) === bytes)
    return unwrap(core.xliffMerge(bytes, JSON.stringify(updates), targetLang ?? ""));
  // A memoQ XLIFF in its archive goes back into it.
  const merged = unwrap(
    core.xliffMerge(unwrap(core.archiveRead(bytes)), JSON.stringify(updates), targetLang ?? ""),
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
  return /\.mqout$/i.test(name) ? tl("handoff package") : tl("project package");
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
