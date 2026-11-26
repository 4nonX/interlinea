// Segment content for the editor: text interleaved with inline tags, the same
// model as the Go core's content package.

import hofmann from "./hofmann.json";
import hofmannDocx from "../../samples/Mietvertrag_Hofmann_2026.docx?inline";

export interface TagRun {
  kind: "open" | "close" | "standalone";
  /** Pairs open and close tags inside a segment. */
  id: string;
  /** Short label shown on the chip, e.g. "b", "fn", "br". */
  label: string;
  /** What the tag stands for in the file, as the core reports it ("bold", "link", "footnote"). */
  type?: string;
  /** A description for people ("Colour #C00000", "Field PAGE"). */
  desc?: string;
}

/** A paragraph of the original document (the core's filter.Block). */
export interface Block {
  id: string;
  part:
    | "body"
    | "footnote"
    | "endnote"
    | "header"
    | "footer"
    | "comment"
    | "chart"
    | "diagram"
    | "notes"
    | "attribute"
    | "meta";
  file: string;
  style?: string;
  styleName?: string;
  heading?: number;
  list?: boolean;
  align?: string;
  cell?: { table: number; row: number; col: number };
  note?: string;
  frame?: string;
  revisions?: boolean;
  /** Tag types kept at the paragraph edges, outside the segments. */
  lead?: string[];
  trail?: string[];
}

/** A run as the core encodes it (content.Run). */
export interface CoreRun {
  text?: string;
  tag?: {
    kind: TagRun["kind"];
    id: string;
    type?: string;
    desc?: string;
    native?: string;
  };
}

/** Short chip labels for the tag types the filter reports. */
export const TAG_LABELS: Record<string, string> = {
  bold: "b",
  italic: "i",
  underline: "u",
  superscript: "sup",
  subscript: "sub",
  format: "fmt",
  strike: "s",
  color: "col",
  highlight: "hl",
  font: "font",
  size: "size",
  caps: "Aa",
  style: "sty",
  hidden: "hid",
  link: "link",
  insertion: "ins",
  deletion: "del",
  end: "¶",
  footnote: "fn",
  "note-mark": "fn",
  tab: "tab",
  lb: "br",
  pagebreak: "page",
  columnbreak: "col",
  field: "field",
  image: "img",
  symbol: "sym",
  anchor: "bm",
  comment: "cmt",
  control: "ctl",
  math: "math",
  inline: "x",
};

export function coreToRuns(runs: CoreRun[]): Run[] {
  return runs.map((r) =>
    r.tag
      ? {
          kind: r.tag.kind,
          id: r.tag.id,
          label: TAG_LABELS[r.tag.type ?? ""] ?? "x",
          type: r.tag.type,
          desc: r.tag.desc,
        }
      : (r.text ?? ""),
  );
}

export function runsToCore(runs: Run[]): CoreRun[] {
  return runs
    .filter((r) => r !== "")
    .map((r) =>
      typeof r === "string" ? { text: r } : { tag: { kind: r.kind, id: r.id } },
    );
}

/** Runs for the core with what a tag is, so that it comes back as it went. */
export function runsToCoreWhole(runs: Run[]): CoreRun[] {
  return runs
    .filter((r) => r !== "")
    .map((r) =>
      typeof r === "string" ? { text: r } : { tag: { kind: r.kind, id: r.id, type: r.type, desc: r.desc } },
    );
}

export type Run = string | TagRun;

/** When a subtitle is shown, in milliseconds; end no later than start where the file does not say. */
export interface Timing {
  start: number;
  end: number;
  /** The most characters a line of the file's format holds (32 in closed captions). */
  lineLimit?: number;
}

/** A remark on a segment by someone working on the project. */
/** The kinds of issue a reviewer marks, and what they are called. */
export const ISSUE_CATEGORIES = [
  ["accuracy", "Accuracy"],
  ["fluency", "Fluency"],
  ["terminology", "Terminology"],
  ["style", "Style"],
  ["locale", "Locale conventions"],
  ["formatting", "Formatting"],
  ["other", "Other"],
] as const;

/** How much an issue weighs: its name and its penalty points. */
export const ISSUE_SEVERITIES = [
  ["minor", "Minor", 1],
  ["major", "Major", 5],
  ["critical", "Critical", 25],
] as const;

export type IssueCategory = (typeof ISSUE_CATEGORIES)[number][0];
export type IssueSeverity = (typeof ISSUE_SEVERITIES)[number][0];

/** Something a reviewer found wrong with a segment's translation. */
export interface SegmentIssue {
  id: string;
  category: IssueCategory;
  severity: IssueSeverity;
  note?: string;
  by: string;
  at: string;
}

export interface SegmentComment {
  id: string;
  text: string;
  by: string;
  at: string;
}

export type SegmentStatus = "new" | "draft" | "confirmed";

export interface Segment {
  id: string;
  /** The paragraph of the original document this segment is in. */
  block?: string;
  source: Run[];
  target: Run[];
  status: SegmentStatus;
  /** Where the current target came from, if it was inserted. */
  origin?:
    | { kind: "tm"; score: number; inserted: string; context?: boolean }
    | { kind: "copy" }
    | { kind: "repetition"; from: string };
  /** Check results saved when the segment was confirmed (as memoQ and Phrase do). */
  warnings?: { kind: string; text: string }[];
  /** Warnings the translator marked as false alarms. */
  ignored?: string[];
  /** Bilingual files (XLIFF): the segment is locked there and cannot be edited. */
  locked?: boolean;
  /** Why a locked segment is locked, when not the file ("Assigned to Carl"). */
  lockedReason?: string;
  /** Counts replacements from the team server, so an open editor shows the new text. */
  rev?: number;
  /** Longest the translation may be, in characters, as the file says. */
  maxLength?: number;
  /** Subtitles: when the subtitle is shown, in milliseconds, and the most a line of the format holds. */
  timing?: Timing;
  /**
   * Bilingual files: the segment's state in the file and the translation it
   * had, so saving keeps a reviewer's approval when nothing changed.
   */
  file?: { state: FileState; target: string; match?: number; /** The file's program keeps its own record of each segment, so it is not split or joined. */ fixed?: boolean };
  /** Bilingual files: comments on the segment. */
  notes?: string[];
  /** A part of a segment of the file that was split: the file's segment, and the space before the part. */
  split?: { unit: string; space?: string };
  /** A segment joined from several of the file: those, as the file has them. */
  joined?: { id: string; source: Run[] }[];
  /** What the people working on the project wrote about the segment, oldest first. */
  comments?: SegmentComment[];
  /** What a reviewer marked as wrong with the translation. */
  issues?: SegmentIssue[];
}

/** States of segments in bilingual files, as the core reports them. */
export type FileState =
  "new" | "draft" | "translated" | "reviewed" | "final" | "rejected";

/** Share of source words in confirmed segments, 0 to 1. */
export function confirmedShare(segments: Segment[]): number {
  const total = segments.reduce((n, sg) => n + wordCount(sg.source), 0) || 1;
  return (
    segments
      .filter((sg) => sg.status === "confirmed")
      .reduce((n, sg) => n + wordCount(sg.source), 0) / total
  );
}

/** Two segments are repetitions when text and tags are identical. */
export function sourceKey(runs: Run[]): string {
  return JSON.stringify(
    normalize(runs).map((r) =>
      typeof r === "string" ? r.trim().replace(/\s+/g, " ") : tagKey(r),
    ),
  );
}

export const isTag = (r: Run): r is TagRun => typeof r !== "string";

export function plain(runs: Run[]): string {
  return runs.filter((r): r is string => typeof r === "string").join("");
}

/**
 * Runs with a stretch of their text replaced: offset and length count in
 * plain(runs). Null where the stretch has a tag in it, which a replacement
 * would have to place.
 */
export function replaceStretch(runs: Run[], offset: number, length: number, value: string): Run[] | null {
  let at = 0;
  for (let i = 0; i < runs.length; i++) {
    const r = runs[i]!;
    if (typeof r !== "string") continue;
    if (offset >= at && offset + length <= at + r.length) {
      const text = r.slice(0, offset - at) + value + r.slice(offset - at + length);
      return [...runs.slice(0, i), ...(text ? [text] : []), ...runs.slice(i + 1)];
    }
    at += r.length;
  }
  return null;
}

export function tagsOf(runs: Run[]): TagRun[] {
  return runs.filter(isTag);
}

export function tagKey(t: TagRun): string {
  return `${t.kind}:${t.id}`;
}

/** Tags in the source that the target does not contain yet, in source order. */
export function missingTags(source: Run[], target: Run[]): TagRun[] {
  const have = new Set(tagsOf(target).map(tagKey));
  return tagsOf(source).filter((t) => !have.has(tagKey(t)));
}

export function normalize(runs: Run[]): Run[] {
  const out: Run[] = [];
  for (const r of runs) {
    if (typeof r === "string") {
      if (!r) continue;
      const last = out[out.length - 1];
      if (typeof last === "string") out[out.length - 1] = last + r;
      else out.push(r);
    } else out.push(r);
  }
  return out;
}

export function wordCount(runs: Run[]): number {
  return plain(runs).match(/[\p{L}\p{N}]+/gu)?.length ?? 0;
}

// The sample lease is a real Word document (samples), bundled into
// the page as a data URL so the page needs no file hosting. Its units come
// from the core's Word filter (make sample regenerates hofmann.json), so the
// editor, the preview and "Save translated document" all work on the file.
const SAMPLE = hofmann as {
  units: { id: string; block: string; source: CoreRun[] }[];
  blocks: Block[];
};

const tag = (kind: TagRun["kind"], id: string, type: string): TagRun => ({
  kind,
  id,
  type,
  label: TAG_LABELS[type] ?? "x",
});
const bold = (id: string, text: string): Run[] => [
  tag("open", id, "bold"),
  text,
  tag("close", id, "bold"),
];

/** Work done in the sample before it opens: the first segments are translated. */
const SAMPLE_WORK: Record<string, { target: Run[]; status: SegmentStatus }> = {
  "document/p1/s1": {
    target: ["Residential lease agreement"],
    status: "confirmed",
  },
  "document/p2/s1": {
    target: [
      "The following lease agreement is concluded between ",
      ...bold("1", "Mr Jonas Hofmann"),
      " (landlord) and ",
      ...bold("2", "Ms Lea Kraus"),
      " (tenant):",
    ],
    status: "confirmed",
  },
  "document/p3/s1": { target: ["§ 1 Leased property"], status: "confirmed" },
  "document/p4/s1": {
    target: [
      "The flat on the second floor of the building at Lindenstraße 12, 80331 Munich, is let.",
    ],
    status: "confirmed",
  },
  "document/p4/s2": {
    target: [
      "The leased property comprises three rooms, a kitchen, a bathroom and a cellar room.",
    ],
    status: "confirmed",
  },
  "document/p5/s1": { target: ["§ 2 Term of the lease"], status: "confirmed" },
  "document/p6/s1": {
    target: [
      "The tenancy commences on 1 January 2027 and runs for an indefinite period.",
    ],
    status: "confirmed",
  },
  "document/p6/s2": {
    target: [
      "Termination with notice is governed by the statutory provisions.",
    ],
    status: "draft",
  },
};

export const HOFMANN_BLOCKS: Block[] = SAMPLE.blocks;
export const HOFMANN_FILE: string = hofmannDocx;

export const HOFMANN_SEGMENTS: Segment[] = SAMPLE.units.map((u) => ({
  id: u.id,
  block: u.block,
  source: coreToRuns(u.source),
  target: SAMPLE_WORK[u.id]?.target ?? [],
  status: SAMPLE_WORK[u.id]?.status ?? "new",
}));

export interface TmUnit {
  source: Run[];
  target: Run[];
  project: string;
  by: string;
  at: string;
  /** Plain source of the segments before and after it when it was translated (context match). */
  prev?: string;
  next?: string;
}

/** The invented translator the sample's earlier work is signed with. */
export const SAMPLE_TRANSLATOR = "Anna Lindner";

/** The sample translation memory "Kanzlei Weber (DE-EN)". Every unit is invented. */
export const TM_UNITS: TmUnit[] = [
  {
    source: [
      "Die monatliche Miete beträgt ",
      ...bold("1", "980,00 EUR"),
      " zuzüglich einer Vorauszahlung auf die Betriebskosten in Höhe von ",
      ...bold("2", "180,00 EUR"),
      ".",
    ],
    target: [
      "The monthly rent is ",
      ...bold("1", "EUR 980.00"),
      " plus an advance payment on the service charges of ",
      ...bold("2", "EUR 180.00"),
      ".",
    ],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:10:00Z",
  },
  {
    source: ["Nettokaltmiete"],
    target: ["Basic rent"],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:11:00Z",
  },
  {
    source: ["Gesamtmiete"],
    target: ["Total rent"],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:11:30Z",
  },
  {
    source: [
      "Die Miete ist monatlich im Voraus, spätestens am dritten Werktag eines Monats, zu zahlen.",
    ],
    target: [
      "The rent is payable monthly in advance, no later than the third working day of each month.",
    ],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:12:00Z",
  },
  {
    source: ["Über die Betriebskosten wird jährlich abgerechnet."],
    target: ["Service charges are settled annually."],
    prev: "Die Miete ist monatlich im Voraus, spätestens am dritten Werktag eines Monats, auf das Konto des Vermieters zu zahlen.",
    next: "§ 4 Kaution",
    project: "Gewerbemietvertrag Lang",
    by: SAMPLE_TRANSLATOR,
    at: "2025-03-04T10:40:00Z",
  },
  {
    source: [
      "Der Mieter leistet eine Kaution in Höhe von zwei Nettokaltmieten.",
    ],
    target: ["The tenant shall pay a deposit equal to two months' basic rent."],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:20:00Z",
  },
  {
    source: [
      "Die Kaution wird nach Beendigung des Mietverhältnisses zurückgezahlt.",
    ],
    target: ["The deposit is repaid after the end of the tenancy."],
    project: "Mietvertrag Schuster",
    by: "Eva Brandt",
    at: "2025-09-18T11:00:00Z",
  },
  {
    source: ["Eine Untervermietung bedarf der Zustimmung des Vermieters."],
    target: ["Subletting requires the landlord's consent."],
    project: "Gewerbemietvertrag Lang",
    by: SAMPLE_TRANSLATOR,
    at: "2025-03-04T11:02:00Z",
  },
  {
    source: ["Änderungen dieses Vertrages bedürfen der Schriftform."],
    target: ["Amendments to this agreement must be made in writing."],
    project: "Gewerbemietvertrag Lang",
    by: SAMPLE_TRANSLATOR,
    at: "2025-03-04T11:30:00Z",
  },
  {
    source: [
      "Sollte eine Bestimmung dieses Vertrages unwirksam sein, bleibt die Wirksamkeit der übrigen Bestimmungen unberührt.",
    ],
    target: [
      "Should any provision of this agreement be invalid, the validity of the remaining provisions shall remain unaffected.",
    ],
    prev: "Änderungen dieses Vertrages bedürfen der Schriftform.",
    project: "Gewerbemietvertrag Lang",
    by: SAMPLE_TRANSLATOR,
    at: "2025-03-04T11:31:00Z",
  },
  {
    source: ["Ort, Datum"],
    target: ["Place, date"],
    project: "Gewerbemietvertrag Lang",
    by: SAMPLE_TRANSLATOR,
    at: "2025-03-04T11:40:00Z",
  },
  {
    source: ["Siehe § 536 BGB."],
    target: ["See section 536 BGB."],
    project: "Mietvertrag Schuster",
    by: "Eva Brandt",
    at: "2025-09-18T11:05:00Z",
  },
  {
    source: ["Mietvertrag Schuster, Entwurf vom 22. April 2025"],
    target: ["Lease Schuster, draft of 22 April 2025"],
    project: "Mietvertrag Schuster",
    by: SAMPLE_TRANSLATOR,
    at: "2025-04-22T16:00:00Z",
  },
];
