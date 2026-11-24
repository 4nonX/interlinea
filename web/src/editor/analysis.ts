// Match scores as Trados Studio shows them: context matches (CM, counted as
// 101%), exact 100% matches, and fuzzy bands. The analysis report itself
// comes from the core (project.ts).

import { diffAgainst, type TmMatch } from "./match";
import { plain, type Segment } from "./model";
import { lookupAll, type MemHit } from "./memory";
import type { CoreApi } from "../core";
import type { ContextRule } from "../data";
import type { MemoryRef } from "../project";

export type Band = "cm" | "rep" | "100" | "95" | "85" | "75" | "50" | "new";

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
  if (context && score === 100)
    return "Context match (101%): the text and the segment before it match";
  if (score === 100) return "Exact match (100%)";
  return `Fuzzy match, ${score}%`;
}

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
export function scoredLookup(
  c: CoreApi,
  memories: MemoryRef[],
  segments: Segment[],
  index: number,
  rule: ContextRule,
  min = 70,
  limit = 3,
): ScoredMatch[] {
  const seg = segments[index];
  if (!seg) return [];
  const prev = index > 0 ? plain(segments[index - 1]!.source) : "";
  const next =
    index + 1 < segments.length ? plain(segments[index + 1]!.source) : "";
  return lookupAll(c, memories, seg.source, min, limit + 3)
    .map((h) => ({
      unit: h,
      score: h.score,
      diff: diffAgainst(seg.source, h.source),
      context:
        h.score === 100 &&
        h.prev !== undefined &&
        h.prev === prev &&
        (rule === "previous" || (h.next !== undefined && h.next === next)),
    }))
    .sort((a, b) => Number(b.context) - Number(a.context) || b.score - a.score)
    .slice(0, limit);
}
