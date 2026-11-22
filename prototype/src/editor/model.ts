// Segment content for the editor: text interleaved with inline tags, the same
// model as the Go core's content package.

export interface TagRun {
  kind: "open" | "close" | "standalone";
  /** Pairs open and close tags inside a segment. */
  id: string;
  /** Short label shown on the chip, e.g. "b", "fn", "br". */
  label: string;
}

export type Run = string | TagRun;

export type SegmentStatus = "new" | "draft" | "confirmed";

export interface Segment {
  id: string;
  source: Run[];
  target: Run[];
  status: SegmentStatus;
  /** Where the current target came from, if it was inserted. */
  origin?: { kind: "tm"; score: number; inserted: string } | { kind: "copy" } | { kind: "repetition"; from: string };
  /** Check results saved when the segment was confirmed (as memoQ and Phrase do). */
  warnings?: { kind: string; text: string }[];
  /** Warnings the translator marked as false alarms. */
  ignored?: string[];
}

/** Share of source words in confirmed segments, 0 to 1. */
export function confirmedShare(segments: Segment[]): number {
  const total = segments.reduce((n, sg) => n + wordCount(sg.source), 0) || 1;
  return segments.filter((sg) => sg.status === "confirmed").reduce((n, sg) => n + wordCount(sg.source), 0) / total;
}

/** Two segments are repetitions when text and tags are identical. */
export function sourceKey(runs: Run[]): string {
  return JSON.stringify(normalize(runs).map((r) => (typeof r === "string" ? r.trim().replace(/\s+/g, " ") : tagKey(r))));
}

export const isTag = (r: Run): r is TagRun => typeof r !== "string";

export function plain(runs: Run[]): string {
  return runs.filter((r): r is string => typeof r === "string").join("");
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

const ABBREVIATIONS = /(?:^|\s)(?:z\.\s?B|d\.\s?h|u\.\s?a|bzw|ca|Nr|Abs|Art|vgl|ggf|inkl|Dr|Prof|Str|e\.g|i\.e|Mr|Mrs|Ms|No|etc)\.$/;

const MONTHS = /^\s+(?:Januar|Jänner|Februar|März|April|Mai|Juni|Juli|August|September|Oktober|November|Dezember)\b/;

/**
 * Splits text into sentences for new projects. The real product uses the
 * SRX segmenter in the Go core; this keeps the common cases right.
 */
export function segmentText(text: string): string[] {
  const out: string[] = [];
  let start = 0;
  const re = /[.!?。！？]+["“”»«)]*(?=\s+[\p{Lu}\p{N}"„“«(\p{Lo}]|\s*$)/gu;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    const end = m.index + m[0].length;
    const candidate = text.slice(start, end);
    const next = text.slice(end);
    if (ABBREVIATIONS.test(candidate)) continue;
    // Ordinals: "am 1. Januar", "im 2. Obergeschoss".
    if (/\b\d{1,3}\.$/.test(candidate) && (/^\s+\p{Ll}/u.test(next) || MONTHS.test(next))) continue;
    const s = candidate.trim();
    if (s) out.push(s);
    start = end;
  }
  const rest = text.slice(start).trim();
  if (rest) out.push(rest);
  return out;
}

const b = (id: string): [TagRun, TagRun] => [
  { kind: "open", id, label: "b" },
  { kind: "close", id, label: "b" },
];
const fn = (id: string): TagRun => ({ kind: "standalone", id, label: "fn" });

const [b1o, b1c] = b("1");
const [b2o, b2c] = b("2");
const [b3o, b3c] = b("3");
const [b4o, b4c] = b("4");

/** Sample: residential lease, German to English. Synthetic text. */
export const HOFMANN_SEGMENTS: Segment[] = [
  { id: "s1", source: [b1o, "Mietvertrag über Wohnraum", b1c], target: [b1o, "Residential lease agreement", b1c], status: "confirmed" },
  { id: "s2", source: ["Zwischen Herrn Jonas Hofmann (Vermieter) und Frau Lea Kraus (Mieterin) wird folgender Mietvertrag geschlossen:"], target: ["The following lease agreement is concluded between Mr Jonas Hofmann (landlord) and Ms Lea Kraus (tenant):"], status: "confirmed" },
  { id: "s3", source: [b2o, "§ 1 Mietsache", b2c], target: [b2o, "§ 1 Leased property", b2c], status: "confirmed" },
  { id: "s4", source: ["Vermietet wird die Wohnung im zweiten Obergeschoss des Hauses Lindenstraße 12, 80331 München."], target: ["The flat on the second floor of the building at Lindenstraße 12, 80331 Munich, is let."], status: "confirmed" },
  { id: "s5", source: ["Die Mietsache besteht aus drei Zimmern, einer Küche, einem Bad und einem Kellerraum."], target: ["The leased property comprises three rooms, a kitchen, a bathroom and a cellar room."], status: "confirmed" },
  { id: "s6", source: [b3o, "§ 2 Mietzeit", b3c], target: [b3o, "§ 2 Term of the lease", b3c], status: "confirmed" },
  { id: "s7", source: ["Das Mietverhältnis beginnt am 1. Januar 2027 und läuft auf unbestimmte Zeit."], target: ["The tenancy commences on 1 January 2027 and runs for an indefinite period."], status: "confirmed" },
  { id: "s8", source: ["Die ordentliche Kündigung richtet sich nach den gesetzlichen Vorschriften.", fn("f1")], target: ["Termination with notice is governed by the statutory provisions.", fn("f1")], status: "draft" },
  { id: "s9", source: [b4o, "§ 3 Miete und Betriebskosten", b4c], target: [], status: "new" },
  { id: "s10", source: ["Die monatliche Miete beträgt 1.250,00 EUR zuzüglich einer Vorauszahlung auf die Betriebskosten in Höhe von 220,00 EUR."], target: [], status: "new" },
  { id: "s11", source: ["Die Miete ist monatlich im Voraus, spätestens am dritten Werktag eines Monats, auf das Konto des Vermieters zu zahlen."], target: [], status: "new" },
  { id: "s12", source: ["Über die Betriebskosten wird jährlich abgerechnet."], target: [], status: "new" },
  { id: "s13", source: [{ kind: "open", id: "5", label: "b" }, "§ 4 Kaution", { kind: "close", id: "5", label: "b" }], target: [], status: "new" },
  { id: "s14", source: ["Der Mieter leistet eine Kaution in Höhe von drei Nettokaltmieten."], target: [], status: "new" },
  { id: "s15", source: ["Die Kaution ist zu Beginn des Mietverhältnisses fällig und wird nach Beendigung des Mietverhältnisses zurückgezahlt."], target: [], status: "new" },
  { id: "s16", source: [{ kind: "open", id: "6", label: "b" }, "§ 5 Schönheitsreparaturen", { kind: "close", id: "6", label: "b" }], target: [], status: "new" },
  { id: "s17", source: ["Der Mieter übernimmt die Schönheitsreparaturen auf eigene Kosten."], target: [], status: "new" },
  { id: "s18", source: ["Eine Untervermietung bedarf der vorherigen schriftlichen Zustimmung des Vermieters."], target: [], status: "new" },
  { id: "s19", source: ["Das Recht zur Mietminderung bleibt unberührt.", fn("f2")], target: [], status: "new" },
  { id: "s20", source: ["Der Vermieter kann Räumungsklage erheben, wenn der Mieter mit zwei Monatsmieten in Verzug ist."], target: [], status: "new" },
  { id: "s21", source: [{ kind: "open", id: "7", label: "b" }, "§ 6 Schlussbestimmungen", { kind: "close", id: "7", label: "b" }], target: [], status: "new" },
  { id: "s22", source: ["Änderungen und Ergänzungen dieses Vertrages bedürfen der Schriftform."], target: [], status: "new" },
  { id: "s23", source: ["Sollte eine Bestimmung dieses Vertrages unwirksam sein, bleibt die Wirksamkeit der übrigen Bestimmungen unberührt."], target: [], status: "new" },
  { id: "s24", source: ["Ort, Datum"], target: [], status: "new" },
  { id: "s25", source: ["Unterschrift Vermieter"], target: [], status: "new" },
  { id: "s26", source: ["Ort, Datum"], target: [], status: "new" },
  { id: "s27", source: ["Unterschrift Mieter"], target: [], status: "new" },
];

export interface TmUnit {
  source: Run[];
  target: Run[];
  project: string;
  by: string;
  at: string;
  /** Set for segments confirmed earlier in the open document. */
  docSegment?: number;
}

/** Translation memory "Kanzlei Weber (DE-EN)". Synthetic. */
export const TM_UNITS: TmUnit[] = [
  { source: ["Die monatliche Miete beträgt 980,00 EUR zuzüglich einer Vorauszahlung auf die Betriebskosten in Höhe von 180,00 EUR."], target: ["The monthly rent is EUR 980.00 plus an advance payment on the service charges of EUR 180.00."], project: "Mietvertrag Schuster", by: "Dan", at: "2025-04-22T16:10:00Z" },
  { source: ["Die Miete ist monatlich im Voraus, spätestens am dritten Werktag eines Monats, zu zahlen."], target: ["The rent is payable monthly in advance, no later than the third working day of each month."], project: "Mietvertrag Schuster", by: "Dan", at: "2025-04-22T16:12:00Z" },
  { source: ["Über die Betriebskosten wird jährlich abgerechnet."], target: ["Service charges are settled annually."], project: "Gewerbemietvertrag Lang", by: "Dan", at: "2025-03-04T10:40:00Z" },
  { source: ["Der Mieter leistet eine Kaution in Höhe von zwei Nettokaltmieten."], target: ["The tenant shall pay a deposit equal to two months' basic rent."], project: "Mietvertrag Schuster", by: "Dan", at: "2025-04-22T16:20:00Z" },
  { source: ["Die Kaution wird nach Beendigung des Mietverhältnisses zurückgezahlt."], target: ["The deposit is repaid after the end of the tenancy."], project: "Mietvertrag Schuster", by: "Eva Brandt", at: "2025-09-18T11:00:00Z" },
  { source: ["Eine Untervermietung bedarf der Zustimmung des Vermieters."], target: ["Subletting requires the landlord's consent."], project: "Gewerbemietvertrag Lang", by: "Dan", at: "2025-03-04T11:02:00Z" },
  { source: ["Änderungen dieses Vertrages bedürfen der Schriftform."], target: ["Amendments to this agreement must be made in writing."], project: "Gewerbemietvertrag Lang", by: "Dan", at: "2025-03-04T11:30:00Z" },
  { source: ["Sollte eine Bestimmung dieses Vertrages unwirksam sein, bleibt die Wirksamkeit der übrigen Bestimmungen unberührt."], target: ["Should any provision of this agreement be invalid, the validity of the remaining provisions shall remain unaffected."], project: "Gewerbemietvertrag Lang", by: "Dan", at: "2025-03-04T11:31:00Z" },
  { source: [{ kind: "open", id: "1", label: "b" }, "§ 3 Miete", { kind: "close", id: "1", label: "b" }], target: [{ kind: "open", id: "1", label: "b" }, "§ 3 Rent", { kind: "close", id: "1", label: "b" }], project: "Mietvertrag Schuster", by: "Dan", at: "2025-04-22T16:05:00Z" },
  { source: ["Der Vermieter kann fristlos kündigen, wenn der Mieter mit zwei Monatsmieten in Verzug ist."], target: ["The landlord may terminate without notice if the tenant is two months' rent in arrears."], project: "Mietvertrag Schuster", by: "Dan", at: "2025-04-22T16:40:00Z" },
];
