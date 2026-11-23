// Match diffs for the editor. Matching, term recognition and the quality
// checks run in the Go core; the diff tokenizer mirrors the core's matcher:
// weighted token edit distance (words 1, punctuation and tags 0.5, case-only
// differences 0.2).

import type { Entry, Term, Termbase } from "../data";
import { isTag, tagKey, type Run, type TmUnit } from "./model";

interface Token {
  kind: "word" | "punct" | "tag";
  text: string;
  fold: string;
}

export function tokenize(runs: Run[]): Token[] {
  const out: Token[] = [];
  for (const r of runs) {
    if (isTag(r)) {
      out.push({ kind: "tag", text: tagKey(r), fold: tagKey(r) });
      continue;
    }
    for (const m of r.matchAll(
      /[\p{L}\p{M}\p{N}]+(?:[.,'’][\p{L}\p{N}]+)*|[^\s\p{L}\p{N}]/gu,
    )) {
      const t = m[0];
      const word = /[\p{L}\p{N}]/u.test(t);
      out.push({
        kind: word ? "word" : "punct",
        text: t,
        fold: t.toLocaleLowerCase(),
      });
    }
  }
  return out;
}

const weight = (t: Token) => (t.kind === "word" ? 1 : 0.5);

function subCost(a: Token, b: Token): number {
  if (a.kind === b.kind && a.text === b.text) return 0;
  const w = Math.max(weight(a), weight(b));
  if (a.kind === b.kind && a.fold === b.fold) return 0.2 * w;
  return w;
}

/** Edit operations aligning a to b; used to highlight what differs. */
export interface Alignment {
  distance: number;
  /** For each token of b: true when it differs from a. */
  changedInB: boolean[];
}

export function align(a: Token[], b: Token[]): Alignment {
  const n = a.length;
  const m = b.length;
  const d: number[][] = Array.from({ length: n + 1 }, () =>
    new Array<number>(m + 1).fill(0),
  );
  for (let i = 1; i <= n; i++) d[i]![0] = d[i - 1]![0]! + weight(a[i - 1]!);
  for (let j = 1; j <= m; j++) d[0]![j] = d[0]![j - 1]! + weight(b[j - 1]!);
  for (let i = 1; i <= n; i++) {
    for (let j = 1; j <= m; j++) {
      d[i]![j] = Math.min(
        d[i - 1]![j]! + weight(a[i - 1]!),
        d[i]![j - 1]! + weight(b[j - 1]!),
        d[i - 1]![j - 1]! + subCost(a[i - 1]!, b[j - 1]!),
      );
    }
  }
  const changedInB = new Array<boolean>(m).fill(false);
  let i = n;
  let j = m;
  while (i > 0 || j > 0) {
    if (
      i > 0 &&
      j > 0 &&
      d[i]![j] === d[i - 1]![j - 1]! + subCost(a[i - 1]!, b[j - 1]!)
    ) {
      changedInB[j - 1] = subCost(a[i - 1]!, b[j - 1]!) > 0;
      i--;
      j--;
    } else if (j > 0 && d[i]![j] === d[i]![j - 1]! + weight(b[j - 1]!)) {
      changedInB[j - 1] = true;
      j--;
    } else {
      i--;
    }
  }
  return { distance: d[n]![m]!, changedInB };
}

export interface TmMatch {
  unit: TmUnit;
  score: number;
  /** TM source tokens with the ones that differ from the current segment marked. */
  diff: { text: string; changed: boolean; space: boolean }[];
}

/** Tokens of a memory source with the ones that differ from the segment marked. */
export function diffAgainst(
  source: Run[],
  memorySource: Run[],
): TmMatch["diff"] {
  const cur = tokenize(source);
  const tmTokens = tokenize(memorySource);
  const { changedInB } = align(cur, tmTokens);
  return tmTokens
    .filter((t) => t.kind !== "tag")
    .map((t, i) => ({
      text: t.text,
      changed: changedInB[tmTokens.indexOf(t)] ?? false,
      space: i > 0 && t.kind === "word",
    }));
}

export interface TermHit {
  entry: Entry;
  termbase: Termbase;
  /** The source term that matched. */
  term: Term;
  start: number;
  end: number;
  targets: Term[];
}

/** A problem the checks found in a translation. */
export interface QaIssue {
  kind: string;
  text: string;
}
