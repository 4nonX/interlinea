// Hands the documents chosen on the Projects page to the New project flow.

import type { Bilingual } from "./core";

export interface PendingDoc {
  file: string;
  /** Path in the folder or package the file came from, if any. */
  path?: string;
  words: number;
  /** Extracted text, or null when this format cannot be read. */
  text: string | null;
  /** The file itself when the core's Word filter can read it (.docx). */
  bytes?: Uint8Array;
  /** Why the text could not be read, when the filter failed. */
  error?: string;
  /** What the filter changes or cannot translate (tracked changes, for example). */
  warnings?: string[];
  /** A bilingual file (XLIFF): its languages, variant and segments with their translations. */
  bilingual?: Bilingual;
  /** A Trados project package the bilingual file comes from: the package and the file's path in it. */
  sdlPackage?: {
    bytes: Uint8Array;
    name: string;
    path: string;
  };
}

let pending: PendingDoc[] = [];

export function setPendingDocs(docs: PendingDoc[]) {
  pending = docs;
}

export function takePendingDocs(): PendingDoc[] {
  const d = pending;
  pending = [];
  return d;
}

export function countWords(text: string): number {
  // Scripts without spaces count characters the way CAT tools do for CJK.
  const cjk = text.match(/[぀-ヿ一-鿿]/gu)?.length ?? 0;
  const words =
    text.replace(/[぀-ヿ一-鿿]/gu, " ").match(/[\p{L}\p{N}]+/gu)?.length ?? 0;
  return words + cjk;
}
