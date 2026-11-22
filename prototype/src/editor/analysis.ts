// Match scores and analysis as Trados Studio shows them: context matches
// (CM, counted as 101%), exact 100% matches, and fuzzy bands. The analysis
// is the basis translators and agencies quote from.

import { lookup, type TmMatch } from "./match";
import { plain, sourceKey, TM_UNITS, wordCount, type Segment, type TmUnit } from "./model";
import type { Project } from "../data";

/** The client's translation memory, if the prototype has one for the pair. */
export function clientMemory(p: Pick<Project, "client" | "sourceLang" | "targetLangs">): TmUnit[] {
  return p.client === "Kanzlei Weber" && p.sourceLang.startsWith("de") && p.targetLangs[0]?.startsWith("en") ? TM_UNITS : [];
}

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

export interface ScoredMatch extends TmMatch {
  /** The memory unit's preceding segment equals this segment's: a context match. */
  context: boolean;
}

/** Looks up a segment and marks context matches. */
export function scoredLookup(segments: Segment[], index: number, units: TmUnit[], min = 70, limit = 3): ScoredMatch[] {
  const seg = segments[index];
  if (!seg) return [];
  const prev = index > 0 ? plain(segments[index - 1]!.source) : "";
  return lookup(seg.source, units, min, limit + 3)
    .map((m) => ({ ...m, context: m.score === 100 && m.unit.prev !== undefined && m.unit.prev === prev }))
    .sort((a, b) => Number(b.context) - Number(a.context) || b.score - a.score)
    .slice(0, limit);
}

/** Units of a document's own confirmed segments, with their context. */
export function documentUnits(segments: Segment[], projectName: string, by: string, at: string): TmUnit[] {
  const out: TmUnit[] = [];
  segments.forEach((s, i) => {
    if (s.status !== "confirmed" || !plain(s.target).trim()) return;
    out.push({ source: s.source, target: s.target, project: projectName, by, at, docSegment: i + 1, prev: i > 0 ? plain(segments[i - 1]!.source) : "" });
  });
  return out;
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
export function analyze(segments: Segment[], units: TmUnit[]): { rows: AnalysisRow[]; words: number; segments: number } {
  const rows = new Map<Band, AnalysisRow>(BANDS.map((b) => [b.id, { band: b.id, segments: 0, words: 0 }]));
  const seen = new Set<string>();
  let words = 0;
  segments.forEach((s, i) => {
    const w = wordCount(s.source);
    words += w;
    const best = scoredLookup(segments, i, units, 50, 1)[0];
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
