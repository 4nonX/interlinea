// Translation services: any machine translation engine or language model
// behind an address that answers Interlinea's contract (docs/integration.md).
// A team server asks its services itself, so their keys stay there; for
// work on this computer the browser asks, with the key kept in the browser.

import type { CoreRun } from "./model";
import { core, loadCore, unwrap } from "../core";
import { tl } from "../i18n";

/** Which translation service it is: one of one's own, or one built in. */
export type MtService = "own" | "deepl" | "google" | "microsoft" | "openai" | "anthropic";

/** What a service built in needs to be set up (mt.Info). */
export interface MtInfo {
  service: MtService;
  name: string;
  key?: boolean;
  region?: boolean;
  model?: boolean;
  url?: boolean;
  header?: boolean;
  /** A web page may ask it itself. */
  browser?: boolean;
  signup?: string;
  note: string;
}

/** A service kept in this browser. */
export interface LocalProvider {
  name: string;
  /** What it is for: suggesting translations (the default) or checking grammar. */
  kind?: "translate" | "check";
  /** Which service it is; one of one's own when not given. */
  service?: MtService;
  /** The address of a service of one's own, or of one that speaks the OpenAI API. */
  url: string;
  /** A header sent with each request, which signs it at a service of one's own. */
  headerName: string;
  /** The key: the header's value, or the key of a service built in. */
  headerValue: string;
  model?: string;
  /** The region of a Microsoft Translator key. */
  region?: string;
}

/** What package mt is asked with, for a service kept here or being set up. */
export interface MtSettings {
  service: MtService;
  url?: string;
  key?: string;
  headerName?: string;
  model?: string;
  region?: string;
}

export const settingsOf = (p: LocalProvider): MtSettings => ({
  service: p.service ?? "own",
  url: p.url,
  key: p.headerValue,
  headerName: p.headerName,
  model: p.model,
  region: p.region,
});

interface MtRequest {
  method: string;
  url: string;
  header: Record<string, string>;
  body?: string;
}

const coreNow = async () => core() ?? (await loadCore());

/** The services built in, as the core lists them. */
export async function mtServices(): Promise<MtInfo[]> {
  return JSON.parse(unwrap((await coreNow()).mtServices())) as MtInfo[];
}

/** What is wrong with settings, by field. */
export async function mtProblems(s: MtSettings): Promise<{ field: string; message: string }[]> {
  return JSON.parse(unwrap((await coreNow()).mtCheck(JSON.stringify(s))));
}

/** Sends a request to a service; unreachable is what to say when its address can't be reached. */
async function send(r: MtRequest, unreachable: string): Promise<{ status: number; body: string }> {
  let res: Response;
  try {
    res = await fetch(r.url, { method: r.method, headers: r.header, body: r.method === "GET" ? undefined : r.body });
  } catch {
    throw new Error(unreachable);
  }
  return { status: res.status, body: await res.text().catch(() => "") };
}

/** The models a key may use, asked from this browser; null for a service without models. */
export async function localModels(s: MtSettings): Promise<string[] | null> {
  const c = await coreNow();
  const raw = unwrap(c.mtModels(JSON.stringify(s)));
  if (!raw) return null;
  const { status, body } = await send(
    JSON.parse(raw) as MtRequest,
    tl("The service did not list its models: its address could not be reached from this browser. A service must allow requests from this page (CORS); one that does not is asked through a team server or the desktop app."),
  );
  return JSON.parse(unwrap(c.mtModelsAnswer(JSON.stringify(s), status, body))) as string[];
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
  if (!p.name.trim() || /[/?#]/.test(p.name)) return tl("Give the service a name, without / ? or #.");
  let u: URL | null = null;
  try {
    u = new URL(p.url.trim());
  } catch {
    u = null;
  }
  if (!u || (u.protocol !== "https:" && u.protocol !== "http:")) return tl("The address starts with https:// or http://.");
  if (/[\s:]/.test(p.headerName.trim())) return tl("The header's name has no blanks and no colon.");
  if (!p.headerName.trim() && p.headerValue) return tl("Name the header its value is sent in.");
  return "";
}

/** Asks a service kept in this browser for the translation of one segment. */
export async function askLocalProvider(p: LocalProvider, sourceLang: string, targetLang: string, source: CoreRun[]): Promise<Suggestion> {
  const c = await coreNow();
  const settings = JSON.stringify(settingsOf(p));
  const segments = JSON.stringify([{ id: "1", text: markup(source) }]);
  const ask = c.mtTranslate(settings, sourceLang, targetLang, segments);
  if (!ask.ok) throw new Error(tl("{name} did not translate: {error}.", { name: p.name, error: ask.error }));
  const { status, body } = await send(
    JSON.parse(ask.value) as MtRequest,
    tl("{name} did not translate: its address could not be reached from this browser. A service must allow requests from this page (CORS); one that does not is asked through a team server or the desktop app.", { name: p.name }),
  );
  const got = c.mtAnswer(settings, segments, status, body);
  if (!got.ok) throw new Error(tl("{name} did not translate: {error}.", { name: p.name, error: got.error }));
  const t = (JSON.parse(got.value) as { id: string; text: string }[]).find((x) => x.id === "1");
  if (!t) throw new Error(tl("{name} did not translate: its answer has no translation of what was asked.", { name: p.name }));
  return suggestionOf(t.text, source);
}

/** Something a grammar service found in a text. */
export interface GrammarMatch {
  /** Where, in UTF-16 code units of the text. */
  offset: number;
  length: number;
  message: string;
  /** The text there. */
  found: string;
  replacements: string[];
}

/**
 * What a grammar service's answer says about a text: the findings that lie
 * in the text and are not about spelling, which the project's dictionary
 * and its own words decide.
 */
export function readMatches(answer: unknown, text: string): GrammarMatch[] | null {
  const matches = (answer as { matches?: unknown } | null)?.matches;
  if (!Array.isArray(matches)) return null;
  const out: GrammarMatch[] = [];
  for (const raw of matches) {
    const m = raw as {
      message?: string;
      offset?: number;
      length?: number;
      replacements?: { value?: string }[];
      rule?: { issueType?: string; category?: { id?: string } };
    };
    const offset = m.offset ?? -1;
    const length = m.length ?? 0;
    const message = (m.message ?? "").trim();
    if (!Number.isInteger(offset) || !Number.isInteger(length) || offset < 0 || length <= 0 || offset + length > text.length || !message) continue;
    if (m.rule?.category?.id === "TYPOS" || m.rule?.issueType === "misspelling") continue;
    const found = text.slice(offset, offset + length);
    const replacements: string[] = [];
    for (const r of m.replacements ?? []) if (typeof r.value === "string" && r.value !== found && replacements.length < 5) replacements.push(r.value);
    out.push({ offset, length, message, found, replacements });
  }
  return out;
}

/** Asks a grammar service kept in this browser what it finds in a text. */
export async function askLocalChecker(p: LocalProvider, lang: string, text: string): Promise<GrammarMatch[]> {
  if (!text.trim()) return [];
  const ask = async (language: string) => {
    const headers: Record<string, string> = { "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json" };
    if (p.headerName) headers[p.headerName] = p.headerValue;
    try {
      return await fetch(p.url, { method: "POST", headers, body: new URLSearchParams({ text, language, disabledCategories: "TYPOS" }).toString() });
    } catch {
      throw new Error(tl("{name} did not check: its address could not be reached from this browser. A service must allow requests from this page (CORS); one that does not is asked through a team server or the desktop app.", { name: p.name }));
    }
  };
  let res = await ask(lang);
  // A service that refuses a language with a region is asked for the language alone.
  if (res.status >= 400 && res.status < 500 && lang.includes("-")) {
    const again = await ask(lang.split("-")[0]!);
    if (again.ok) res = again;
  }
  if (!res.ok) throw new Error(tl("{name} did not check: it answered {status} {slice}.", { name: p.name, status: res.status, slice: (await res.text().catch(() => "")).trim().slice(0, 200) }));
  const found = readMatches(await res.json().catch(() => null), text);
  if (!found) throw new Error(tl("{name} did not check: its answer is not the JSON the contract asks for.", { name: p.name }));
  return found;
}
