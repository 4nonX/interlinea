// Match scores and analysis as Trados Studio shows them: context matches
// (CM, counted as 101%), exact 100% matches, and fuzzy bands. The analysis
// is the basis translators and agencies quote from.

import { diffAgainst, type TmMatch } from "./match";
import { plain, sourceKey, wordCount, type Segment } from "./model";
import { lookupAll, type MemHit, type MemoryInfo } from "./memory";
import type { core } from "../core";

type Core = NonNullable<ReturnType<typeof core>>;

export type Band = "cm" | "rep" | "100" | "95" | "85" | "75" | "50" | "new";

export const BANDS: { id: Band; label: string; hint: string }[] = [
  { id: "cm", label: "Context match", hint: "100% match whose preceding segment also matches (101%)" },
  { id: "rep", label: "Repetitions", hint: "Text and tags occur earlier in this document" },
  { id: "100", label: "100%", hint: "Same text and tags as a memory unit" },
  { id: "95", label: "95% to 99%", hint: "" },
  { id: "85", label: "85% to 94%", hint: "" },
  { id: "75", label: "75% to 84%", hint: "" },
  { id: "50", label: "50% to 74%", hint: "Usually charged like new text" },
  { id: "new", label: "New", hint: "No match of 50% or more" },
];

export function bandOf(score: number, context = false): Band {
  if (context && score === 100) return "cm";
  if (score >= 100) return "100";
  if (score >= 95) return "95";
  if (score >= 85) return "85";
  if (score >= 75) return "75";
  if (score >= 50) return "50";
  return "new";
}

/** The label Trados shows in the status column: "CM" or the percentage. */
export function scoreLabel(score: number, context = false): string {
  return context && score === 100 ? "CM" : `${score}%`;
}

export function scoreDescription(score: number, context = false): string {
  if (context && score === 100) return "Context match (101%): the text and the segment before it match";
  if (score === 100) return "Exact match (100%)";
  return `Fuzzy match, ${score}%`;
}

export type ContextRule = "previous" | "both";

export interface ScoredMatch {
  unit: MemHit;
  score: number;
  /** Memory source tokens, the ones that differ from the segment marked. */
  diff: TmMatch["diff"];
  /** A 100% match whose neighbouring segments also match: a context match. */
  context: boolean;
}

/**
 * Looks a segment up and marks context matches: Trados compares the
 * segment before ("previous"), memoQ the segments before and after ("both").
 */
export function scoredLookup(c: Core, memories: MemoryInfo[], segments: Segment[], index: number, rule: ContextRule, min = 70, limit = 3): ScoredMatch[] {
  const seg = segments[index];
  if (!seg) return [];
  const prev = index > 0 ? plain(segments[index - 1]!.source) : "";
  const next = index + 1 < segments.length ? plain(segments[index + 1]!.source) : "";
  return lookupAll(c, memories, seg.source, min, limit + 3)
    .filter((h) => !(h.memory.kind === "document" && h.docSegment === index + 1))
    .map((h) => ({
      unit: h,
      score: h.score,
      diff: diffAgainst(seg.source, h.source),
      context: h.score === 100 && h.prev !== undefined && h.prev === prev && (rule === "previous" || (h.next !== undefined && h.next === next)),
    }))
    .sort((a, b) => Number(b.context) - Number(a.context) || b.score - a.score)
    .slice(0, limit);
}

export interface AnalysisRow {
  band: Band;
  segments: number;
  words: number;
}

/**
 * Analyses a document against a memory, Trados-style: a context match wins,
 * then later occurrences of a repeated segment count as repetitions, then
 * the best match decides the band. Matches below 50% count as new.
 */
export function analyze(c: Core, memories: MemoryInfo[], segments: Segment[], rule: ContextRule): { rows: AnalysisRow[]; words: number; segments: number } {
  const rows = new Map<Band, AnalysisRow>(BANDS.map((b) => [b.id, { band: b.id, segments: 0, words: 0 }]));
  const seen = new Set<string>();
  let words = 0;
  segments.forEach((s, i) => {
    const w = wordCount(s.source);
    words += w;
    const best = memories.length ? scoredLookup(c, memories, segments, i, rule, 50, 1)[0] : undefined;
    const key = sourceKey(s.source);
    let band: Band;
    if (best?.context) band = "cm";
    else if (seen.has(key)) band = "rep";
    else band = best ? bandOf(best.score) : "new";
    seen.add(key);
    const row = rows.get(band)!;
    row.segments++;
    row.words += w;
  });
  return { rows: [...rows.values()], words, segments: segments.length };
}
