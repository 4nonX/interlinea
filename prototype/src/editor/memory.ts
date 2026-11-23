// Translation memories for the editor, kept in the core (WebAssembly) so
// matching, scores and concordance are the core's. Three kinds per project:
// the client's memory, a project memory TMX files are imported into, and
// the document itself (its confirmed segments, as Trados and memoQ store
// them on confirm).

import { tm, type MemUnit } from "../core";
import { keys, load, save } from "../persist";
import type { Project } from "../data";
import {
  coreToRuns,
  plain,
  runsToCore,
  sourceKey,
  TM_UNITS,
  type Run,
  type Segment,
  type TmUnit,
} from "./model";

type Core = NonNullable<ReturnType<typeof import("../core").core>>;

export interface MemoryInfo {
  name: string;
  label: string;
  kind: "client" | "project" | "document";
}

const opened = new Set<string>();
const docVersion = new Map<string, string>();

// Project memories are saved as TMX between visits (the client memory is
// sample data, the document memory is rebuilt from the segments).
const SAVED = "memory:";
const saved = new Map<string, Uint8Array>();

/** Loads the saved project memories; call once before the editor opens. */
export async function preloadMemories(): Promise<void> {
  for (const k of await keys(SAVED)) {
    const bytes = await load<Uint8Array>(k);
    if (bytes instanceof Uint8Array) saved.set(k.slice(SAVED.length), bytes);
  }
}

/** Forgets the saved memories (the prototype data was reset); they are opened empty again. */
export function forgetMemories() {
  saved.clear();
  opened.clear();
  docVersion.clear();
}

/** Saves a memory after it changed, so the next visit has it. */
export async function persistMemory(c: Core, name: string): Promise<boolean> {
  const bytes = tm.exportTMX(c, name);
  saved.set(name, bytes);
  return save(SAVED + name, bytes);
}

function toMem(u: TmUnit): MemUnit {
  const attributes: Record<string, string> = { project: u.project };
  // memoQ's TMX property names, so exported context survives a round trip.
  if (u.prev !== undefined) attributes["x-context-pre"] = u.prev;
  if (u.next !== undefined) attributes["x-context-post"] = u.next;
  if (u.docSegment !== undefined) attributes.docSegment = String(u.docSegment);
  return {
    source: runsToCore(u.source),
    target: runsToCore(u.target),
    createdBy: u.by,
    createdAt: u.at,
    attributes,
  };
}

export function fromMem(
  m: MemUnit,
  memory: MemoryInfo,
): TmUnit & { memory: MemoryInfo } {
  const a = m.attributes ?? {};
  return {
    source: coreToRuns(m.source),
    target: coreToRuns(m.target),
    project: a.project ?? memory.label,
    by: m.createdBy ?? "",
    at: m.createdAt ?? "",
    prev: a["x-context-pre"] ?? a.prev,
    next: a["x-context-post"] ?? a.next,
    docSegment: a.docSegment ? Number(a.docSegment) : undefined,
    memory,
  };
}

/** The client's memory for a language pair, if the prototype has one. */
export function clientMemories(
  c: Core,
  p: Pick<Project, "client" | "sourceLang" | "targetLangs">,
): MemoryInfo[] {
  const src = p.sourceLang;
  const tgt = p.targetLangs[0] ?? "en-GB";
  const out: MemoryInfo[] = [];
  if (
    p.client === "Kanzlei Weber" &&
    src.startsWith("de") &&
    tgt.startsWith("en")
  ) {
    const name = "client:Kanzlei Weber";
    if (!opened.has(name)) {
      tm.open(c, name, "de-DE", "en-GB");
      tm.add(c, name, TM_UNITS.map(toMem));
      opened.add(name);
    }
    out.push({ name, label: "Kanzlei Weber (DE-EN)", kind: "client" });
  }
  return out;
}

/** The memories a project uses, opened in the core. */
export function projectMemories(c: Core, p: Project): MemoryInfo[] {
  const src = p.sourceLang;
  const tgt = p.targetLangs[0] ?? "en-GB";
  const out = clientMemories(c, p);
  const proj = `project:${p.id}`;
  if (!opened.has(proj)) {
    tm.open(c, proj, src, tgt);
    const bytes = saved.get(proj);
    if (bytes) tm.importTMX(c, proj, bytes);
    opened.add(proj);
  }
  out.push({
    name: proj,
    label: `${p.name} (project memory)`,
    kind: "project",
  });
  return out;
}

/** The document's confirmed segments as a memory, rebuilt when they change. */
export function documentMemory(
  c: Core,
  p: Project,
  segments: Segment[],
  by: string,
): MemoryInfo {
  const name = `document:${p.id}`;
  const units: MemUnit[] = [];
  segments.forEach((s, i) => {
    if (s.status !== "confirmed" || !plain(s.target).trim()) return;
    units.push(
      toMem({
        source: s.source,
        target: s.target,
        project: p.name,
        by,
        at: p.createdAt,
        docSegment: i + 1,
        prev: i > 0 ? plain(segments[i - 1]!.source) : "",
        next: i + 1 < segments.length ? plain(segments[i + 1]!.source) : "",
      }),
    );
  });
  const version = units
    .map((u) => JSON.stringify([u.source, u.target, u.attributes?.docSegment]))
    .join("|");
  if (docVersion.get(name) !== version) {
    tm.open(c, name, p.sourceLang, p.targetLangs[0] ?? "en-GB");
    if (units.length) tm.add(c, name, units);
    docVersion.set(name, version);
  }
  return { name, label: "This document", kind: "document" };
}

export type MemHit = ReturnType<typeof fromMem> & { score: number };

/** Looks a segment up in several memories and returns the best hits first. */
export function lookupAll(
  c: Core,
  memories: MemoryInfo[],
  source: Run[],
  min: number,
  limit: number,
): MemHit[] {
  const out: MemHit[] = [];
  const seen = new Set<string>();
  for (const m of memories) {
    for (const u of tm.lookup(c, m.name, runsToCore(source), min, limit)) {
      const unit = fromMem(u, m);
      const key =
        sourceKey(unit.source) +
        "|" +
        sourceKey(unit.target) +
        "|" +
        (unit.docSegment ?? "");
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({ ...unit, score: u.score ?? 0 });
    }
  }
  return out.sort((a, b) => b.score - a.score);
}

/** Concordance in source and target of several memories. */
export function concordanceAll(
  c: Core,
  memories: MemoryInfo[],
  query: string,
  limit: number,
) {
  const out: { unit: ReturnType<typeof fromMem>; side: "source" | "target" }[] =
    [];
  for (const m of memories) {
    for (const side of ["source", "target"] as const) {
      for (const u of tm.concordance(
        c,
        m.name,
        query,
        side === "target",
        limit,
      )) {
        out.push({ unit: fromMem(u, m), side });
      }
    }
  }
  return out.slice(0, limit);
}
