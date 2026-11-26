import { tl, uiLanguage, uiLocale } from "./i18n";

const dateFmt = new Intl.DateTimeFormat(uiLocale(), {
  day: "numeric",
  month: "short",
  year: "numeric",
});
const timeFmt = new Intl.DateTimeFormat(uiLocale(), {
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
});
const numFmt = new Intl.NumberFormat(uiLocale());

export const fmtDate = (iso: string) => dateFmt.format(new Date(iso));
export const fmtTime = (iso: string) => timeFmt.format(new Date(iso));
export const fmtNum = (n: number) => numFmt.format(n);
/** A count with its noun: plural(3, "file", "files") is "3 files". The
 * two forms are looked up as "{n} file" and "{n} files", so a language
 * can put the number where it goes. */
export const plural = (n: number, one: string, many: string) =>
  tl(n === 1 ? `{n} ${one}` : `{n} ${many}`, { n: fmtNum(n) });

/** A number with a fixed count of decimals, written as the interface
 * language writes it. */
export const fmtDec = (n: number, digits: number) =>
  new Intl.NumberFormat(uiLocale(), { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(n);

/** Items in a list: "A, B and C" in the interface language. */
export const listOf = (items: string[]) =>
  new Intl.ListFormat(uiLocale(), { type: "conjunction" }).format(items);

/** A word put inside a sentence: lower case, but not in German, where
 * nouns keep their capital. */
export const inSentence = (s: string) => (uiLanguage() === "de" ? s : s.toLowerCase());
