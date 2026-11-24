// Documents chosen for a project, before and while they become its files:
// what each can be used for, and the segments each target language gets.
// Used by the New project flow and by a project's settings page.

import {
  LANGUAGES,
  freshCopy,
  primary,
  type FileKind,
  type FileRole,
  type LangCode,
  type ProjectFile,
} from "./data";
import { extractDocx, type Extracted } from "./core";
import { segmentText, type Segment } from "./editor/model";
import type { PendingDoc } from "./draft";

/** A document chosen for a project, with what it is used for. */
export interface DraftDoc {
  key: string;
  doc: PendingDoc;
  role: FileRole;
}

let docSeq = 0;

export function draftDocs(docs: PendingDoc[]): DraftDoc[] {
  return docs.map((doc) => ({
    key: `d${++docSeq}`,
    doc,
    // What Interlinea can't read travels with the project for reference.
    role: readable(doc) ? "translatable" : "reference",
  }));
}

export function kindOf(doc: PendingDoc): FileKind {
  if (doc.bilingual) return "xliff";
  if (doc.bytes && /\.docx$/i.test(doc.file)) return "docx";
  return doc.text ? "text" : "none";
}

/** Whether the document can be translated here. */
export function readable(doc: PendingDoc): boolean {
  if (doc.error) return false;
  if (doc.bilingual) return !!doc.bilingual.targetLang;
  return kindOf(doc) !== "none";
}

export function docName(d: DraftDoc): string {
  return d.doc.path ?? d.doc.file;
}

/** The target language a bilingual document is made for. */
export function docTarget(d: DraftDoc): LangCode | undefined {
  return d.role === "translatable" && d.doc.bilingual?.targetLang
    ? knownLang(d.doc.bilingual.targetLang)
    : undefined;
}

/** A language code as the language list has it (de-de becomes de-DE). */
export function knownLang(code: string): LangCode {
  const lower = code.toLowerCase().replace("_", "-");
  const exact = LANGUAGES.find((l) => l.code.toLowerCase() === lower);
  if (exact) return exact.code;
  if (!lower.includes("-")) {
    const byPrimary = LANGUAGES.find((l) => primary(l.code) === lower);
    if (byPrimary) return byPrimary.code;
  }
  const [p, r] = lower.split("-");
  return r ? `${p}-${r.toUpperCase()}` : lower;
}

/**
 * Word documents extracted with the project's source language, so their
 * sentences break as that language does. Keyed by document.
 */
export async function extractWordDocs(
  docs: DraftDoc[],
  source: LangCode,
): Promise<Map<string, Extracted>> {
  const out = new Map<string, Extracted>();
  for (const d of docs) {
    if (d.role === "translatable" && kindOf(d.doc) === "docx")
      out.set(d.key, await extractDocx(d.doc.bytes!, source));
  }
  return out;
}

/**
 * The documents as project files: reference files keep their bytes, a
 * bilingual file its segments for its target language, and every other
 * file the same segments for each target language. Returns null while a
 * Word document is not extracted yet.
 */
export function buildFiles(
  docs: DraftDoc[],
  words: Map<string, Extracted>,
  targets: LangCode[],
  id: (i: number) => string,
): ProjectFile[] | null {
  const out: ProjectFile[] = [];
  for (const [i, d] of docs.entries()) {
    const base = {
      id: id(i),
      name: docName(d),
      role: d.role,
      words: d.doc.words,
      bytes: d.doc.bytes,
      warnings: d.doc.warnings,
    };
    if (d.role === "reference") {
      out.push({ ...base, kind: kindOf(d.doc), segments: {} });
      continue;
    }
    if (d.doc.bilingual) {
      const t = knownLang(d.doc.bilingual.targetLang!);
      out.push({
        ...base,
        kind: "xliff",
        targetLang: t,
        packagePath: d.doc.sdlPackage?.path,
        blocks: d.doc.bilingual.blocks,
        segments: { [t]: d.doc.bilingual.segments },
      });
      continue;
    }
    let segs: Segment[];
    let blocks;
    if (kindOf(d.doc) === "docx") {
      const ex = words.get(d.key);
      if (!ex) return null;
      segs = ex.segments;
      blocks = ex.blocks;
    } else {
      segs = segmentText(d.doc.text ?? "").map((t, j) => ({
        id: `s${j + 1}`,
        source: [t],
        target: [],
        status: "new" as const,
      }));
    }
    out.push({
      ...base,
      kind: kindOf(d.doc),
      blocks,
      segments: Object.fromEntries(
        targets.map((t, j) => [t, j === 0 ? segs : freshCopy(segs)]),
      ),
    });
  }
  return out;
}
