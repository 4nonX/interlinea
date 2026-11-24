// What the web client keeps: projects, termbases, entries, memories and
// their settings, with the defaults of each.

import { confirmedShare, type Block, type Segment } from "./editor/model";
import { LISTED_LANGUAGES } from "./languages.gen";

export type LangCode = string;

export interface Language {
  code: LangCode;
  name: string;
  /** Native name, shown next to the English name. */
  native: string;
  dir?: "rtl";
}

/**
 * The languages offered by name: the list of the core (langtag/languages.go),
 * the same as in the desktop app. Any other language a file names still
 * works; lang() names it.
 */
export const LANGUAGES: Language[] = LISTED_LANGUAGES;

/** Languages written from right to left, for those outside the list. */
const RTL_LANGUAGES = new Set(["ar", "he", "fa", "ur", "ps", "sd", "ug", "yi", "dv", "ckb"]);

let displayNames: Intl.DisplayNames | null | undefined;

/** A language by code; codes outside the list get their English name from the browser (de becomes German). */
export function lang(code: LangCode): Language {
  const known = LANGUAGES.find((l) => l.code === code);
  if (known) return known;
  if (displayNames === undefined) {
    try {
      displayNames = new Intl.DisplayNames(["en"], { type: "language" });
    } catch {
      displayNames = null;
    }
  }
  let name = code;
  try {
    name = displayNames?.of(code) ?? code;
  } catch {
    // Not a valid language tag: show it as it is.
  }
  const rtl = RTL_LANGUAGES.has(code.split("-")[0]!.toLowerCase());
  return rtl ? { code, name, native: name, dir: "rtl" } : { code, name, native: name };
}

export function primary(code: LangCode): string {
  return code.split("-")[0]!.toLowerCase();
}

export type Location = "local" | "server";

export interface InfoType {
  id: string;
  name: string;
  /** Where the value lives: the whole concept, one language, or one term. */
  level: "concept" | "language" | "term";
  kind: "text" | "picklist";
  values?: string[];
}

export type TermStatus = "approved" | "proposed" | "forbidden";

export interface Term {
  text: string;
  status: TermStatus;
  fields?: Record<string, string>;
}

export interface Commit {
  id: string;
  at: string; // ISO date
  by: string;
  /** Plain-language description of the change. */
  summary: string;
  project?: string;
  /** Source segment the change came from, when added while translating. */
  segment?: string;
}

export interface Entry {
  id: string;
  termbaseId: string;
  concept: Record<string, string>;
  terms: Record<LangCode, Term[]>;
  /** Values of language-level fields, per language. */
  langFields?: Record<LangCode, Record<string, string>>;
  /** The entry's ID in the file it was imported from; used to match it on the next import. */
  externalId?: string;
  /** Increases with every change; used to detect conflicts on merge. */
  version: number;
  /** For entries in a project termbase that started as a copy of a master entry. */
  base?: { termbaseId: string; entryId: string; version: number };
  history: Commit[];
}

export interface Termbase {
  id: string;
  name: string;
  kind: "master" | "project";
  location: Location;
  server?: string;
  languages: LangCode[];
  client?: string;
  domain?: string;
  /** Project termbases: the masters they are based on. */
  basedOn?: string[];
  projectId?: string;
  infoTypes: InfoType[];
  /** A server termbase listed without its entries: how many it has. */
  entryCount?: number;
  /** A termbase of a team-server project, loaded from the server: never saved in the browser. */
  remote?: boolean;
  /** A server termbase opened on its own (from Terminology), not only for a project. */
  opened?: boolean;
  /** A server termbase: what the signed-in person may do in it. */
  role?: string;
  /** A master on the server: the project termbases based on it. */
  dependents?: string[];
}

export type SetupMode = "layered" | "own" | "direct";

export type ContextRule = "previous" | "both";

/** What a file of a project is for. */
export type FileRole = "translatable" | "reference";

/** How a file is read and writes its translation. */
/**
 * "docx" is a document one of the core's filters reads (Word, and by the
 * file's name PowerPoint, Excel, a web page or InDesign IDML), "xliff" a
 * bilingual file, "text" plain text, "none" a file that is not read.
 */
export type FileKind = "docx" | "xliff" | "text" | "none";

export interface ProjectFile {
  id: string;
  /** File name, with the folder it came from ("Anlagen/Hausordnung.docx"). */
  name: string;
  role: FileRole;
  words: number;
  kind: FileKind;
  /** A bilingual file is translated into this language only. */
  targetLang?: LangCode;
  /** Where the file sits in the project's Trados package. */
  packagePath?: string;
  /** The original file: needed to save the translation or open a reference file. */
  bytes?: Uint8Array;
  url?: string;
  /** Paragraph structure from the Word filter, for the preview. */
  blocks?: Block[];
  /** Segments per target language. Empty for reference files and unreadable ones. */
  segments: Record<LangCode, Segment[]>;
  /** What the filter changed or could not read. */
  warnings?: string[];
}

/** How a project uses a translation memory for one target language. */
export interface MemorySetting {
  /** The memory's name. */
  memory: string;
  lookup: boolean;
  concordance: boolean;
  update: boolean;
  /** Points taken off its match scores. */
  penalty: number;
}

/** A translation memory. Its units live in the core, saved as TMX between visits. */
export interface TranslationMemory {
  name: string;
  sourceLang: LangCode;
  targetLang: LangCode;
  client?: string;
  /** A project memory collects one project's confirmed translations. */
  projectId?: string;
  createdAt: string;
  /** Sample memories are filled from sample data instead of saved TMX. */
  sample?: boolean;
  /**
   * A project memory that starts with its project's confirmed segments:
   * the sample project, and projects saved before projects had memories.
   */
  seedFromProject?: boolean;
  /** A memory a team-server project uses, loaded from the server: never saved in the browser. */
  remote?: boolean;
}

/** A check of the project's own, as the core's QA runs it. */
export interface QaRule {
  name: string;
  /** Only where the source matches this (regular expression). */
  source?: string;
  /** What the translation must or must not match (regular expression). */
  target: string;
  mode: "must" | "must-not" | "count";
  severity?: "error" | "warning";
  message?: string;
}

export interface QaSettings {
  /** Core checks turned off that are on by default. */
  disabled: string[];
  /** Core checks turned on that are off by default. */
  enabled: string[];
  /** Warn when a fuzzy match is confirmed unchanged. */
  uneditedFuzzy: boolean;
  rules: QaRule[];
  /** Spelling dictionary per target language, by name. */
  dictionaries: Record<LangCode, string>;
  /** Words always spelled right in this project. */
  words: string[];
}

/** A Hunspell spelling dictionary. Its files are saved in the browser. */
export interface SpellDictionary {
  name: string;
  /** The language it spells. */
  lang: LangCode;
  words: number;
  addedAt: string;
}

export interface PrepareSettings {
  pretranslate: boolean;
  /** Lowest match inserted, 50 to 100. */
  minScore: number;
  /** Confirm segments filled with a 100% match (context matches included). */
  confirmExact: boolean;
  /** Confirm segments filled with a context match. */
  confirmContext: boolean;
}

export interface TermSetup {
  mode: SetupMode;
  masters: string[];
  projectTermbaseId?: string;
  /** Where the project termbase is kept. */
  location: Location;
  /** Direct mode: the master new terms go into. */
  writeTo?: string;
  /** Take in the masters' changes each time the project opens. */
  autoUpdate: boolean;
}

export interface Project {
  id: string;
  name: string;
  client?: string;
  description?: string;
  sourceLang: LangCode;
  targetLangs: LangCode[];
  files: ProjectFile[];
  /** Memories per target language, in priority order. */
  memories: Record<LangCode, MemorySetting[]>;
  /** The project has its own memory per target language. */
  projectMemory: boolean;
  setup: TermSetup;
  context: ContextRule;
  qa: QaSettings;
  prepare: PrepareSettings;
  createdAt: string;
  due?: string;
  /** Share of confirmed segments over all files and languages. */
  progress: number;
  /** The Trados package the project came from, to save one return package. */
  sdlPackage?: { bytes: Uint8Array; name: string };
  /** A project on the team server: opened from there, saved there, never in the browser. */
  remote?: RemoteProject;
}

/** Where a project on the team server is, and what the signed-in person may do. */
export interface RemoteProject {
  /** The server's number for the project. */
  id: number;
  /** The settings' version, for changing them. */
  version: number;
  role: string;
  assigned: boolean;
  /** The server's name for each file, by the file ID used here. */
  files: Record<string, string>;
}

/** What a new project can start from: everything but name, files and dates. */
export interface Template {
  id: string;
  name: string;
  client?: string;
  sourceLang: LangCode;
  targetLangs: LangCode[];
  memories: Record<LangCode, MemorySetting[]>;
  projectMemory: boolean;
  terminology: {
    mode: SetupMode;
    masters: string[];
    writeTo?: string;
    location: Location;
    autoUpdate: boolean;
  };
  context: ContextRule;
  qa: QaSettings;
  prepare: PrepareSettings;
  createdAt: string;
}

export const DEFAULT_QA: QaSettings = {
  disabled: [],
  enabled: [],
  uneditedFuzzy: true,
  rules: [],
  dictionaries: {},
  words: [],
};

export const DEFAULT_PREPARE: PrepareSettings = {
  pretranslate: false,
  minScore: 100,
  confirmExact: false,
  confirmContext: true,
};

/** Name of a project's own memory for a target language. */
export function projectMemoryName(projectName: string, target: LangCode) {
  return `${projectName} (${target})`;
}

/** Translatable segments of every file and language. */
export function allSegments(p: Project): Segment[] {
  return p.files.flatMap((f) => Object.values(f.segments).flat());
}

/** Share of confirmed segments, or the stored share for projects without readable text. */
export function projectProgress(p: Project): number {
  const segs = allSegments(p);
  return segs.length ? confirmedShare(segs) : p.progress;
}

/** The target languages a file is translated into. */
export function fileTargets(
  f: ProjectFile,
  p: Pick<Project, "targetLangs">,
): LangCode[] {
  if (f.role !== "translatable") return [];
  return f.targetLang ? [f.targetLang] : p.targetLangs;
}

/** Segments for another target language: the same sources, nothing translated. */
export function freshCopy(segments: Segment[]): Segment[] {
  return segments.map((s) => ({
    id: s.id,
    block: s.block,
    source: s.source,
    target: [],
    status: "new" as const,
    locked: s.locked,
    maxLength: s.maxLength,
  }));
}

export const DEFAULT_INFO_TYPES: InfoType[] = [
  {
    id: "domain",
    name: "Domain",
    level: "concept",
    kind: "picklist",
    values: [
      "Tenancy law",
      "Contract law",
      "Civil procedure",
      "Medical devices",
    ],
  },
  { id: "definition", name: "Definition", level: "concept", kind: "text" },
  {
    id: "grammar",
    name: "Grammar",
    level: "term",
    kind: "picklist",
    values: ["noun, masc.", "noun, fem.", "noun, neut.", "verb", "adjective"],
  },
  { id: "note", name: "Note", level: "term", kind: "text" },
];

/** The name work on this computer is signed with until the person sets their own. */
export const DEFAULT_USER_NAME = "Me";
