// Diffs and checks for the editor. Term recognition runs in the Go core
// (termio.ts); the diff tokenizer mirrors the core's matcher:
// weighted token edit distance (words 1, punctuation and tags 0.5, case-only
// differences 0.2).

import type { Entry, Term, Termbase } from "../data";
import { isTag, plain, tagKey, type Run, type TmUnit } from "./model";

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

export function score(a: Run[], b: Run[]): number {
  if (
    plain(a) === plain(b) &&
    a.filter(isTag).map(tagKey).join() === b.filter(isTag).map(tagKey).join()
  )
    return 100;
  const ta = tokenize(a);
  const tb = tokenize(b);
  const total = Math.max(
    ta.reduce((s, t) => s + weight(t), 0),
    tb.reduce((s, t) => s + weight(t), 0),
  );
  if (!total) return 99;
  const s = Math.floor((1 - align(ta, tb).distance / total) * 100);
  return Math.max(0, Math.min(99, s));
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

export function lookup(
  source: Run[],
  units: TmUnit[],
  min = 70,
  limit = 3,
): TmMatch[] {
  const cur = tokenize(source);
  return units
    .map((u) => ({ u, s: score(source, u.source) }))
    .filter((x) => x.s >= min)
    .sort((a, b) => b.s - a.s)
    .slice(0, limit)
    .map(({ u, s }) => {
      const tmTokens = tokenize(u.source);
      const { changedInB } = align(cur, tmTokens);
      return {
        unit: u,
        score: s,
        diff: tmTokens
          .filter((t) => t.kind !== "tag")
          .map((t, i) => ({
            text: t.text,
            changed: changedInB[tmTokens.indexOf(t)] ?? false,
            space: i > 0 && t.kind === "word",
          })),
      };
    });
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

export interface QaIssue {
  kind: "term-missing" | "forbidden" | "tags" | "numbers" | "unedited";
  text: string;
}

/** Terminology issues as the core's term check reports them. */
export interface TermIssue {
  kind: "missing" | "forbidden";
  term?: string;
  expected?: string;
  found?: string;
}

/**
 * Checks a translation: terminology (from the core's term check, which
 * tolerates inflection), tags and numbers.
 */
export function qa(
  source: Run[],
  target: Run[],
  termIssues: TermIssue[],
  sourceLang: string,
  targetLang: string,
): QaIssue[] {
  const out: QaIssue[] = [];
  const t = plain(target);
  if (!t.trim()) return out;
  for (const i of termIssues) {
    if (i.kind === "forbidden")
      out.push({
        kind: "forbidden",
        text: i.term
          ? `“${i.found}” is a forbidden term for “${i.term}”.`
          : `“${i.found}” is a forbidden term.`,
      });
    else
      out.push({
        kind: "term-missing",
        text: `“${i.term}” is usually “${i.expected}”.`,
      });
  }
  const missing = source
    .filter(isTag)
    .filter((s) => !target.some((r) => isTag(r) && tagKey(r) === tagKey(s)));
  if (missing.length)
    out.push({
      kind: "tags",
      text: `${missing.length === 1 ? "A tag is" : `${missing.length} tags are`} missing (press F9 to insert the next one).`,
    });
  const srcNums = numbers(plain(source), sourceLang);
  const tgtValues = numbers(plain(target), targetLang).map((n) => n.value);
  const missingNums = srcNums
    .filter((n) => !tgtValues.includes(n.value))
    .map((n) => n.text);
  if (missingNums.length === 1)
    out.push({
      kind: "numbers",
      text: `The number ${missingNums[0]} from the source is not in the target.`,
    });
  else if (missingNums.length > 1)
    out.push({
      kind: "numbers",
      text: `The numbers ${missingNums.slice(0, -1).join(", ")} and ${missingNums.at(-1)} from the source are not in the target.`,
    });
  return out;
}

/** Languages that write a decimal comma (1.250,00); the rest use a decimal point (1,250.00). */
const DECIMAL_COMMA = new Set([
  "de",
  "fr",
  "es",
  "it",
  "nl",
  "pt",
  "pl",
  "cs",
  "da",
  "sv",
  "nb",
  "fi",
  "ru",
  "tr",
]);

/**
 * Numbers in a text with their value, read with the separators of the
 * language, so 1.250,00 in German and 1,250.00 in English are the same.
 */
export function numbers(
  text: string,
  language: string,
): { text: string; value: number }[] {
  const comma = DECIMAL_COMMA.has(language.split("-")[0]!.toLowerCase());
  const out: { text: string; value: number }[] = [];
  for (const m of text.matchAll(
    /\d+(?:[.,\u00a0\u202f ]\d{3})*(?:[.,]\d+)?/g,
  )) {
    let raw = m[0].replace(/[\u00a0\u202f ]/g, "");
    const decimal = comma ? "," : ".";
    const group = comma ? "." : ",";
    raw = raw.split(group).join("");
    const value = Number(raw.replace(decimal, "."));
    if (!Number.isNaN(value)) out.push({ text: m[0].trim(), value });
  }
  return out;
}

/** Concordance: memory units whose source or target contains the phrase. */
export function concordance(
  phrase: string,
  units: TmUnit[],
  limit = 8,
): { unit: TmUnit; side: "source" | "target"; start: number; end: number }[] {
  const q = phrase.trim().toLocaleLowerCase();
  if (q.length < 2) return [];
  const out: {
    unit: TmUnit;
    side: "source" | "target";
    start: number;
    end: number;
  }[] = [];
  for (const u of units) {
    for (const side of ["source", "target"] as const) {
      const i = plain(u[side]).toLocaleLowerCase().indexOf(q);
      if (i >= 0) {
        out.push({ unit: u, side, start: i, end: i + q.length });
        break;
      }
    }
    if (out.length >= limit) break;
  }
  return out;
}
