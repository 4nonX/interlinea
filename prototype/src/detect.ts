// Source language detection for the prototype.
//
// The real product detects languages in the Go core (n-gram models). This
// stand-in scores frequent function words and scripts, which is reliable on
// a page of running text and honest about uncertainty on short or mixed text.

import type { LangCode } from "./data";
import { countWords, type PendingDoc } from "./draft";
import { extractDocx } from "./core";
import { plain } from "./editor/model";

export interface Detection {
  /** Best guess, or null when the text gives no usable evidence. */
  code: LangCode | null;
  /** 0 to 1. Below CONFIDENT the UI asks the user to confirm. */
  confidence: number;
  /** Runner-up guesses, best first. */
  alternatives: { code: LangCode; confidence: number }[];
  /** Amount of text the guess is based on. */
  basis: number;
  /** Unit of basis: characters for scripts written without spaces. */
  unit: "words" | "characters";
  reason: "ok" | "empty" | "short" | "mixed";
}

export const CONFIDENT = 0.75;

const STOPWORDS: Record<LangCode, string[]> = {
  "de-DE": ["der", "die", "das", "und", "ist", "nicht", "mit", "von", "dem", "den", "des", "ein", "eine", "zu", "für", "auf", "im", "bei", "nach", "sich", "wird", "werden", "oder", "auch", "über"],
  "en-GB": ["the", "and", "of", "to", "in", "is", "for", "on", "with", "by", "this", "that", "shall", "be", "or", "any", "may", "not", "are", "as", "an", "each", "between", "which"],
  "fr-FR": ["le", "la", "les", "et", "de", "des", "du", "un", "une", "est", "à", "en", "il", "pour", "dans", "par", "sur", "au", "aux", "qui", "que", "ne", "pas", "ce", "cette", "ces", "son", "sa", "avec", "sont", "être", "doit", "entre", "présent"],
  "es-ES": ["el", "los", "las", "y", "del", "que", "por", "con", "para", "una", "es", "se", "al", "su", "como", "entre", "este", "está"],
  "it-IT": ["lo", "gli", "e", "di", "del", "della", "che", "per", "con", "un", "una", "è", "non", "sono", "tra", "nel", "alla", "questo", "essere"],
  "nl-NL": ["de", "het", "een", "en", "van", "in", "is", "op", "te", "dat", "met", "voor", "niet", "zijn", "door", "aan", "bij", "wordt", "tussen"],
  "pl-PL": ["i", "w", "z", "na", "się", "do", "nie", "że", "jest", "to", "przez", "od", "dla", "lub", "oraz", "jego", "po", "być"],
};

const SCRIPTS: { code: LangCode; re: RegExp }[] = [
  { code: "ko-KR", re: /[가-힯ᄀ-ᇿ]/gu },
  { code: "ja-JP", re: /[぀-ヿ]/gu },
  { code: "zh-CN", re: /[一-鿿]/gu },
  { code: "ar-SA", re: /[؀-ۿ]/gu },
];

export function detectLanguage(text: string): Detection {
  const trimmed = text.trim();
  if (!trimmed) return { code: null, confidence: 0, alternatives: [], basis: 0, unit: "words", reason: "empty" };

  // Scripts with their own writing system decide by character share. Kana
  // decides Japanese even though Japanese text also contains Han characters.
  const letters = [...trimmed].filter((c) => /\p{L}/u.test(c)).length || 1;
  const scriptCounts = SCRIPTS.map(({ code, re }) => ({ code, n: trimmed.match(re)?.length ?? 0 }));
  const kana = scriptCounts.find((s) => s.code === "ja-JP")!.n;
  const best = scriptCounts.filter((s) => s.n > 0).sort((a, b) => b.n - a.n)[0];
  if (best && best.n / letters > 0.3) {
    const code = kana > 0 && best.code === "zh-CN" ? "ja-JP" : best.code;
    const share = Math.min(1, (best.n + (code === "ja-JP" ? kana : 0)) / letters);
    return { code, confidence: Math.min(0.99, 0.6 + share * 0.4), alternatives: [], basis: best.n, unit: "characters", reason: "ok" };
  }

  const words = trimmed.toLowerCase().match(/\p{L}+/gu) ?? [];
  if (words.length < 8) {
    return { code: null, confidence: 0, alternatives: [], basis: words.length, unit: "words", reason: "short" };
  }
  const scores = Object.entries(STOPWORDS).map(([code, list]) => {
    const set = new Set(list);
    let hits = 0;
    for (const w of words) if (set.has(w)) hits++;
    return { code, hits };
  });
  // Characters that only some languages use break near-ties.
  const bonus: Record<LangCode, RegExp> = { "de-DE": /[äöüß]/u, "fr-FR": /[éèêàçù]/u, "es-ES": /[ñ¿¡]/u, "pl-PL": /[ąęłńśźż]/u };
  for (const s of scores) {
    const re = bonus[s.code];
    if (re && re.test(trimmed)) s.hits += Math.max(1, Math.round(words.length * 0.01));
  }
  scores.sort((a, b) => b.hits - a.hits);
  const total = scores.reduce((sum, s) => sum + s.hits, 0);
  if (total === 0) return { code: null, confidence: 0, alternatives: [], basis: words.length, unit: "words", reason: "mixed" };

  const [top, second] = scores;
  const share = top!.hits / total;
  const margin = second ? (top!.hits - second.hits) / top!.hits : 1;
  // Confidence grows with the share of function words that point the same
  // way and with the amount of text.
  const amount = Math.min(1, words.length / 40);
  const confidence = Math.max(0, Math.min(0.99, (share * 0.6 + margin * 0.4) * (0.55 + 0.45 * amount)));
  return {
    code: top!.code,
    confidence,
    alternatives: scores.slice(1, 3).filter((s) => s.hits > 0).map((s) => ({ code: s.code, confidence: s.hits / total })),
    basis: words.length,
    unit: "words",
    reason: confidence < CONFIDENT && share < 0.6 ? "mixed" : "ok",
  };
}

/** Reads the text of a dropped file when the browser can (plain text formats). */
export async function readText(file: File): Promise<string | null> {
  const textual = /\.(txt|md|csv|tsv|html?|xml|xliff|xlf|sdlxliff|json|srt|po)$/i.test(file.name) || file.type.startsWith("text/");
  if (!textual) return null;
  const text = await file.text();
  return text.slice(0, 20000);
}

/**
 * Reads a chosen file for the New project flow. Word documents go through
 * the core's filter (WebAssembly), which also keeps their formatting for
 * the translated file; plain text formats are read directly.
 */
export async function readDocument(file: File): Promise<PendingDoc> {
  if (/\.docx$/i.test(file.name)) {
    let bytes: Uint8Array;
    try {
      bytes = new Uint8Array(await file.arrayBuffer());
    } catch (e) {
      return { file: file.name, words: 0, text: null, error: `The file could not be read (${e instanceof Error ? e.message : String(e)}). Check that it still exists and is not open elsewhere.` };
    }
    try {
      // Sentence rules barely matter for detection; the units are extracted
      // again with the confirmed source language when the project is created.
      const ex = await extractDocx(bytes, "en");
      const text = ex.segments.map((s) => plain(s.source)).join("\n");
      return { file: file.name, words: countWords(text), text: text.slice(0, 20000), bytes, warnings: ex.warnings };
    } catch (e) {
      return { file: file.name, words: 0, text: null, error: e instanceof Error ? e.message : String(e) };
    }
  }
  try {
    const text = await readText(file);
    return { file: file.name, words: text ? countWords(text) : 0, text };
  } catch (e) {
    return { file: file.name, words: 0, text: null, error: `The file could not be read (${e instanceof Error ? e.message : String(e)}). Check that it still exists and is not open elsewhere.` };
  }
}
