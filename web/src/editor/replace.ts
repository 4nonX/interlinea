// Find and replace in translations. A place found lies within the text
// between two tags, so the tags stay where they are. The same rules are
// in the desktop app (editorpage.cpp).

import type { Run } from "./model";
import { normalize } from "./model";

export interface ReplaceOptions {
  matchCase: boolean;
  wholeWords: boolean;
}

/** What to look for, or null when there is nothing to look for. */
export function searchPattern(find: string, o: ReplaceOptions): RegExp | null {
  if (!find) return null;
  const literal = find.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  // A whole word has no letter or digit before or after it.
  const body = o.wholeWords ? `(?<![\\p{L}\\p{N}])${literal}(?![\\p{L}\\p{N}])` : literal;
  return new RegExp(body, o.matchCase ? "gu" : "giu");
}

/** How many places a text holds. */
export function countIn(runs: Run[], re: RegExp): number {
  let n = 0;
  for (const r of runs) if (typeof r === "string") n += [...r.matchAll(re)].length;
  return n;
}

/** The runs with every place replaced, and how many there were. */
export function replaceInRuns(runs: Run[], re: RegExp, to: string): { runs: Run[]; count: number } {
  let count = 0;
  const out = runs.map((r) =>
    typeof r === "string"
      ? r.replace(re, () => {
          count++;
          return to;
        })
      : r,
  );
  return { runs: count ? normalize(out) : runs, count };
}
