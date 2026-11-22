// Hands a chosen document from the Projects page to the New project flow.

export interface PendingDoc {
  file: string;
  words: number;
  /** Extracted text, or null when the prototype cannot read this format. */
  text: string | null;
  client?: string;
  /** The file itself when the core's Word filter can read it (.docx). */
  bytes?: Uint8Array;
  /** Why the text could not be read, when the filter failed. */
  error?: string;
  /** What the filter changes or cannot translate (tracked changes, for example). */
  warnings?: string[];
}

let pending: PendingDoc | null = null;

export function setPendingDoc(doc: PendingDoc) {
  pending = doc;
}

export function takePendingDoc(): PendingDoc | null {
  const d = pending;
  pending = null;
  return d;
}

export function countWords(text: string): number {
  // Scripts without spaces count characters the way CAT tools do for CJK.
  const cjk = text.match(/[぀-ヿ一-鿿]/gu)?.length ?? 0;
  const words = text.replace(/[぀-ヿ一-鿿]/gu, " ").match(/[\p{L}\p{N}]+/gu)?.length ?? 0;
  return words + cjk;
}
