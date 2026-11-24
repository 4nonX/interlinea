// Git-like terminology: a project termbase is a branch of one or more
// masters. This module computes the changes a project would merge into a
// master and applies the reviewer's decisions.

import type { Commit, Entry, LangCode, Term } from "./data";

export type ChangeKind = "new" | "changed" | "conflict";

export interface TermDiff {
  lang: LangCode;
  added: string[];
  removed: string[];
}

export interface FieldDiff {
  field: string;
  before?: string;
  after?: string;
}

export interface Change {
  kind: ChangeKind;
  project: Entry;
  /** The master entry the project entry started from (changed/conflict). */
  master?: Entry;
  /** What the project changed relative to its starting point. */
  terms: TermDiff[];
  fields: FieldDiff[];
  /** For conflicts: what changed on the master meanwhile. */
  masterHistory: Commit[];
}

/** Decision per change. "keep-both" only applies to conflicts. */
export type Decision = "accept" | "skip" | "keep-master" | "keep-both";

function termTexts(e: Entry | undefined, code: LangCode): string[] {
  return (e?.terms[code] ?? []).map((t) => t.text);
}

export function diffEntries(
  before: Entry | undefined,
  after: Entry,
  langs: LangCode[],
): { terms: TermDiff[]; fields: FieldDiff[] } {
  const terms: TermDiff[] = [];
  for (const code of langs) {
    const b = new Set(termTexts(before, code));
    const a = new Set(termTexts(after, code));
    const added = [...a].filter((x) => !b.has(x));
    const removed = [...b].filter((x) => !a.has(x));
    if (added.length || removed.length)
      terms.push({ lang: code, added, removed });
  }
  const fields: FieldDiff[] = [];
  const keys = new Set([
    ...Object.keys(before?.concept ?? {}),
    ...Object.keys(after.concept),
  ]);
  for (const k of keys) {
    const bv = before?.concept[k];
    const av = after.concept[k];
    if (bv !== av) fields.push({ field: k, before: bv, after: av });
  }
  return { terms, fields };
}

/**
 * Changes of a project termbase relative to one master. `baseSnapshots`
 * holds each master entry as it was when the project copied it, so the
 * project's own edits can be told apart from later master edits.
 */
export function computeChanges(
  projectEntries: Entry[],
  master: { id: string; languages: LangCode[] },
  masterEntries: Entry[],
  baseSnapshots: Record<string, Entry>,
  branchedAt: string,
): Change[] {
  const out: Change[] = [];
  for (const p of projectEntries) {
    if (!p.base) {
      const { terms, fields } = diffEntries(undefined, p, master.languages);
      out.push({ kind: "new", project: p, terms, fields, masterHistory: [] });
      continue;
    }
    if (p.base.termbaseId !== master.id) continue;
    const m = masterEntries.find((e) => e.id === p.base!.entryId);
    if (!m) {
      const { terms, fields } = diffEntries(undefined, p, master.languages);
      out.push({ kind: "new", project: p, terms, fields, masterHistory: [] });
      continue;
    }
    const start = baseSnapshots[p.id] ?? m;
    const { terms, fields } = diffEntries(start, p, master.languages);
    if (!terms.length && !fields.length) continue;
    const conflict = m.version !== p.base.version;
    out.push({
      kind: conflict ? "conflict" : "changed",
      project: p,
      master: m,
      terms,
      fields,
      masterHistory: conflict ? m.history.filter((c) => c.at > branchedAt) : [],
    });
  }
  const order: Record<ChangeKind, number> = { conflict: 0, changed: 1, new: 2 };
  return out.sort((a, b) => order[a.kind] - order[b.kind]);
}

function mergeTerms(a: Term[] | undefined, b: Term[] | undefined): Term[] {
  const out = [...(a ?? [])];
  for (const t of b ?? []) if (!out.some((x) => x.text === t.text)) out.push(t);
  return out;
}

export interface MergeResult {
  masterEntries: Entry[];
  projectEntries: Entry[];
  merged: number;
}

let seq = 0;
const newId = (p: string) =>
  `${p}-${Date.now().toString(36)}-${(seq++).toString(36)}`;

/** Applies decisions; returns the new master and project entries. */
export function applyMerge(
  changes: Change[],
  decisions: Record<string, Decision>,
  masterId: string,
  masterEntries: Entry[],
  projectEntries: Entry[],
  ctx: { by: string; projectName: string; at: string },
): MergeResult {
  const master = masterEntries.map((e) => ({ ...e }));
  const project = projectEntries.map((e) => ({ ...e }));
  let merged = 0;
  const commit = (summary: string): Commit => ({
    id: newId("c"),
    at: ctx.at,
    by: ctx.by,
    summary,
    project: ctx.projectName,
  });

  for (const ch of changes) {
    const decision =
      decisions[ch.project.id] ??
      (ch.kind === "conflict" ? undefined : "accept");
    if (!decision || decision === "skip") continue;
    const pi = project.findIndex((e) => e.id === ch.project.id);
    if (ch.kind === "new") {
      const id = newId("e");
      master.push({
        ...ch.project,
        id,
        termbaseId: masterId,
        version: 1,
        base: undefined,
        history: [
          ...ch.project.history,
          commit(`Merged from project “${ctx.projectName}”`),
        ],
      });
      project[pi] = {
        ...project[pi]!,
        base: { termbaseId: masterId, entryId: id, version: 1 },
      };
      merged++;
      continue;
    }
    const mi = master.findIndex((e) => e.id === ch.master!.id);
    const m = master[mi]!;
    if (decision === "keep-master") {
      // The project adopts the master's version; nothing changes on the master.
      project[pi] = {
        ...m,
        id: ch.project.id,
        termbaseId: ch.project.termbaseId,
        base: { termbaseId: masterId, entryId: m.id, version: m.version },
        history: [...ch.project.history, commit("Adopted master version")],
      };
      merged++;
      continue;
    }
    let terms = ch.project.terms;
    let concept = { ...m.concept, ...ch.project.concept };
    let summary = `Merged changes from project “${ctx.projectName}”`;
    if (decision === "keep-both") {
      terms = {};
      for (const code of new Set([
        ...Object.keys(m.terms),
        ...Object.keys(ch.project.terms),
      ])) {
        terms[code] = mergeTerms(m.terms[code], ch.project.terms[code]);
      }
      concept = { ...ch.project.concept, ...m.concept };
      summary = `Merged with project “${ctx.projectName}”, kept both versions`;
    }
    const updated: Entry = {
      ...m,
      terms,
      concept,
      version: m.version + 1,
      history: [
        ...m.history,
        ...ch.project.history.filter((c) => c.project === ctx.projectName),
        commit(summary),
      ],
    };
    master[mi] = updated;
    project[pi] = {
      ...project[pi]!,
      terms,
      concept,
      base: { termbaseId: masterId, entryId: m.id, version: updated.version },
    };
    merged++;
  }
  return { masterEntries: master, projectEntries: project, merged };
}

/** Master entries created or changed after the project branched. */
export function incomingFromMaster(
  masterEntries: Entry[],
  branchedAt: string,
): { created: Entry[]; changed: Entry[] } {
  const created: Entry[] = [];
  const changed: Entry[] = [];
  for (const m of masterEntries) {
    if (!m.history.some((c) => c.at > branchedAt)) continue;
    if (m.history[0]!.at > branchedAt) created.push(m);
    else changed.push(m);
  }
  return { created, changed };
}
