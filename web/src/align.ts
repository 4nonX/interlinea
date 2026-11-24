// The review of an alignment: the segments of two documents side by side,
// with a line from a segment to each segment it is paired with. Segments
// joined by lines, directly or through others, are one unit of the memory;
// a segment without a line is left out. The same model is in the desktop
// app (alignmentdialog.cpp).

import type { Alignment } from "./core";
import type { CoreRun } from "./editor/model";

/**
 * A line from a segment of the document to one of its translation. A line
 * someone drew or confirmed is theirs: it is drawn solid, and stays when
 * the rest is paired again.
 */
export interface Link {
  source: number;
  target: number;
  mine?: boolean;
}

/** Segments joined by lines: what is added to the memory as one unit. */
export interface Unit {
  source: number[];
  target: number[];
}

/** A pair needs a look below this score. */
export const LOOK_BELOW = 50;

export const linkKey = (l: Link) => `${l.source}|${l.target}`;

const unitKey = (u: Unit) => `${u.source.join(",")}|${u.target.join(",")}`;

/** The lines of the pairs as the core made them. */
export function linksOf(a: Alignment): Link[] {
  const out = new Map<string, Link>();
  const add = (l: Link) => out.set(linkKey(l), l);
  for (const p of a.pairs) {
    const first = p.targetSegments[0];
    const last = p.sourceSegments[p.sourceSegments.length - 1];
    if (first === undefined || last === undefined) continue;
    // Every segment of a pair hangs together: each sentence on the first
    // translation, each translation on the last sentence.
    for (const s of p.sourceSegments) add({ source: s, target: first, mine: p.fixed });
    for (const t of p.targetSegments) add({ source: last, target: t, mine: p.fixed });
  }
  return [...out.values()];
}

/** The scores of the pairs as the core made them, by their segments. */
export function scoresOf(a: Alignment): Map<string, number> {
  const out = new Map<string, number>();
  for (const p of a.pairs) out.set(unitKey({ source: p.sourceSegments, target: p.targetSegments }), p.score);
  return out;
}

/** Adds the line, or takes it away if it is there. A line added is its maker's, and so is what it joins. */
export function toggle(links: Link[], l: Link): Link[] {
  const k = linkKey(l);
  return links.some((x) => linkKey(x) === k) ? links.filter((x) => linkKey(x) !== k) : draw(links, l);
}

/** Adds the line as its maker's own, with the lines of the unit it joins. */
export function draw(links: Link[], l: Link): Link[] {
  const k = linkKey(l);
  return confirm(links.some((x) => linkKey(x) === k) ? links : [...links, { source: l.source, target: l.target }], "source", l.source);
}

/** Makes the lines of a segment's unit someone's own: checked, and kept when the rest is paired again. */
export function confirm(links: Link[], side: "source" | "target", i: number): Link[] {
  const unit = unitsOf(links).find((u) => u[side].includes(i));
  if (!unit) return links;
  return links.map((l) => (unit.source.includes(l.source) ? { ...l, mine: true } : l));
}

/** Whether a segment has a line that is not yet someone's own. */
export function unchecked(links: Link[], side: "source" | "target", i: number): boolean {
  return links.some((l) => l[side] === i && !l.mine);
}

/** The units whose lines are all someone's own: what stays when the rest is paired again. */
export function fixedOf(links: Link[]): Unit[] {
  return unitsOf(links).filter((u) => links.every((l) => !u.source.includes(l.source) || l.mine));
}

/** The units the lines make, in the order of the document. */
export function unitsOf(links: Link[]): Unit[] {
  // Segments joined through lines are found by following them: s0, s1…
  // for the document, t0, t1… for the translation.
  const parent = new Map<string, string>();
  const find = (x: string): string => {
    let r = x;
    while ((parent.get(r) ?? r) !== r) r = parent.get(r) ?? r;
    parent.set(x, r);
    return r;
  };
  for (const l of links) {
    const a = find(`s${l.source}`);
    const b = find(`t${l.target}`);
    if (a !== b) parent.set(a, b);
  }
  const groups = new Map<string, { source: Set<number>; target: Set<number> }>();
  for (const l of links) {
    const root = find(`s${l.source}`);
    const g = groups.get(root) ?? { source: new Set<number>(), target: new Set<number>() };
    g.source.add(l.source);
    g.target.add(l.target);
    groups.set(root, g);
  }
  const asc = (a: number, b: number) => a - b;
  return [...groups.values()]
    .map((g) => ({ source: [...g.source].sort(asc), target: [...g.target].sort(asc) }))
    .sort((a, b) => (a.source[0] ?? 0) - (b.source[0] ?? 0));
}

/** The score the core gave the unit; none for one made by hand. */
export function scoreOf(u: Unit, scores: Map<string, number>): number | undefined {
  return scores.get(unitKey(u));
}

export function needsLook(u: Unit, scores: Map<string, number>): boolean {
  const s = scoreOf(u, scores);
  return s !== undefined && s < LOOK_BELOW;
}

const SPACELESS = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Thai}\p{Script=Lao}\p{Script=Khmer}\p{Script=Myanmar}\s]/u;

const plainOf = (runs: CoreRun[]) => runs.map((r) => r.text ?? "").join("");

/** Segments as one text, with a space between them where the language writes one. */
export function joined(segments: CoreRun[][], which: number[]): CoreRun[] {
  const out: CoreRun[] = [];
  for (const i of which) {
    const next = segments[i] ?? [];
    const before = plainOf(out);
    const after = plainOf(next);
    if (before && after && !SPACELESS.test([...before].pop() ?? "") && !SPACELESS.test([...after][0] ?? "")) out.push({ text: " " });
    out.push(...next);
  }
  return out;
}
