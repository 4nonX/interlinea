// Git-like terminology: a project termbase is a branch of one or more
// masters. This module computes the changes a project would merge into a
// master and applies the reviewer's decisions.

import type { Commit, Entry, LangCode, Removal, ReviewRules, Term } from "./data";

/** "removed" is a master entry the project leaves out: merging deletes it in the master. */
export type ChangeKind = "new" | "changed" | "conflict" | "removed";

/** The rule of the master by which a change needs no review. */
export type Rule = "identical" | "minor-fields" | "trusted";

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
  /** The project's entry; of an entry left out, the master's own (decisions go by its ID). */
  project: Entry;
  /** The master entry the project entry started from (changed/conflict). */
  master?: Entry;
  /** What the project changed relative to its starting point. */
  terms: TermDiff[];
  fields: FieldDiff[];
  /** For conflicts: what changed on the master meanwhile. */
  masterHistory: Commit[];
  /** Who made the change in the project. */
  author: string;
  /** Of an entry left out: when the project left it out. */
  leftOutAt?: string;
  /** A new entry the master already has as it is: merging links the two. */
  identicalTo?: Entry;
  /** The master's rule by which the change needs no review. */
  rule?: Rule;
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

/** Whether two entries hold the same: terms with status and fields, and the fields of the entry and its languages. */
export function sameContent(a: Entry, b: Entry): boolean {
  const sorted = (o: Record<string, unknown> | undefined) =>
    Object.entries(o ?? {})
      .filter(([, v]) => v !== "" && v !== undefined)
      .sort(([x], [y]) => (x < y ? -1 : x > y ? 1 : 0));
  const shape = (e: Entry) =>
    JSON.stringify([
      sorted(e.concept),
      sorted(e.terms)
        .filter(([, ts]) => (ts as Term[]).length > 0)
        .map(([l, ts]) => [
          l,
          (ts as Term[]).map((t) => [t.text, t.status, sorted(t.fields)]),
        ]),
      sorted(e.langFields)
        .map(([l, f]) => [l, sorted(f as Record<string, string>)])
        .filter(([, f]) => (f as unknown[]).length > 0),
    ]);
  return shape(a) === shape(b);
}

/** The rule of the master a change needs no review by, as the core decides it (termdb's applyRules). */
function ruleOf(ch: Change, rules: ReviewRules): Rule | undefined {
  if (ch.kind === "conflict" || ch.kind === "removed") return undefined;
  if (!rules.keepIdentical && ch.identicalTo) return "identical";
  const minor = rules.minorFields ?? [];
  if (
    ch.kind === "changed" &&
    minor.length > 0 &&
    ch.terms.length === 0 &&
    ch.fields.length > 0 &&
    ch.fields.every((f) => minor.includes(f.field))
  )
    return "minor-fields";
  const who = ch.author.toLowerCase();
  for (const t of rules.trusted ?? []) {
    if (t.author.toLowerCase() !== who) continue;
    if (!t.lang || (ch.project.terms[t.lang]?.length ?? 0) > 0) return "trusted";
  }
  return undefined;
}

/**
 * Changes of a project termbase relative to one master. `baseSnapshots`
 * holds each master entry as it was when the project copied it, so the
 * project's own edits can be told apart from later master edits.
 * `removals` are the master's entries the project leaves out.
 */
export function computeChanges(
  projectEntries: Entry[],
  master: { id: string; languages: LangCode[]; rules?: ReviewRules },
  masterEntries: Entry[],
  baseSnapshots: Record<string, Entry>,
  branchedAt: string,
  removals: Removal[] = [],
): Change[] {
  const out: Change[] = [];
  const author = (e: Entry) => e.history[e.history.length - 1]?.by ?? "";
  const fresh = (p: Entry) => {
    const { terms, fields } = diffEntries(undefined, p, master.languages);
    out.push({
      kind: "new",
      project: p,
      terms,
      fields,
      masterHistory: [],
      author: author(p),
      identicalTo: masterEntries.find((m) => sameContent(m, p)),
    });
  };
  for (const p of projectEntries) {
    if (!p.base) {
      fresh(p);
      continue;
    }
    if (p.base.termbaseId !== master.id) continue;
    const m = masterEntries.find((e) => e.id === p.base!.entryId);
    if (!m) {
      fresh(p);
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
      author: author(p),
    });
  }
  for (const x of removals) {
    if (x.masterId !== master.id) continue;
    const m = masterEntries.find((e) => e.id === x.entryId);
    if (!m) continue;
    out.push({
      kind: "removed",
      project: m,
      master: m,
      terms: [],
      fields: [],
      // What the master changed in the entry after the project left it out.
      masterHistory: m.history.filter((c) => c.at > x.at),
      author: x.by,
      leftOutAt: x.at,
    });
  }
  const rules = master.rules ?? {};
  for (const ch of out) ch.rule = ruleOf(ch, rules);
  const order: Record<ChangeKind, number> = {
    conflict: 0,
    removed: 1,
    changed: 2,
    new: 3,
  };
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
  /** The master's entries the merge deleted, by ID. */
  removed: string[];
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
  const removed: string[] = [];
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
    if (ch.kind === "removed") {
      const at = master.findIndex((e) => e.id === ch.master!.id);
      if (at >= 0) {
        master.splice(at, 1);
        removed.push(ch.master!.id);
        merged++;
      }
      continue;
    }
    const pi = project.findIndex((e) => e.id === ch.project.id);
    if (ch.kind === "new" && ch.identicalTo) {
      // The master has the entry as it is: the project's is linked to it.
      const m = master.find((e) => e.id === ch.identicalTo!.id);
      if (m) {
        project[pi] = {
          ...project[pi]!,
          base: { termbaseId: masterId, entryId: m.id, version: m.version },
        };
        merged++;
        continue;
      }
    }
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
  return { masterEntries: master, projectEntries: project, merged, removed };
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
