// Translation services: any machine translation engine or language model
// behind an address that answers Interlinea's contract (docs/integration.md).
// A team server asks its services itself, so their keys stay there; for
// work on this computer the browser asks, with the key kept in the browser.

import type { CoreRun } from "./model";

/** A translation service kept in this browser. */
export interface LocalProvider {
  name: string;
  url: string;
  /** A header sent with each request, which signs it at the service. */
  headerName: string;
  headerValue: string;
}

export interface Suggestion {
  target: CoreRun[];
  /** The service's translation did not keep the source's tags, and was taken without them. */
  tagsLost: boolean;
}

const key = (r: CoreRun) => `${r.tag?.kind}:${r.tag?.id}`;

/** Content as text a service reads: a pair of tags as <1>…</1>, a tag on its own as <2/>. */
export function markup(runs: CoreRun[]): string {
  return runs
    .map((r) => {
      if (!r.tag) return (r.text ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;");
      return r.tag.kind === "open" ? `<${r.tag.id}>` : r.tag.kind === "close" ? `</${r.tag.id}>` : `<${r.tag.id}/>`;
    })
    .join("");
}

const unescape = (t: string) => t.replace(/&lt;/g, "<").replace(/&amp;/g, "&");

/** Reads what markup writes; null where it has a tag the source lacks, or not each of its tags as often. */
export function parseMarkup(text: string, source: CoreRun[]): CoreRun[] | null {
  const tags = new Map<string, NonNullable<CoreRun["tag"]>>();
  const count = new Map<string, number>();
  for (const r of source)
    if (r.tag) {
      tags.set(key(r), r.tag);
      count.set(key(r), (count.get(key(r)) ?? 0) + 1);
    }
  const out: CoreRun[] = [];
  let rest = text;
  while (rest) {
    const lt = rest.indexOf("<");
    if (lt < 0) {
      out.push({ text: unescape(rest) });
      break;
    }
    const gt = rest.indexOf(">", lt);
    if (gt < 0) return null;
    if (lt > 0) out.push({ text: unescape(rest.slice(0, lt)) });
    let id = rest.slice(lt + 1, gt);
    rest = rest.slice(gt + 1);
    let kind = "open";
    if (id.startsWith("/")) {
      kind = "close";
      id = id.slice(1);
    } else if (id.endsWith("/")) {
      kind = "standalone";
      id = id.slice(0, -1);
    }
    const tag = tags.get(`${kind}:${id}`);
    if (!tag) return null;
    count.set(`${kind}:${id}`, (count.get(`${kind}:${id}`) ?? 0) - 1);
    out.push({ tag });
  }
  return [...count.values()].every((n) => n === 0) ? out : null;
}

/** A text as markup writes it, without its tags. */
export function stripMarkup(text: string): string {
  return unescape(text.replace(/<[^<>]*>/g, ""));
}

/** What a service's answer gives for a segment. */
export function suggestionOf(text: string, source: CoreRun[]): Suggestion {
  const target = parseMarkup(text, source);
  if (target) return { target, tagsLost: false };
  return { target: [{ text: stripMarkup(text) }], tagsLost: source.some((r) => r.tag) || text.includes("<") };
}

/** Why an address can't be a service's, or nothing. */
export function providerProblem(p: { name: string; url: string; headerName: string; headerValue: string }): string {
  if (!p.name.trim() || /[/?#]/.test(p.name)) return "Give the service a name, without / ? or #.";
  let u: URL | null = null;
  try {
    u = new URL(p.url.trim());
  } catch {
    u = null;
  }
  if (!u || (u.protocol !== "https:" && u.protocol !== "http:")) return "The address starts with https:// or http://.";
  if (/[\s:]/.test(p.headerName.trim())) return "The header's name has no blanks and no colon.";
  if (!p.headerName.trim() && p.headerValue) return "Name the header its value is sent in.";
  return "";
}

/** Asks a service kept in this browser for the translation of one segment. */
export async function askLocalProvider(p: LocalProvider, sourceLang: string, targetLang: string, source: CoreRun[]): Promise<Suggestion> {
  const headers: Record<string, string> = { "Content-Type": "application/json", Accept: "application/json" };
  if (p.headerName) headers[p.headerName] = p.headerValue;
  let res: Response;
  try {
    res = await fetch(p.url, { method: "POST", headers, body: JSON.stringify({ sourceLang, targetLang, segments: [{ id: "1", text: markup(source) }] }) });
  } catch {
    throw new Error(`${p.name} did not translate: its address could not be reached from this browser. A service must allow requests from this page (CORS); one that does not is asked through a team server or the desktop app.`);
  }
  if (!res.ok) throw new Error(`${p.name} did not translate: it answered ${res.status} ${(await res.text().catch(() => "")).trim().slice(0, 200)}.`);
  const body = (await res.json().catch(() => null)) as { translations?: { id: string; text: string }[] } | null;
  const t = body?.translations?.find((x) => x.id === "1");
  if (!t) throw new Error(`${p.name} did not translate: its answer has no translation of what was asked.`);
  return suggestionOf(t.text, source);
}
