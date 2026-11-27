// The interface in German and Korean, page by page: no English left on
// the screen, no word built by gluing an English ending to a translated
// one ("Konflikts"), in the visible text and in the labels screen readers
// and tooltips read. Content (terms, segments, file names, people) is the
// user's and stays as it is: elements marked with another language, and
// words that are no interface word in English, are not looked at.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { run } from "./harness.mjs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const catalog = (lang) => JSON.parse(readFileSync(`${root}i18n/${lang}.json`, "utf8"));
const words = (s) => s.match(/[\p{L}][\p{L}'’-]*/gu) ?? [];

// The sample's terms are content of the termbase, in its languages: they
// stay as they are, wherever the interface shows them.
const sample = readFileSync(`${root}web/src/sample.ts`, "utf8");
const CONTENT = new Set(
  [...sample.matchAll(/\bt\(\s*"([^"]+)"/g)].flatMap((m) => words(m[1]).map((w) => w.toLowerCase())),
);
// Language names in their own language, as the language menu lists them.
const NATIVE = new Set(["english", "deutsch"]);

// Words that are the same in the language, or names: a German "Master"
// or "TMX" is German too.
function vocabulary(lang) {
  const cat = catalog(lang);
  const english = new Set();
  const translated = new Set();
  const whole = new Set();
  for (const [k, v] of Object.entries(cat)) {
    for (const w of words(k.replace(/\{[^}]*\}/g, " "))) if (w.length >= 3) english.add(w.toLowerCase());
    for (const w of words(v.replace(/\{[^}]*\}/g, " "))) translated.add(w.toLowerCase());
    if (/^[\p{L}-]+$/u.test(v)) whole.add(v.toLowerCase());
  }
  for (const w of translated) english.delete(w);
  return { english, translated, whole };
}

// Everything the page says: its text, outside content in another
// language, and what labels, tooltips and placeholders say.
const SAY = `(() => {
  const lang = document.documentElement.lang;
  const out = [];
  const content = (el) => { const c = el.closest('[lang]'); return c && c !== document.documentElement && !c.lang.startsWith(lang); };
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n = walk.nextNode(); n; n = walk.nextNode()) {
    const el = n.parentElement;
    if (!el || content(el) || el.closest('script,style,[contenteditable],textarea,code,.seg-source,.seg-target,.term-text')) continue;
    const s = getComputedStyle(el);
    if (s.display === 'none' || s.visibility === 'hidden') continue;
    const t = n.textContent.trim();
    if (t) out.push(t);
  }
  for (const el of document.querySelectorAll('[aria-label],[title],[placeholder]')) {
    if (content(el)) continue;
    for (const a of ['aria-label', 'title', 'placeholder']) { const v = el.getAttribute(a); if (v) out.push(v); }
  }
  return JSON.stringify(out);
})()`;

export default () =>
  run("The interface in German and Korean", async ({ evaluate, check, open, setValue, waitFor, sleep }) => {
    for (const [lang, marker] of [["de", "Sprache der Oberfläche"], ["ko", "인터페이스 언어"]]) {
      const { english, translated, whole } = vocabulary(lang);
      await open("/settings", { wait: 600 });
      await setValue("#ui-lang", lang);
      check(await waitFor(`document.documentElement.lang === ${JSON.stringify(lang)} && document.body.innerText.includes(${JSON.stringify(marker)})`, 15000), `${lang}: the page comes back in it`);
      // The sample as someone starting in this language gets it.
      await open("/projects", { wait: 1500 });
      check(await waitFor(`document.documentElement.lang === ${JSON.stringify(lang)}`, 15000), `${lang}: and stays in it with a fresh start`);
      const found = new Map();
      const look = async (where) => {
        const said = JSON.parse(await evaluate(SAY));
        for (const s of said) {
          for (const w of words(s)) {
            const l = w.toLowerCase();
            // Codes such as a Word field's PAGE are no words.
            if (CONTENT.has(l) || NATIVE.has(l) || (w.length > 1 && w === w.toUpperCase())) continue;
            let why = "";
            if (english.has(l)) why = "English";
            else if (l.length > 3 && /s$/.test(l) && !translated.has(l) && whole.has(l.slice(0, -1))) why = "glued ending";
            if (why && !found.has(`${why}: ${w} in “${s}”`)) found.set(`${why}: ${w} in “${s}”`, where);
          }
        }
      };
      // Every page, and every tab or filter on it.
      const routes = ["/projects", "/project/p-hofmann", "/editor/p-hofmann", "/merge/p-hofmann", "/memories", "/terminology", "/terminology/tb-weber", "/terminology/tb-p-weber-lease", "/accounts", "/settings", "/new"];
      for (const r of routes) {
        await open(r, { reload: false, wait: 900 });
        await look(r);
        const tabs = await evaluate("document.querySelectorAll('[role=tab], label.segment').length");
        for (let i = 0; i < tabs; i++) {
          await evaluate(`(() => { const t = document.querySelectorAll('[role=tab], label.segment')[${i}]; if (t) t.click(); })()`);
          await sleep(300);
          await look(`${r} (tab ${i + 1})`);
        }
      }
      const list = [...found].map(([k, where]) => `${k} (${where})`);
      check(list.length === 0, list.length ? `${lang}: nothing left in English or glued together:\n    ${list.join("\n    ")}` : `${lang}: nothing left in English or glued together`);
    }
    await open("/settings", { reload: false, wait: 600 });
    await setValue("#ui-lang", "");
    await waitFor("document.documentElement.lang === 'en'", 15000);
  });
