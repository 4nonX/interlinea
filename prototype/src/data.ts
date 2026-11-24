// Mock data for the prototype. Every name, entry and server address here is
// synthetic demonstration material.

import {
  HOFMANN_BLOCKS,
  HOFMANN_FILE,
  HOFMANN_SEGMENTS,
  confirmedShare,
  wordCount,
  type Block,
  type Segment,
} from "./editor/model";
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
  /** Entries that exist on the server but are not part of this prototype's data. */
  extraEntries?: number;
  /** A termbase of a team-server project, loaded from the server: never saved in the browser. */
  remote?: boolean;
  /** A server termbase opened on its own (from Terminology), not only for a project. */
  opened?: boolean;
}

export type SetupMode = "layered" | "own" | "direct";

export type ContextRule = "previous" | "both";

/** What a file of a project is for. */
export type FileRole = "translatable" | "reference";

/** How the prototype reads a file and writes its translation. */
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
  role: "" | "read" | "suggest" | "edit" | "approve" | "manage";
  assigned: boolean;
  /** The server's name for each file, by the prototype's file ID. */
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

export const SERVER = "terms.kanzlei-weber.example";

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

const d = (s: string) => new Date(s).toISOString();

export const INITIAL_TERMBASES: Termbase[] = [
  {
    id: "tb-weber",
    name: "Kanzlei Weber",
    kind: "master",
    location: "server",
    server: SERVER,
    languages: ["de-DE", "en-GB"],
    client: "Kanzlei Weber",
    domain: "Tenancy law",
    infoTypes: DEFAULT_INFO_TYPES,
    extraEntries: 1268,
  },
  {
    id: "tb-mietrecht",
    name: "Mietrecht",
    kind: "master",
    location: "local",
    languages: ["de-DE", "en-GB", "fr-FR"],
    domain: "Tenancy law",
    infoTypes: DEFAULT_INFO_TYPES.filter((t) => t.id !== "grammar"),
    extraEntries: 412,
  },
  {
    id: "tb-medtech",
    name: "Medizinprodukte",
    kind: "master",
    location: "local",
    languages: ["de-DE", "en-US"],
    domain: "Medical devices",
    infoTypes: DEFAULT_INFO_TYPES,
    extraEntries: 2031,
  },
  {
    id: "tb-p-weber-lease",
    name: "Mietvertrag Hofmann (project)",
    kind: "project",
    location: "server",
    server: SERVER,
    languages: ["de-DE", "en-GB"],
    client: "Kanzlei Weber",
    basedOn: ["tb-weber"],
    projectId: "p-hofmann",
    infoTypes: DEFAULT_INFO_TYPES,
  },
];

const t = (
  text: string,
  status: TermStatus = "approved",
  fields?: Record<string, string>,
): Term => ({ text, status, fields });

export const INITIAL_ENTRIES: Entry[] = [
  // Master: Kanzlei Weber
  {
    id: "e1",
    termbaseId: "tb-weber",
    version: 3,
    concept: {
      domain: "Tenancy law",
      definition:
        "Die Sache, die der Vermieter dem Mieter zum Gebrauch überlässt.",
    },
    terms: {
      "de-DE": [t("Mietsache", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("leased property"), t("rented property", "proposed")],
    },
    history: [
      {
        id: "c1",
        at: d("2025-03-04T09:12:00Z"),
        by: "Dan",
        summary: "Created entry",
        project: "Gewerbemietvertrag Lang",
      },
      {
        id: "c2",
        at: d("2025-09-18T14:40:00Z"),
        by: "Dan",
        summary: "Added English synonym “rented property” (proposed)",
        project: "Mietvertrag Schuster",
      },
      {
        id: "c3",
        at: d("2026-02-02T11:05:00Z"),
        by: "Eva Brandt",
        summary: "Added definition",
      },
    ],
  },
  {
    id: "e2",
    termbaseId: "tb-weber",
    version: 2,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [
        t("Kaution", "approved", { grammar: "noun, fem." }),
        t("Mietsicherheit"),
      ],
      "en-GB": [t("deposit"), t("security deposit")],
    },
    history: [
      {
        id: "c4",
        at: d("2025-03-04T09:20:00Z"),
        by: "Dan",
        summary: "Created entry",
        project: "Gewerbemietvertrag Lang",
      },
      {
        id: "c5",
        at: d("2025-06-11T08:02:00Z"),
        by: "Dan",
        summary: "Added “Mietsicherheit” and “security deposit”",
      },
    ],
  },
  {
    id: "e3",
    termbaseId: "tb-weber",
    version: 1,
    concept: {
      domain: "Tenancy law",
      definition:
        "Kosten, die dem Eigentümer durch das Eigentum am Grundstück laufend entstehen und auf den Mieter umgelegt werden dürfen.",
    },
    terms: {
      "de-DE": [t("Betriebskosten", "approved", { grammar: "noun, pl." })],
      "en-GB": [
        t("service charges"),
        t("operating costs", "forbidden", {
          note: "Client prefers “service charges” in residential leases.",
        }),
      ],
    },
    history: [
      {
        id: "c6",
        at: d("2025-04-22T16:30:00Z"),
        by: "Dan",
        summary: "Created entry",
        project: "Mietvertrag Schuster",
      },
    ],
  },
  {
    id: "e4",
    termbaseId: "tb-weber",
    version: 2,
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("action for possession")],
    },
    history: [
      {
        id: "c7",
        at: d("2025-05-02T10:00:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
      // Changed on the master after the Hofmann project started: produces a conflict.
      {
        id: "c8",
        at: d("2026-11-18T15:22:00Z"),
        by: "Eva Brandt",
        summary: "Changed English term to “action for possession”",
      },
    ],
  },
  {
    id: "e5",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("ordentliche Kündigung")],
      "en-GB": [t("termination with notice")],
    },
    history: [
      {
        id: "c9",
        at: d("2025-07-14T09:45:00Z"),
        by: "Dan",
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e6",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("außerordentliche Kündigung")],
      "en-GB": [
        t("termination without notice"),
        t("extraordinary termination", "proposed"),
      ],
    },
    history: [
      {
        id: "c10",
        at: d("2025-07-14T09:47:00Z"),
        by: "Dan",
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e7",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Contract law" },
    terms: {
      "de-DE": [t("Schriftformklausel")],
      "en-GB": [t("written form clause")],
    },
    history: [
      {
        id: "c11",
        at: d("2026-11-20T08:10:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e8",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Contract law" },
    terms: {
      "de-DE": [t("salvatorische Klausel")],
      "en-GB": [t("severability clause")],
    },
    history: [
      {
        id: "c12",
        at: d("2026-11-20T08:14:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },

  // Project termbase: Mietvertrag Hofmann (branch of Kanzlei Weber)
  {
    id: "p1",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [
        t("Schönheitsreparaturen", "proposed", { grammar: "noun, pl." }),
      ],
      "en-GB": [t("decorative repairs", "proposed")],
    },
    history: [
      {
        id: "pc1",
        at: d("2026-11-19T10:31:00Z"),
        by: "Dan",
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment:
          "Der Mieter übernimmt die Schönheitsreparaturen auf eigene Kosten.",
      },
    ],
  },
  {
    id: "p2",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Staffelmiete", "proposed", { grammar: "noun, fem." })],
      "en-GB": [t("graduated rent", "proposed")],
    },
    history: [
      {
        id: "pc2",
        at: d("2026-11-19T11:02:00Z"),
        by: "Dan",
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Die Parteien vereinbaren eine Staffelmiete gemäß § 557a BGB.",
      },
    ],
  },
  {
    id: "p3",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Untervermietung", "proposed")],
      "en-GB": [t("subletting", "proposed")],
    },
    history: [
      {
        id: "pc3",
        at: d("2026-11-20T09:15:00Z"),
        by: "Dan",
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Eine Untervermietung bedarf der Zustimmung des Vermieters.",
      },
    ],
  },
  {
    id: "p4",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Mietminderung", "proposed")],
      "en-GB": [t("rent reduction", "proposed")],
    },
    history: [
      {
        id: "pc4",
        at: d("2026-11-20T14:48:00Z"),
        by: "Dan",
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Das Recht zur Mietminderung bleibt unberührt.",
      },
    ],
  },
  {
    // Changed copy of master e2 (clean change: master unchanged since).
    id: "p5",
    termbaseId: "tb-p-weber-lease",
    version: 3,
    base: { termbaseId: "tb-weber", entryId: "e2", version: 2 },
    concept: {
      domain: "Tenancy law",
      definition:
        "Sicherheitsleistung des Mieters für Ansprüche des Vermieters aus dem Mietverhältnis.",
    },
    terms: {
      "de-DE": [
        t("Kaution", "approved", { grammar: "noun, fem." }),
        t("Mietsicherheit"),
      ],
      "en-GB": [
        t("deposit"),
        t("security deposit"),
        t("rental deposit", "proposed"),
      ],
    },
    history: [
      {
        id: "pc5",
        at: d("2026-11-19T10:50:00Z"),
        by: "Dan",
        summary: "Added definition and “rental deposit” (proposed)",
        project: "Mietvertrag Hofmann",
        segment: "Die Kaution beträgt drei Nettokaltmieten.",
      },
    ],
  },
  {
    // Changed copy of master e4, but the master changed too: conflict.
    id: "p6",
    termbaseId: "tb-p-weber-lease",
    version: 2,
    base: { termbaseId: "tb-weber", entryId: "e4", version: 1 },
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("eviction action")],
    },
    history: [
      {
        id: "pc6",
        at: d("2026-11-17T16:05:00Z"),
        by: "Dan",
        summary: "Changed English term to “eviction action”",
        project: "Mietvertrag Hofmann",
        segment: "Der Vermieter kann Räumungsklage erheben.",
      },
    ],
  },
];

export const SAMPLE_MEMORY = "Kanzlei Weber (DE-EN)";

export const INITIAL_MEMORIES: TranslationMemory[] = [
  {
    name: SAMPLE_MEMORY,
    sourceLang: "de-DE",
    targetLang: "en-GB",
    client: "Kanzlei Weber",
    createdAt: d("2024-03-12T09:00:00Z"),
    sample: true,
  },
  {
    name: projectMemoryName("Mietvertrag Hofmann", "en-GB"),
    sourceLang: "de-DE",
    targetLang: "en-GB",
    client: "Kanzlei Weber",
    projectId: "p-hofmann",
    createdAt: d("2026-11-16T08:30:00Z"),
    seedFromProject: true,
  },
];

export const INITIAL_PROJECTS: Project[] = [
  {
    id: "p-hofmann",
    name: "Mietvertrag Hofmann",
    sourceLang: "de-DE",
    targetLangs: ["en-GB"],
    client: "Kanzlei Weber",
    files: [
      {
        id: "f-hofmann",
        name: "Mietvertrag_Hofmann_2026.docx",
        role: "translatable",
        words: HOFMANN_SEGMENTS.reduce((n, sg) => n + wordCount(sg.source), 0),
        kind: "docx",
        url: HOFMANN_FILE,
        blocks: HOFMANN_BLOCKS,
        segments: { "en-GB": HOFMANN_SEGMENTS },
      },
    ],
    memories: {
      "en-GB": [
        {
          memory: SAMPLE_MEMORY,
          lookup: true,
          concordance: true,
          update: false,
          penalty: 0,
        },
      ],
    },
    projectMemory: true,
    setup: {
      mode: "layered",
      masters: ["tb-weber"],
      projectTermbaseId: "tb-p-weber-lease",
      location: "server",
      autoUpdate: false,
    },
    context: "previous",
    qa: DEFAULT_QA,
    prepare: DEFAULT_PREPARE,
    createdAt: d("2026-11-16T08:30:00Z"),
    due: d("2026-11-26T16:00:00Z"),
    progress: confirmedShare(HOFMANN_SEGMENTS),
  },
  {
    id: "p-ifu",
    name: "Gebrauchsanweisung Infusionspumpe",
    sourceLang: "de-DE",
    targetLangs: ["en-US"],
    files: [
      {
        id: "f-ifu",
        name: "IFU_Infusionspumpe_v4.docx",
        role: "translatable",
        words: 11830,
        kind: "none",
        segments: {},
      },
    ],
    memories: {},
    projectMemory: false,
    setup: {
      mode: "direct",
      masters: ["tb-medtech"],
      writeTo: "tb-medtech",
      location: "local",
      autoUpdate: false,
    },
    context: "previous",
    qa: DEFAULT_QA,
    prepare: DEFAULT_PREPARE,
    createdAt: d("2026-11-10T13:00:00Z"),
    due: d("2026-12-04T12:00:00Z"),
    progress: 0.37,
  },
];

/**
 * Master entries as they were when the Hofmann project copied them. The
 * project changed "eviction proceedings" to "eviction action" while the
 * master changed it to "action for possession": a conflict.
 */
export const BASE_SNAPSHOTS: Record<string, Entry> = {
  p5: INITIAL_ENTRIES.find((e) => e.id === "e2")!,
  p6: {
    id: "e4",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("eviction proceedings")],
    },
    history: [
      {
        id: "c7",
        at: d("2025-05-02T10:00:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },
};

/** The moment the Hofmann project was branched from its master. */
export const BRANCHED_AT: Record<string, string> = {
  "p-hofmann": d("2026-11-16T08:30:00Z"),
};

export interface SampleDoc {
  file: string;
  words: number;
  text: string;
  client?: string;
}

/** Sample documents for the "New project" flow. Text is synthetic. */
export const SAMPLE_DOCS: SampleDoc[] = [
  {
    file: "Kaufvertrag_Mueller_Entwurf.docx",
    words: 4920,
    client: "Kanzlei Weber",
    text:
      "Kaufvertrag über ein Grundstück. Der Verkäufer verkauft dem Käufer das im Grundbuch eingetragene Grundstück mit allen Rechten und Pflichten. " +
      "Der Kaufpreis ist innerhalb von vierzehn Tagen nach Zugang der Mitteilung des Notars fällig. Die Übergabe erfolgt mit vollständiger Zahlung des Kaufpreises. " +
      "Der Käufer übernimmt die Grundsteuer ab dem Tag der Übergabe. Mündliche Nebenabreden bestehen nicht; Änderungen bedürfen der Schriftform.",
  },
  {
    file: "Lease_Agreement_Smith.pdf",
    words: 3310,
    text:
      "This lease agreement is made between the landlord and the tenant for the property described below. The tenant shall pay the rent in advance on the first day of each month. " +
      "The landlord may enter the property with reasonable notice to carry out repairs. Either party may terminate this agreement by giving not less than two months notice in writing.",
  },
  {
    file: "contrat_de_bail_Lyon.odt",
    words: 2775,
    text:
      "Le présent contrat de location est conclu entre le bailleur et le locataire pour le logement décrit ci-dessous. Le loyer est payable le premier jour de chaque mois. " +
      "Le locataire doit user paisiblement des locaux loués et répondre des dégradations qui surviennent pendant la durée du contrat.",
  },
  {
    file: "임대차계약서_서울.txt",
    words: 1840,
    text: "본 임대차계약은 임대인과 임차인 사이에 아래 표시 부동산에 관하여 체결한다. 임차인은 매월 말일까지 차임을 지급하여야 한다. 계약기간이 만료되면 임차인은 목적물을 원상으로 회복하여 반환한다.",
  },
  {
    file: "Scan_Urkunde_0412.pdf",
    words: 0,
    text: "",
  },
];

export const USER = "Dan";
