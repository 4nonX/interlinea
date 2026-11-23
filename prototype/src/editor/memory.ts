// Translation memories, kept in the core (WebAssembly) so matching, scores,
// penalties and concordance are the core's. Each memory the user has is
// opened in the core by its name the first time it is needed, from its
// saved TMX (or the sample data for the sample memory). Confirmed segments
// are stored in the memories a project updates, as Trados and memoQ do.

import { tm, type CoreApi, type MemUnit } from "../core";
import { keys, load, remove, save } from "../persist";
import type { LangCode, Project, TranslationMemory } from "../data";
import type { MemoryRef } from "../project";
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

const opened = new Set<string>();

// Memories are saved as TMX between visits, one key per memory name.
const SAVED = "memory:";
const saved = new Map<string, Uint8Array>();

/** Loads the saved memories; call once before the first memory opens. */
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
}

/** Moves a saved memory to a new name (saved work from an older version). */
export async function renameSavedMemory(from: string, to: string) {
  const bytes = await load<Uint8Array>(SAVED + from);
  if (!(bytes instanceof Uint8Array)) return;
  await save(SAVED + to, bytes);
  await remove(SAVED + from);
}

// Memories of team-server projects: loaded from the server's TMX, kept by
// the server, never saved in the browser.
const remote = new Set<string>();

/**
 * Opens a team-server memory in the core from the server's TMX, replacing
 * what was opened under its name before.
 */
export function openRemoteMemory(c: CoreApi, m: TranslationMemory, tmx: Uint8Array | null): void {
  remote.add(m.name);
  tm.open(c, m.name, m.sourceLang, m.targetLang);
  if (tmx) tm.importTMX(c, m.name, tmx);
  opened.add(m.name);
}

/** Saves a memory after it changed, so the next visit has it. */
export async function persistMemory(
  c: CoreApi,
  name: string,
): Promise<boolean> {
  // The server stores what is confirmed in its projects' memories.
  if (remote.has(name)) return true;
  const bytes = tm.exportTMX(c, name);
  saved.set(name, bytes);
  return save(SAVED + name, bytes);
}

/** Deletes a memory's saved units. */
export async function dropMemory(name: string): Promise<void> {
  saved.delete(name);
  opened.delete(name);
  await remove(SAVED + name);
}

export function toMem(u: TmUnit): MemUnit {
  const attributes: Record<string, string> = { project: u.project };
  // memoQ's TMX property names, so exported context survives a round trip.
  if (u.prev !== undefined) attributes["x-context-pre"] = u.prev;
  if (u.next !== undefined) attributes["x-context-post"] = u.next;
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
  memory: MemoryRef,
): TmUnit & { memory: MemoryRef } {
  const a = m.attributes ?? {};
  return {
    source: coreToRuns(m.source),
    target: coreToRuns(m.target),
    project: a.project ?? memory.name,
    by: m.createdBy ?? "",
    at: m.createdAt ?? "",
    prev: a["x-context-pre"] ?? a.prev,
    next: a["x-context-post"] ?? a.next,
    memory,
  };
}

/** A confirmed segment as a memory unit, with its neighbours for context matches. */
export function unitOf(
  segments: Segment[],
  index: number,
  project: string,
  by: string,
  at: string,
): MemUnit {
  const s = segments[index]!;
  return toMem({
    source: s.source,
    target: s.target,
    project,
    by,
    at,
    prev: index > 0 ? plain(segments[index - 1]!.source) : "",
    next: index + 1 < segments.length ? plain(segments[index + 1]!.source) : "",
  });
}

/**
 * Opens a memory in the core once per page: from its saved units, or the
 * sample data. A project memory seeded from its project also gets the
 * project's confirmed segments in its language: the sample project's, and
 * those of projects saved before projects had memories, whose confirmed
 * segments were never stored in one. An identical pair updates the unit
 * already there, so seeding again adds nothing twice.
 */
export function openMemory(
  c: CoreApi,
  m: TranslationMemory,
  projects: Project[],
): void {
  if (opened.has(m.name)) return;
  // A team-server memory not loaded yet: open empty until it is.
  if (m.remote) {
    openRemoteMemory(c, m, null);
    return;
  }
  tm.open(c, m.name, m.sourceLang, m.targetLang);
  const bytes = saved.get(m.name);
  if (bytes) tm.importTMX(c, m.name, bytes);
  else if (m.sample) tm.add(c, m.name, TM_UNITS.map(toMem));
  if (m.seedFromProject) {
    const p = projects.find((x) => x.id === m.projectId);
    const units = p ? confirmedUnits(p, m.targetLang) : [];
    const before = tm.count(c, m.name);
    if (units.length) tm.add(c, m.name, units);
    if (tm.count(c, m.name) !== before) void persistMemory(c, m.name);
  }
  opened.add(m.name);
}

/**
 * Opens a new, empty memory in the core, filled from a TMX file if one is
 * given; returns the number of units stored. It is saved once the project
 * or memory it is made for is created.
 */
export function openNewMemory(
  c: CoreApi,
  m: TranslationMemory,
  tmx?: Uint8Array,
): number {
  tm.open(c, m.name, m.sourceLang, m.targetLang);
  opened.add(m.name);
  return tmx ? tm.importTMX(c, m.name, tmx).stored : 0;
}

function confirmedUnits(p: Project, lang: LangCode): MemUnit[] {
  const out: MemUnit[] = [];
  for (const f of p.files) {
    const segs = f.segments[lang];
    if (!segs) continue;
    segs.forEach((s, i) => {
      if (s.status === "confirmed" && plain(s.target).trim())
        out.push(unitOf(segs, i, p.name, "", p.createdAt));
    });
  }
  return out;
}

/** Opens every memory the references name; ones that no longer exist are left out. */
export function openRefs(
  c: CoreApi,
  refs: MemoryRef[],
  all: TranslationMemory[],
  projects: Project[],
): MemoryRef[] {
  const out: MemoryRef[] = [];
  for (const r of refs) {
    const m = all.find((x) => x.name === r.name);
    if (!m) continue;
    openMemory(c, m, projects);
    out.push(r);
  }
  return out;
}

export type MemHit = ReturnType<typeof fromMem> & { score: number };

/** Looks a segment up in several memories, penalties applied, best first. */
export function lookupAll(
  c: CoreApi,
  memories: MemoryRef[],
  source: Run[],
  min: number,
  limit: number,
): MemHit[] {
  const out: MemHit[] = [];
  const seen = new Set<string>();
  for (const m of memories) {
    for (const u of tm.lookup(
      c,
      m.name,
      runsToCore(source),
      min,
      limit,
      m.penalty,
    )) {
      const unit = fromMem(u, m);
      const key = sourceKey(unit.source) + "|" + sourceKey(unit.target);
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({ ...unit, score: u.score ?? 0 });
    }
  }
  return out.sort((a, b) => b.score - a.score);
}

/** Concordance in source and target of several memories. */
export function concordanceAll(
  c: CoreApi,
  memories: MemoryRef[],
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

/**
 * Stores a confirmed segment in the memories a project updates and saves
 * them. Returns false when the browser would not save.
 */
export async function storeConfirmed(
  c: CoreApi,
  memories: MemoryRef[],
  unit: MemUnit,
): Promise<boolean> {
  let kept = true;
  for (const m of memories) {
    tm.add(c, m.name, [unit]);
    kept = (await persistMemory(c, m.name)) && kept;
  }
  return kept;
}
