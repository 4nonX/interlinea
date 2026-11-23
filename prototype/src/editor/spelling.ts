// Spelling dictionaries for the core's spell checker (Hunspell in Go).
// A dictionary is added once, from a LibreOffice extension (.oxt), a ZIP
// file with .aff and .dic files, or an .aff and .dic pair, saved in the
// browser, and loaded into the core the first time a project needs it.

import { unwrap, type CoreApi, type SpellBundle } from "../core";
import { load, remove, save } from "../persist";
import type { LangCode, SpellDictionary } from "../data";
import { knownLang } from "../setup";

const SAVED = "dict:";
const loaded = new Set<string>();

interface Saved {
  aff: Uint8Array;
  dic: Uint8Array;
}

/** A dictionary found in the chosen files, not added yet. */
export interface FoundDictionary {
  name: string;
  lang: LangCode;
  aff: Uint8Array;
  dic: Uint8Array;
}

function langOfName(name: string): LangCode {
  const m = /([a-z]{2,3})(?:[_-]([A-Za-z]{2}))?/.exec(name);
  if (!m) return "";
  return knownLang(m[2] ? `${m[1]}-${m[2]}` : m[1]!);
}

const base = (f: string) => f.replace(/\.[^.]+$/, "");

/**
 * The dictionaries in the chosen files: every one in an extension or ZIP
 * file, and each .aff file with the .dic file of the same name. Files
 * that are neither are reported.
 */
export async function findDictionaries(
  c: CoreApi,
  files: File[],
): Promise<{ found: FoundDictionary[]; problems: string[] }> {
  const found: FoundDictionary[] = [];
  const problems: string[] = [];
  const affs = new Map<string, File>();
  const dics = new Map<string, File>();
  for (const f of files) {
    if (/\.(oxt|zip)$/i.test(f.name)) {
      let bundles: SpellBundle[];
      try {
        bundles = unwrap(c.spellArchive(new Uint8Array(await f.arrayBuffer())));
      } catch (e) {
        problems.push(
          `${f.name} could not be read: ${e instanceof Error ? e.message : String(e)}`,
        );
        continue;
      }
      if (!bundles.length)
        problems.push(`${f.name} has no spelling dictionary.`);
      for (const b of bundles)
        found.push({
          name: b.name,
          lang: b.locales[0] ? knownLang(b.locales[0]) : langOfName(b.name),
          aff: b.aff,
          dic: b.dic,
        });
    } else if (/\.aff$/i.test(f.name)) affs.set(base(f.name), f);
    else if (/\.dic$/i.test(f.name)) dics.set(base(f.name), f);
    else
      problems.push(
        `${f.name} is not a dictionary: choose a LibreOffice extension (.oxt), a ZIP file, or an .aff and a .dic file.`,
      );
  }
  for (const [name, aff] of affs) {
    const dic = dics.get(name);
    if (!dic) {
      problems.push(`${aff.name} needs ${name}.dic as well.`);
      continue;
    }
    dics.delete(name);
    found.push({
      name,
      lang: langOfName(name),
      aff: new Uint8Array(await aff.arrayBuffer()),
      dic: new Uint8Array(await dic.arrayBuffer()),
    });
  }
  for (const [name] of dics)
    problems.push(`${name}.dic needs ${name}.aff as well.`);
  return { found, problems };
}

/**
 * Loads a found dictionary into the core and saves its files. The core
 * reads it completely, so a broken dictionary is refused here.
 */
export async function addDictionary(
  c: CoreApi,
  d: FoundDictionary,
  lang: LangCode,
): Promise<{ dictionary: SpellDictionary; kept: boolean }> {
  const info = JSON.parse(unwrap(c.spellLoad(d.name, d.aff, d.dic, "[]"))) as {
    words: number;
  };
  loaded.add(d.name);
  const kept = await save(SAVED + d.name, {
    aff: d.aff,
    dic: d.dic,
  } satisfies Saved);
  return {
    dictionary: {
      name: d.name,
      lang,
      words: info.words,
      addedAt: new Date().toISOString(),
    },
    kept,
  };
}

/** Loads a saved dictionary into the core if it isn't yet; false if its files are gone. */
export async function ensureDictionary(
  c: CoreApi,
  name: string,
): Promise<boolean> {
  if (loaded.has(name)) return true;
  const files = await load<Saved>(SAVED + name);
  if (
    !files ||
    !(files.aff instanceof Uint8Array) ||
    !(files.dic instanceof Uint8Array)
  )
    return false;
  unwrap(c.spellLoad(name, files.aff, files.dic, "[]"));
  loaded.add(name);
  return true;
}

/** Whether the core has the dictionary loaded (ensureDictionary was awaited). */
export function dictionaryLoaded(name: string): boolean {
  return loaded.has(name);
}

/** Deletes a dictionary from the core and the browser. */
export async function dropDictionary(c: CoreApi, name: string): Promise<void> {
  unwrap(c.spellDrop(name));
  loaded.delete(name);
  await remove(SAVED + name);
}

/** The saved data was reset; dictionaries load again from nothing. */
export function forgetDictionaries() {
  loaded.clear();
}
