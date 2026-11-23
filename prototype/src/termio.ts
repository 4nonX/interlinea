// Termbase exchange through the core: TBX, MultiTerm XML, CSV, TSV and
// Excel are read and written by the Go term package (compiled to
// WebAssembly). This module converts between the prototype's entries,
// whose fields are keyed by field ID, and the core's entries, whose fields
// are keyed by field name, as in the exchange formats.

import { unwrap, type CoreApi } from "./core";
import {
  LANGUAGES,
  primary,
  type Entry,
  type InfoType,
  type LangCode,
  type Term,
  type Termbase,
  type TermStatus,
} from "./data";

export type TermFormat =
  "tbx" | "tbx2" | "multiterm" | "xdt" | "csv" | "tsv" | "xlsx";
export type CoreLevel = "entry" | "language" | "term";

export interface CoreValue {
  field: string;
  value: string;
}
export interface CoreTerm {
  text: string;
  status?: string;
  fields?: CoreValue[];
}
export interface CoreLanguage {
  lang: string;
  fields?: CoreValue[];
  terms: CoreTerm[];
}
export interface CoreEntry {
  id?: string;
  fields?: CoreValue[];
  languages: CoreLanguage[];
  createdAt?: string;
  createdBy?: string;
  changedAt?: string;
  changedBy?: string;
}
export interface CoreField {
  name: string;
  level: CoreLevel;
  kind: string;
  values?: string[];
}
export interface CoreTermbase {
  name?: string;
  languages: string[] | null;
  fields?: CoreField[] | null;
  entries: CoreEntry[] | null;
}
export interface ReadReport {
  entries: number;
  terms: number;
  skipped: number;
  warnings?: string[];
}

export type ColumnRole = "term" | "status" | "field" | "id" | "ignore";
export interface Column {
  index: number;
  header?: string;
  role: ColumnRole;
  lang?: string;
  field?: string;
  level?: CoreLevel;
}
export interface Mapping {
  columns: Column[];
  header: boolean;
  separator?: string;
}
export interface Detected {
  format: TermFormat;
  tabular: boolean;
  rows?: string[][];
  total?: number;
  mapping?: Mapping;
}

export interface Policy {
  match: "none" | "term" | "id";
  lang?: string;
  onMatch: "merge" | "overwrite" | "skip" | "add";
}
export interface PlanAction {
  kind: "add" | "merge" | "overwrite" | "skip";
  existing?: string;
  entry?: CoreEntry;
  incoming: number;
  ambiguous?: string[];
}
export interface PlanReport {
  added: number;
  merged: number;
  overwritten: number;
  skipped: number;
  unchanged: number;
  warnings?: string[];
}

interface TermCore {
  termDetect(
    bytes: Uint8Array,
    name: string,
  ): { ok: true; value: string } | { ok: false; error: string };
  termImport(
    bytes: Uint8Array,
    format: string,
    mapping: string,
  ): { ok: true; value: string } | { ok: false; error: string };
  termExport(
    termbase: string,
    format: string,
  ): { ok: true; value: Uint8Array } | { ok: false; error: string };
  termPlan(
    existing: string,
    incoming: string,
    policy: string,
  ): { ok: true; value: string } | { ok: false; error: string };
  termMerge(
    a: string,
    b: string,
  ): { ok: true; value: string } | { ok: false; error: string };
  termDuplicates(
    entries: string,
    lang: string,
  ): { ok: true; value: string } | { ok: false; error: string };
}

function api(c: CoreApi): TermCore {
  return c as unknown as TermCore;
}

export const FORMATS: {
  id: TermFormat;
  label: string;
  ext: string;
  type: string;
  note: string;
}[] = [
  {
    id: "tbx",
    label: "TBX 3",
    ext: ".tbx",
    type: "application/x-tbx+xml",
    note: "The ISO standard (30042:2019). Phrase, memoQ and current Trados read it.",
  },
  {
    id: "tbx2",
    label: "TBX 2",
    ext: ".tbx",
    type: "application/x-tbx+xml",
    note: "The older <martif> form, for tools that do not read TBX 3 yet.",
  },
  {
    id: "multiterm",
    label: "MultiTerm XML",
    ext: ".xml",
    type: "application/xml",
    note: "For importing into Trados MultiTerm.",
  },
  {
    id: "xlsx",
    label: "Excel workbook",
    ext: ".xlsx",
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    note: "One row per entry, one column per language and field. Synonyms share a cell, separated by “; ”.",
  },
  {
    id: "csv",
    label: "CSV",
    ext: ".csv",
    type: "text/csv",
    note: "Comma-separated, UTF-8. Opens in Excel and imports into memoQ.",
  },
  {
    id: "tsv",
    label: "Tab-separated text",
    ext: ".tsv",
    type: "text/tab-separated-values",
    note: "UTF-8, one row per entry.",
  },
];

export const FORMAT_LABEL: Record<TermFormat, string> = {
  tbx: "TBX 3",
  tbx2: "TBX 2",
  multiterm: "MultiTerm XML",
  xdt: "MultiTerm termbase definition",
  csv: "CSV",
  tsv: "Tab-separated text",
  xlsx: "Excel workbook",
};

export const IMPORT_ACCEPT =
  ".tbx,.tbxm,.xml,.mtf,.xdt,.csv,.tsv,.tab,.txt,.xlsx";

const LEVEL_TO_CORE: Record<InfoType["level"], CoreLevel> = {
  concept: "entry",
  language: "language",
  term: "term",
};
const LEVEL_FROM_CORE: Record<CoreLevel, InfoType["level"]> = {
  entry: "concept",
  language: "language",
  term: "term",
};

export const term = {
  detect(c: CoreApi, bytes: Uint8Array, name: string): Detected {
    return JSON.parse(unwrap(api(c).termDetect(bytes, name))) as Detected;
  },
  read(
    c: CoreApi,
    bytes: Uint8Array,
    format: TermFormat,
    mapping?: Mapping,
  ): { termbase: CoreTermbase; report: ReadReport } {
    return JSON.parse(
      unwrap(
        api(c).termImport(
          bytes,
          format,
          mapping ? JSON.stringify(mapping) : "",
        ),
      ),
    ) as { termbase: CoreTermbase; report: ReadReport };
  },
  write(c: CoreApi, tb: CoreTermbase, format: TermFormat): Uint8Array {
    return unwrap(api(c).termExport(JSON.stringify(tb), format));
  },
  plan(
    c: CoreApi,
    existing: CoreEntry[],
    incoming: CoreEntry[],
    policy: Policy,
  ): { actions: PlanAction[] | null; report: PlanReport } {
    return JSON.parse(
      unwrap(
        api(c).termPlan(
          JSON.stringify(existing),
          JSON.stringify(incoming),
          JSON.stringify(policy),
        ),
      ),
    ) as { actions: PlanAction[] | null; report: PlanReport };
  },
  merge(c: CoreApi, a: CoreEntry, b: CoreEntry): CoreEntry {
    return JSON.parse(
      unwrap(api(c).termMerge(JSON.stringify(a), JSON.stringify(b))),
    ) as CoreEntry;
  },
  duplicates(c: CoreApi, entries: CoreEntry[], langCode: string): number[][] {
    return JSON.parse(
      unwrap(api(c).termDuplicates(JSON.stringify(entries), langCode)),
    ) as number[][];
  },
};

function values(
  rec: Record<string, string> | undefined,
  names: Map<string, string>,
): CoreValue[] | undefined {
  if (!rec) return undefined;
  const out: CoreValue[] = [];
  for (const [id, v] of Object.entries(rec)) {
    if (v.trim()) out.push({ field: names.get(id) ?? id, value: v });
  }
  return out.length ? out : undefined;
}

/** A prototype entry as the core sees it. The ID is the file's own ID when the entry was imported. */
export function toCoreEntry(e: Entry, tb: Termbase): CoreEntry {
  const names = new Map(tb.infoTypes.map((i) => [i.id, i.name]));
  const codes = [
    ...new Set([
      ...tb.languages,
      ...Object.keys(e.terms),
      ...Object.keys(e.langFields ?? {}),
    ]),
  ];
  const languages: CoreLanguage[] = [];
  for (const code of codes) {
    const terms = (e.terms[code] ?? []).map((t): CoreTerm => ({
      text: t.text,
      status: t.status,
      fields: values(t.fields, names),
    }));
    const fields = values(e.langFields?.[code], names);
    if (terms.length || fields) languages.push({ lang: code, fields, terms });
  }
  const first = e.history[0];
  const last = e.history[e.history.length - 1];
  return {
    id: e.externalId ?? e.id,
    fields: values(e.concept, names),
    languages,
    createdAt: first?.at,
    createdBy: first?.by,
    changedAt: last?.at,
    changedBy: last?.by,
  };
}

/** A whole termbase as the core writes it. */
export function toCoreTermbase(tb: Termbase, entries: Entry[]): CoreTermbase {
  return {
    name: tb.name,
    languages: tb.languages,
    fields: tb.infoTypes.map((i) => ({
      name: i.name,
      level: LEVEL_TO_CORE[i.level],
      kind: i.kind,
      values: i.values,
    })),
    entries: entries.map((e) => toCoreEntry(e, tb)),
  };
}

/**
 * Fields of a file matched to a termbase's fields by name; fields the
 * termbase lacks are created with the level and kind the file gives.
 */
export function matchFields(
  tb: Pick<Termbase, "infoTypes">,
  file: CoreTermbase,
  incoming: CoreEntry[],
): { ids: Map<string, string>; created: InfoType[] } {
  const ids = new Map<string, string>();
  const created: InfoType[] = [];
  const byName = new Map(
    tb.infoTypes.map((i) => [i.name.toLocaleLowerCase(), i.id]),
  );
  const declared = new Map(
    (file.fields ?? []).map((f) => [f.name.toLocaleLowerCase(), f]),
  );
  const seen = new Map<string, CoreLevel>();
  for (const e of incoming) {
    for (const v of e.fields ?? [])
      seen.set(v.field, seen.get(v.field) ?? "entry");
    for (const l of e.languages) {
      for (const v of l.fields ?? [])
        seen.set(v.field, seen.get(v.field) ?? "language");
      for (const t of l.terms)
        for (const v of t.fields ?? [])
          seen.set(v.field, seen.get(v.field) ?? "term");
    }
  }
  for (const [name, level] of seen) {
    const key = name.toLocaleLowerCase();
    const known = byName.get(key);
    if (known) {
      ids.set(name, known);
      continue;
    }
    const d = declared.get(key);
    const id = `f-${slug(name)}-${created.length + 1}`;
    const info: InfoType = {
      id,
      name,
      level: LEVEL_FROM_CORE[d?.level ?? level],
      kind: d?.kind === "picklist" && d.values?.length ? "picklist" : "text",
      values: d?.kind === "picklist" ? d.values : undefined,
    };
    created.push(info);
    byName.set(key, id);
    ids.set(name, id);
  }
  return { ids, created };
}

function slug(s: string): string {
  return (
    s
      .toLocaleLowerCase()
      .replace(/[^\p{L}\p{N}]+/gu, "-")
      .replace(/^-|-$/g, "") || "field"
  );
}

function record(
  vs: CoreValue[] | undefined,
  ids: Map<string, string>,
): Record<string, string> | undefined {
  if (!vs?.length) return undefined;
  const out: Record<string, string> = {};
  for (const v of vs) {
    const id = ids.get(v.field) ?? v.field;
    out[id] = out[id] ? `${out[id]}; ${v.value}` : v.value;
  }
  return out;
}

function statusOf(s: string | undefined, fallback: TermStatus): TermStatus {
  return s === "approved" || s === "proposed" || s === "forbidden"
    ? s
    : fallback;
}

/** The content of a core entry in the prototype's shape. Languages must already be the termbase's codes. */
export function fromCoreEntry(
  e: CoreEntry,
  ids: Map<string, string>,
  fallback: TermStatus,
): Pick<Entry, "concept" | "terms" | "langFields"> {
  const terms: Record<LangCode, Term[]> = {};
  const langFields: Record<LangCode, Record<string, string>> = {};
  for (const l of e.languages) {
    const list = l.terms.map((t): Term => ({
      text: t.text,
      status: statusOf(t.status, fallback),
      fields: record(t.fields, ids),
    }));
    if (list.length) terms[l.lang] = [...(terms[l.lang] ?? []), ...list];
    const f = record(l.fields, ids);
    if (f) langFields[l.lang] = { ...langFields[l.lang], ...f };
  }
  return {
    concept: record(e.fields, ids) ?? {},
    terms,
    langFields: Object.keys(langFields).length ? langFields : undefined,
  };
}

/**
 * The termbase language a file language most likely means: the same code,
 * else a termbase language with the same primary language, else the first
 * known language with it (de becomes de-DE).
 */
export function suggestLanguage(
  fileLang: string,
  tbLangs: LangCode[],
): LangCode {
  const lower = fileLang.toLowerCase();
  const exact = tbLangs.find((l) => l.toLowerCase() === lower);
  if (exact) return exact;
  const p = primary(fileLang);
  const regional = fileLang.includes("-");
  if (!regional) {
    const inTb = tbLangs.find((l) => primary(l) === p);
    if (inTb) return inTb;
    const known = LANGUAGES.find((l) => primary(l.code) === p);
    if (known) return known.code;
    return fileLang;
  }
  const known = LANGUAGES.find((l) => l.code.toLowerCase() === lower);
  return known?.code ?? fileLang;
}

/** Applies a language mapping (file code → termbase code, or null to leave out) and drops entries left without terms. */
export function remapLanguages(
  entries: CoreEntry[],
  map: Record<string, LangCode | null>,
): { entries: CoreEntry[]; dropped: number } {
  const out: CoreEntry[] = [];
  let dropped = 0;
  for (const e of entries) {
    const byCode = new Map<string, CoreLanguage>();
    for (const l of e.languages) {
      const to = map[l.lang];
      if (to === null) continue;
      const code = to ?? l.lang;
      const prev = byCode.get(code);
      byCode.set(
        code,
        prev
          ? {
              lang: code,
              fields: [...(prev.fields ?? []), ...(l.fields ?? [])],
              terms: [...prev.terms, ...l.terms],
            }
          : { ...l, lang: code },
      );
    }
    const languages = [...byCode.values()];
    if (!languages.some((l) => l.terms.length)) {
      dropped++;
      continue;
    }
    out.push({ ...e, languages });
  }
  return { entries: out, dropped };
}

/** All languages used in a file, in the order the file declares them. */
export function fileLanguages(tb: CoreTermbase): string[] {
  const seen = new Set<string>(tb.languages ?? []);
  for (const e of tb.entries ?? [])
    for (const l of e.languages) seen.add(l.lang);
  return [...seen];
}

export function fileName(tb: Termbase, format: TermFormat): string {
  const f = FORMATS.find((x) => x.id === format)!;
  const base = tb.name
    .replace(/ \(project\)$/, "")
    .replace(/[^\p{L}\p{N}]+/gu, "_")
    .replace(/^_|_$/g, "");
  return `${base || "termbase"}${format === "tbx2" ? "_tbx2" : ""}${f.ext}`;
}

/* Term recognition */

export interface CoreHit {
  source: string;
  entryId: string;
  term: CoreTerm;
  start: number;
  end: number;
  exact: boolean;
  targets: CoreTerm[];
}

interface RecognitionCore {
  termIndex(
    name: string,
    sources: string,
  ): { ok: true; value: number } | { ok: false; error: string };
  termRecognize(
    name: string,
    text: string,
    src: string,
    tgt: string,
  ): { ok: true; value: string } | { ok: false; error: string };
}

function rc(c: CoreApi): RecognitionCore {
  return c as unknown as RecognitionCore;
}

/**
 * Builds the core's term index for a set of termbases, highest priority
 * first. Entries a later termbase holds in a newer version (a project
 * termbase's copy of a master entry) replace the older one, so each concept
 * is offered once.
 */
export function buildIndex(
  c: CoreApi,
  name: string,
  termbases: Termbase[],
  entries: Entry[],
): number {
  const shadowed = new Set(
    entries
      .filter((e) => e.base && termbases.some((t) => t.id === e.termbaseId))
      .map((e) => `${e.base!.termbaseId}/${e.base!.entryId}`),
  );
  const sources = termbases.map((tb) => ({
    source: tb.id,
    entries: entries
      .filter(
        (e) => e.termbaseId === tb.id && !shadowed.has(`${tb.id}/${e.id}`),
      )
      .map((e) => ({ ...toCoreEntry(e, tb), id: e.id })),
  }));
  return unwrap(rc(c).termIndex(name, JSON.stringify(sources)));
}

export function recognizeTerms(
  c: CoreApi,
  name: string,
  text: string,
  src: string,
  tgt: string,
): CoreHit[] {
  return JSON.parse(
    unwrap(rc(c).termRecognize(name, text, src, tgt)),
  ) as CoreHit[];
}
