// The interface in other languages. Strings are written in English and
// looked up by their English text in the catalog of the interface
// language (i18n/de.json, i18n/ko.json, shared with the desktop app);
// `go run ./i18n/gen` lists new strings there and checks every one has a
// translation.
//
// tl("Saved {name}.", { name }) gives a string; tlx() the same with React
// nodes in the placeholders. The catalog is loaded before the app's
// modules, so strings made when a module loads are translated too.

import { Fragment, createElement, type ReactNode } from "react";

export type UiLanguage = "en" | "de" | "ko";

export const UI_LANGUAGES: { code: UiLanguage; name: string }[] = [
  { code: "en", name: "English" },
  { code: "de", name: "Deutsch" },
  { code: "ko", name: "한국어" },
];

const KEY = "interlinea.interface-language";

let catalog: Record<string, string> = {};
let current: UiLanguage = "en";

/** The language chosen in the settings; "" to follow the browser's. */
export function chosenLanguage(): UiLanguage | "" {
  try {
    const v = localStorage.getItem(KEY);
    return v === "en" || v === "de" || v === "ko" ? v : "";
  } catch {
    return "";
  }
}

/** The browser's language, where the interface has it. */
export function browserLanguage(): UiLanguage {
  for (const l of navigator.languages ?? [navigator.language]) {
    const base = l.toLowerCase().split("-")[0];
    if (base === "de" || base === "ko" || base === "en") return base;
  }
  return "en";
}

/** Saves the choice; the page is loaded again in that language. */
export function chooseLanguage(l: UiLanguage | "") {
  try {
    if (l) localStorage.setItem(KEY, l);
    else localStorage.removeItem(KEY);
  } catch {
    // Without storage the choice lasts until the page is closed.
  }
  location.reload();
}

/** The language the interface is shown in. */
export function uiLanguage(): UiLanguage {
  return current;
}

/** Loads the catalog of the interface language. */
export async function loadLanguage(): Promise<void> {
  const l = chosenLanguage() || browserLanguage();
  current = l;
  document.documentElement.lang = l;
  if (l === "de") catalog = (await import("../../i18n/de.json")).default;
  else if (l === "ko") catalog = (await import("../../i18n/ko.json")).default;
  else catalog = {};
}

/** Sets the catalog directly (tests). */
export function setCatalog(l: UiLanguage, c: Record<string, string>) {
  current = l;
  catalog = c;
  templates = null;
}

type Vars = Record<string, string | number | boolean | null | undefined>;

function fill(s: string, vars?: Vars): string {
  if (!vars) return s;
  return s.replace(/\{([A-Za-z0-9_]+)\}/g, (m, k: string) =>
    k in vars ? String(vars[k]) : m,
  );
}

/** The string in the interface language, its placeholders filled in. */
export function tl(s: string, vars?: Vars): string {
  return fill(catalog[s] || s, vars);
}

/** As tl, with React nodes (links, emphasis) in the placeholders. */
export function tlx(s: string, vars: Record<string, ReactNode>): ReactNode {
  const parts = (catalog[s] || s).split(/(\{[A-Za-z0-9_]+\})/);
  return createElement(
    Fragment,
    null,
    ...parts.map((p, i) => {
      const name = /^\{([A-Za-z0-9_]+)\}$/.exec(p)?.[1];
      const v = name !== undefined && name in vars ? vars[name] : p;
      return createElement(Fragment, { key: i }, v);
    }),
  );
}

/** The locale numbers and dates are written in. */
export function uiLocale(): string {
  return current === "de" ? "de-DE" : current === "ko" ? "ko-KR" : "en-GB";
}

// The core's texts (checks, analysis rows, tag descriptions, messages)
// arrive in English, made from texts it marks with placeholders, such as
// "Font {value}" filled to "Font Arial". They are recognized by those
// marked texts and translated with the filled parts kept.
let templates: { re: RegExp; names: string[]; quoted: boolean[]; key: string }[] | null = null;

function coreTemplates() {
  if (templates) return templates;
  templates = [];
  for (const key of Object.keys(catalog)) {
    const parts = key.split(/\{([A-Za-z0-9_]+)\}/);
    if (parts.length < 3) continue;
    const names: string[] = [];
    const quoted: boolean[] = [];
    let src = "^";
    parts.forEach((p, i) => {
      if (i % 2) {
        names.push(p);
        // Between quotes: words of the text, never translated.
        quoted.push(parts[i - 1]!.endsWith("“") || parts[i - 1]!.endsWith('"'));
        src += "(.+?)";
      } else src += p.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    });
    templates.push({ re: new RegExp(src + "$", "s"), names, quoted, key });
  }
  // The longest fixed text first: "Size {n} pt" before "{x}".
  templates.sort((a, b) => b.key.replace(/\{[^}]+\}/g, "").length - a.key.replace(/\{[^}]+\}/g, "").length);
  return templates;
}

// A number the core wrote with a full stop, written as the interface
// language writes numbers (3.5 is 3,5 in German).
function localNumber(v: string): string {
  if (!/^-?\d+\.\d+$/.test(v)) return v;
  const digits = v.split(".")[1]!.length;
  return new Intl.NumberFormat(uiLocale(), { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(Number(v));
}

/**
 * A text of the core in the interface language: as it is in the catalog,
 * or recognized by a marked text with placeholders; a list of such texts
 * joined with commas is translated part by part. What is not recognized
 * (a file's own markup, a name) stays as it is.
 */
export function tlCore(s: string): string {
  if (!s || current === "en") return s;
  const exact = catalog[s];
  if (exact) return exact;
  for (const t of coreTemplates()) {
    const m = t.re.exec(s);
    if (!m) continue;
    const vars: Record<string, string> = {};
    t.names.forEach((n, i) => {
      const v = m[i + 1]!;
      vars[n] = t.quoted[i] ? v : localNumber(tlCore(v));
    });
    return fill(catalog[t.key]!, vars);
  }
  if (s.includes(", ")) {
    const parts = s.split(", ");
    const done = parts.map(tlCore);
    if (done.some((p, i) => p !== parts[i])) return done.join(", ");
  }
  return s;
}

// Key names as the keyboards of the interface language print them:
// German keyboards say Strg, Umschalt, Einfg, Entf.
const KEYS: Partial<Record<UiLanguage, Record<string, string>>> = {
  de: { Ctrl: "Strg", Shift: "Umschalt", Ins: "Einfg", Insert: "Einfg", Del: "Entf", Delete: "Entf", Enter: "Eingabe", Space: "Leertaste", Esc: "Esc" },
};

/** A key or key combination ("Ctrl+Enter") with the interface language's key names. */
export function keyName(combo: string): string {
  const names = KEYS[current];
  if (!names) return combo;
  return combo
    .split("+")
    .map((k) => names[k] ?? k)
    .join("+");
}
