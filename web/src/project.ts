// Projects as the core sees them: the setup the core validates, the
// memories a target language uses, the analysis and pre-translation, and
// the quality checks. Everything here calls the core (WebAssembly), so the
// web client accepts and counts exactly what the desktop apps will.

import { unwrap, type CoreApi } from "./core";
import {
  type ContextRule,
  type FileRole,
  type LangCode,
  type Location,
  type MemorySetting,
  type PrepareSettings,
  type SegmentationRule,
  type Project,
  type ProjectFile,
  type QaSettings,
  type SetupMode,
  type SpellDictionary,
  type Template,
  type Termbase,
  type TranslationMemory,
} from "./data";
import {
  coreToRuns,
  plain,
  runsToCore,
  type CoreRun,
  type Segment,
} from "./editor/model";

/** A project being set up or edited. */
export interface Draft {
  name: string;
  client: string;
  description: string;
  /** ISO date and time, or empty. */
  due: string;
  sourceLang: LangCode;
  targetLangs: LangCode[];
  files: { name: string; role: FileRole; targetLang?: LangCode }[];
  memories: Record<LangCode, MemorySetting[]>;
  projectMemory: boolean;
  terminology: {
    mode: SetupMode;
    masters: string[];
    writeTo?: string;
    /** Masters that gain the project languages they lack. */
    addLanguages: string[];
    location: Location;
    autoUpdate: boolean;
  };
  context: ContextRule;
  qa: QaSettings;
  prepare: PrepareSettings;
  segmentation: SegmentationRule[];
}

export interface Problem {
  field: string;
  message: string;
}

export function draftOf(p: Project): Draft {
  return {
    name: p.name,
    client: p.client ?? "",
    description: p.description ?? "",
    due: p.due ?? "",
    sourceLang: p.sourceLang,
    targetLangs: p.targetLangs,
    files: p.files.map((f) => ({
      name: f.name,
      role: f.role,
      targetLang: f.targetLang,
    })),
    memories: p.memories,
    projectMemory: p.projectMemory,
    terminology: {
      mode: p.setup.mode,
      masters: p.setup.masters,
      writeTo: p.setup.writeTo,
      addLanguages: [],
      location: p.setup.location,
      autoUpdate: p.setup.autoUpdate,
    },
    context: p.context,
    qa: p.qa,
    prepare: p.prepare,
    segmentation: p.segmentation ?? [],
  };
}

export function draftOfTemplate(t: Template): Omit<Draft, "name" | "files"> {
  return {
    client: t.client ?? "",
    description: "",
    due: "",
    sourceLang: t.sourceLang,
    targetLangs: t.targetLangs,
    memories: t.memories,
    projectMemory: t.projectMemory,
    terminology: { ...t.terminology, addLanguages: [] },
    context: t.context,
    qa: t.qa,
    prepare: t.prepare,
    segmentation: t.segmentation ?? [],
  };
}

export function templateOf(
  id: string,
  name: string,
  d: Draft,
  now: string,
): Template {
  const { mode, masters, writeTo, location, autoUpdate } = d.terminology;
  return {
    id,
    name: name.trim(),
    client: d.client.trim() || undefined,
    sourceLang: d.sourceLang,
    targetLangs: d.targetLangs,
    memories: d.memories,
    projectMemory: d.projectMemory,
    terminology: { mode, masters, writeTo, location, autoUpdate },
    context: d.context,
    qa: d.qa,
    prepare: d.prepare,
    segmentation: d.segmentation.length ? d.segmentation : undefined,
    createdAt: now,
  };
}

/** A setup as the core keeps it. */
export function coreSettings(d: Omit<Draft, "files"> & { files: Draft["files"] }) {
  const t = d.terminology;
  return {
    name: d.name,
    client: d.client.trim(),
    description: d.description.trim(),
    due: d.due,
    sourceLang: d.sourceLang,
    targetLangs: d.targetLangs,
    files: d.files.map((f) => ({
      name: f.name,
      role: f.role,
      targetLang: f.targetLang ?? "",
    })),
    memories: d.memories,
    projectMemory: d.projectMemory,
    terminology: {
      mode: t.mode,
      masters: t.mode === "own" ? [] : t.masters,
      writeTo: t.mode === "direct" ? (t.writeTo ?? "") : "",
      addLanguages: t.addLanguages,
      location: t.location,
      autoUpdate: t.autoUpdate,
    },
    analysis: { context: d.context },
    qa: d.qa,
    prepare: d.prepare,
    segmentation: d.segmentation.length ? d.segmentation : undefined,
  };
}

export interface CatalogSource {
  memories: TranslationMemory[];
  termbases: Termbase[];
  dictionaries: SpellDictionary[];
}

function catalog(s: CatalogSource) {
  return {
    memories: Object.fromEntries(
      s.memories
        .filter((m) => !m.projectId)
        .map((m) => [
          m.name,
          { Name: m.name, SourceLang: m.sourceLang, TargetLang: m.targetLang },
        ]),
    ),
    termbases: Object.fromEntries(
      s.termbases
        .filter((t) => t.kind === "master")
        .map((t) => [t.id, t.languages]),
    ),
    dictionaries: Object.fromEntries(
      s.dictionaries.map((d) => [d.name, d.lang]),
    ),
  };
}

/** Everything wrong with a setup, as the core sees it. */
export function validate(c: CoreApi, d: Draft, from: CatalogSource): Problem[] {
  return JSON.parse(
    unwrap(
      c.projectValidate(
        JSON.stringify(coreSettings(d)),
        JSON.stringify(catalog(from)),
      ),
    ),
  ) as Problem[];
}

export function validateTemplate(
  c: CoreApi,
  t: Template,
  from: CatalogSource,
): Problem[] {
  const tpl = {
    ...coreSettings({
      ...draftOfTemplate(t),
      name: t.name,
      files: [],
    }),
    name: t.name,
  };
  return JSON.parse(
    unwrap(
      c.projectTemplateValidate(
        JSON.stringify(tpl),
        JSON.stringify(catalog(from)),
      ),
    ),
  ) as Problem[];
}

/** Messages of the problems with a field or anything under it. */
export function problemsAt(ps: Problem[], field: string): string[] {
  return ps
    .filter(
      (p) =>
        p.field === field ||
        p.field.startsWith(field + ".") ||
        p.field.startsWith(field + "["),
    )
    .map((p) => p.message);
}

/** A memory as a target language of a project uses it. */
export interface MemoryRef {
  name: string;
  penalty: number;
  /** The project's own memory. */
  project: boolean;
}

/** A project's own memory for a target language, if it has one. */
export function projectMemoryOf(
  p: Pick<Project, "id" | "projectMemory">,
  lang: LangCode,
  all: TranslationMemory[],
): TranslationMemory | undefined {
  if (!p.projectMemory) return undefined;
  return all.find((m) => m.projectId === p.id && m.targetLang === lang);
}

/**
 * The memories a target language uses for lookup, concordance or
 * updating, in priority order: the project memory first (it holds this
 * project's own confirmed translations), then the chosen memories.
 */
export function memoryRefs(
  p: Pick<Project, "id" | "memories" | "projectMemory">,
  lang: LangCode,
  use: "lookup" | "concordance" | "update",
  all: TranslationMemory[],
): MemoryRef[] {
  const out: MemoryRef[] = [];
  const own = projectMemoryOf(p, lang, all);
  if (own) out.push({ name: own.name, penalty: 0, project: true });
  for (const m of p.memories[lang] ?? []) {
    if (m[use])
      out.push({
        name: m.memory,
        penalty: use === "lookup" ? m.penalty : 0,
        project: false,
      });
  }
  return out;
}

function analysisFiles(files: ProjectFile[], lang: LangCode) {
  return files
    .filter((f) => f.role === "translatable" && f.segments[lang])
    .map((f) => ({
      name: f.name,
      segments: f.segments[lang]!.map((s) => ({
        id: s.id,
        source: runsToCore(s.source),
        locked: !!s.locked,
      })),
    }));
}

export type CoreBand =
  | "locked"
  | "context"
  | "repetition"
  | "cross-file"
  | "100"
  | "95"
  | "85"
  | "75"
  | "50"
  | "new";

export interface CoreCounts {
  segments: number;
  words: number;
  characters: number;
  tags: number;
  placeables: number;
}

export interface CoreRow extends CoreCounts {
  band: CoreBand;
  name: string;
}

export interface CoreReport {
  memories: string[];
  files: { name: string; rows: CoreRow[]; total: CoreCounts }[];
  rows: CoreRow[];
  total: CoreCounts;
  /** How each segment was counted, with the score and memory of its best match. */
  segments: { file: string; id: string; band: CoreBand; score?: number; memory?: string }[];
}

/** The core's analysis of the files in one target language. */
export function analyzeFiles(
  c: CoreApi,
  files: ProjectFile[],
  sourceLang: LangCode,
  lang: LangCode,
  memories: MemoryRef[],
  context: ContextRule,
): CoreReport {
  return JSON.parse(
    unwrap(
      c.analyze(
        JSON.stringify(analysisFiles(files, lang)),
        JSON.stringify(
          memories.map(({ name, penalty }) => ({ name, penalty })),
        ),
        JSON.stringify({ sourceLang, targetLang: lang, context }),
      ),
    ),
  ) as CoreReport;
}

interface CoreTranslation {
  file: string;
  id: string;
  target: CoreRun[];
  score: number;
  memory: string;
  context: boolean;
  status: "draft" | "confirmed";
}

/**
 * Pre-translates one target language of the files and returns them with
 * the matches inserted; untouched files are returned as they were. The
 * count says how many segments were filled.
 */
export function pretranslateFiles(
  c: CoreApi,
  files: ProjectFile[],
  lang: LangCode,
  memories: MemoryRef[],
  prepare: PrepareSettings,
  context: ContextRule,
): { files: ProjectFile[]; filled: number; confirmed: number } {
  if (!memories.length) return { files, filled: 0, confirmed: 0 };
  const input = files
    .filter((f) => f.role === "translatable" && f.segments[lang])
    .map((f) => ({
      name: f.name,
      segments: f.segments[lang]!.map((s) => ({
        id: s.id,
        source: runsToCore(s.source),
        target: runsToCore(s.target),
        status: s.status,
        locked: !!s.locked,
      })),
    }));
  const out = JSON.parse(
    unwrap(
      c.pretranslate(
        JSON.stringify(input),
        JSON.stringify(
          memories.map(({ name, penalty }) => ({ name, penalty })),
        ),
        JSON.stringify(prepare),
        context,
      ),
    ),
  ) as CoreTranslation[];
  const byFile = new Map<string, Map<string, CoreTranslation>>();
  for (const t of out) {
    if (!byFile.has(t.file)) byFile.set(t.file, new Map());
    byFile.get(t.file)!.set(t.id, t);
  }
  let confirmed = 0;
  const next = files.map((f) => {
    const ts = byFile.get(f.name);
    if (!ts) return f;
    const segs = f.segments[lang]!.map((s): Segment => {
      const t = ts.get(s.id);
      if (!t) return s;
      const target = coreToRuns(t.target);
      if (t.status === "confirmed") confirmed++;
      return {
        ...s,
        target,
        status: t.status,
        origin: {
          kind: "tm",
          score: t.score,
          inserted: plain(target),
          context: t.context,
        },
      };
    });
    return { ...f, segments: { ...f.segments, [lang]: segs } };
  });
  return { files: next, filled: out.length, confirmed };
}

export interface QaKind {
  kind: string;
  name: string;
  default: boolean;
  description: string;
}

let kinds: QaKind[] | null = null;

/** The core's quality checks. */
export function qaKinds(c: CoreApi): QaKind[] {
  kinds ??= JSON.parse(unwrap(c.qaKinds())) as QaKind[];
  return kinds;
}

/** Whether a check runs under the project's settings. */
export function checkOn(q: QaSettings, k: QaKind): boolean {
  return k.default ? !q.disabled.includes(k.kind) : q.enabled.includes(k.kind);
}

/** Turns a check on or off, as a change to the project's settings. */
export function setCheck(q: QaSettings, k: QaKind, on: boolean): QaSettings {
  const disabled = q.disabled.filter((x) => x !== k.kind);
  const enabled = q.enabled.filter((x) => x !== k.kind);
  if (k.default && !on) disabled.push(k.kind);
  if (!k.default && on) enabled.push(k.kind);
  return { ...q, disabled, enabled };
}

export interface CoreIssue {
  segment: string;
  kind: string;
  severity: string;
  message: string;
  found?: string;
  /** Spelling: the dictionary's suggestions, best first. */
  suggestions?: string[];
}

/**
 * The core's checks of one segment's translation. The file's other
 * segments go along, so consistency with them is checked too. dictionary
 * is the name of a spelling dictionary loaded in the core, or empty.
 */
export function checkSegment(
  c: CoreApi,
  segments: Segment[],
  id: string,
  sourceLang: LangCode,
  targetLang: LangCode,
  q: QaSettings,
  termIndex?: string,
  dictionary?: string,
): CoreIssue[] {
  const issues = JSON.parse(
    unwrap(
      c.qaCheck(
        JSON.stringify(
          segments
            .filter((s) => s.id === id || (plain(s.target).trim() && !s.locked))
            .map((s) => ({
              id: s.id,
              source: runsToCore(s.source),
              target: runsToCore(s.target),
              maxLength: s.maxLength ?? 0,
              // A part of a split subtitle has the time of the whole, not its own.
              timing: s.split ? undefined : s.timing,
            })),
        ),
        JSON.stringify({
          sourceLang,
          targetLang,
          disabled: q.disabled,
          enabled: q.enabled,
          rules: q.rules,
          spellIgnore: q.words,
          subtitles: q.subtitles,
          termIndex: termIndex ?? "",
          dictionary: dictionary ?? "",
        }),
      ),
    ),
  ) as CoreIssue[];
  return issues.filter((i) => i.segment === id);
}
