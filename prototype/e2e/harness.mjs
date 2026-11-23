// Browser tests for the prototype, driven through the Chrome DevTools
// Protocol with no extra packages. start() serves the production build
// (dist/) on a free port, launches headless Chrome and returns helpers.
//
// Chrome is found through $CHROME, else the usual names on the PATH.

import { spawn, spawnSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdtempSync, readFileSync, existsSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, extname, dirname, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const fixtures = join(here, "fixtures");
export const samples = join(here, "..", "samples");
const dist = join(here, "..", "dist");

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const TYPES = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".css": "text/css",
  ".wasm": "application/wasm",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".woff2": "font/woff2",
};

function serve() {
  if (!existsSync(join(dist, "index.html")))
    throw new Error(
      "dist/ is missing: run `make wasm` and `npm run build` first",
    );
  const server = createServer((req, res) => {
    const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
    let file = normalize(join(dist, path));
    if (!file.startsWith(dist)) {
      res.writeHead(403).end();
      return;
    }
    if (!existsSync(file) || statSync(file).isDirectory())
      file = join(dist, "index.html");
    res.writeHead(200, {
      "Content-Type": TYPES[extname(file)] ?? "application/octet-stream",
    });
    res.end(readFileSync(file));
  });
  return new Promise((resolve) =>
    server.listen(0, "127.0.0.1", () => resolve(server)),
  );
}

function findChrome() {
  if (process.env.CHROME) return process.env.CHROME;
  for (const name of [
    "google-chrome",
    "google-chrome-stable",
    "chromium",
    "chromium-browser",
    "chrome",
  ]) {
    if (spawnSync(name, ["--version"], { stdio: "ignore" }).status === 0)
      return name;
  }
  throw new Error("Chrome not found: set CHROME to its path");
}

export async function start() {
  const server = await serve();
  const base = `http://127.0.0.1:${server.address().port}`;
  const port = 9300 + Math.floor(Math.random() * 600);
  const profile = mkdtempSync(join(tmpdir(), "interlinea-e2e-"));
  const downloads = mkdtempSync(join(tmpdir(), "interlinea-dl-"));
  const chrome = spawn(
    findChrome(),
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-gpu",
      `--remote-debugging-port=${port}`,
      `--user-data-dir=${profile}`,
      "--window-size=1440,1000",
      "about:blank",
    ],
    { stdio: "ignore" },
  );

  let ws;
  let id = 0;
  const pending = new Map();
  const errors = [];
  let failed = 0;
  let passed = 0;

  // A protocol call that gets no answer in 30 seconds fails the test
  // instead of hanging it.
  const send = (method, params = {}) =>
    new Promise((res, rej) => {
      const i = ++id;
      const timer = setTimeout(() => {
        pending.delete(i);
        rej(new Error(`${method} got no answer in 30 s`));
      }, 30000);
      pending.set(i, {
        res: (v) => {
          clearTimeout(timer);
          res(v);
        },
        rej,
      });
      ws.send(JSON.stringify({ id: i, method, params }));
    });
  const evaluate = async (expr) => {
    const r = await send("Runtime.evaluate", {
      expression: expr,
      awaitPromise: true,
      returnByValue: true,
    });
    if (r.exceptionDetails)
      throw new Error(
        r.exceptionDetails.exception?.description ??
          JSON.stringify(r.exceptionDetails),
      );
    return r.result.value;
  };
  const click = (sel, txt) =>
    evaluate(
      `(() => { const els = [...document.querySelectorAll(${JSON.stringify(sel)})]; const el = ${txt ? `els.find(e => e.textContent.includes(${JSON.stringify(txt)}))` : "els[0]"}; if (!el) throw new Error(${JSON.stringify(`not found: ${sel} ${txt ?? ""}`)}); el.click(); return true; })()`,
    );
  const text = () => evaluate("document.querySelector('main').innerText");
  const body = () => evaluate("document.body.innerText");
  const dlg = () =>
    evaluate("document.querySelector('dialog[open]')?.innerText ?? ''");
  const check = (cond, msg) => {
    if (!cond) {
      failed++;
      console.log("FAIL", msg);
    } else {
      passed++;
      console.log("ok  ", msg);
    }
  };
  const setFile = async (sel, path) => {
    if (!existsSync(path)) throw new Error(`no such file: ${path}`);
    const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
    const q = await send("DOM.querySelector", {
      nodeId: doc.root.nodeId,
      selector: sel,
    });
    if (!q.nodeId) throw new Error(`not found: ${sel}`);
    await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files: [path] });
    await sleep(500);
  };
  const setValue = (sel, v) =>
    evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(sel)}); if (!el) throw new Error(${JSON.stringify(`not found: ${sel}`)}); const proto = el.tagName === "SELECT" ? HTMLSelectElement.prototype : el.tagName === "TEXTAREA" ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, "value").set.call(el, ${JSON.stringify(v)}); el.dispatchEvent(new Event(el.tagName === "SELECT" ? "change" : "input", { bubbles: true })); return true; })()`,
    );
  const key = async (k, code, vk, modifiers = 0) => {
    await send("Input.dispatchKeyEvent", {
      type: "keyDown",
      key: k,
      code,
      windowsVirtualKeyCode: vk,
      modifiers,
    });
    await send("Input.dispatchKeyEvent", {
      type: "keyUp",
      key: k,
      code,
      windowsVirtualKeyCode: vk,
      modifiers,
    });
    await sleep(250);
  };
  /**
   * Opens a route on a fresh page load, starting from the sample data
   * (the prototype saves work between visits). keep: true reloads without
   * resetting, to see what was saved; reload: false changes the route
   * within the current page.
   */
  let loads = 0;
  const open = async (
    route,
    { keep = false, wait = 800, reload = true } = {},
  ) => {
    if (!reload) {
      await evaluate(`location.hash = ${JSON.stringify(route)}`);
      await sleep(wait);
      return;
    }
    const q = new URLSearchParams({ load: String(++loads) });
    if (!keep) q.set("reset", "1");
    await send("Page.navigate", { url: `${base}/?${q}#${route}` });
    await sleep(wait);
  };
  /** Waits until a script returns a truthy value. */
  const waitFor = async (expr, ms = 8000) => {
    for (let t = 0; t < ms; t += 100) {
      if (await evaluate(expr).catch(() => false)) return true;
      await sleep(100);
    }
    return false;
  };

  for (let i = 0; i < 100; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
      const page = list.find((t) => t.type === "page");
      if (page) {
        ws = new WebSocket(page.webSocketDebuggerUrl);
        break;
      }
    } catch {
      // Chrome is still starting.
    }
    await sleep(100);
  }
  if (!ws) throw new Error("Chrome did not start");
  await new Promise((r) => (ws.onopen = r));
  ws.addEventListener("message", (m) => {
    const d = JSON.parse(m.data);
    if (d.id && pending.has(d.id)) {
      pending.get(d.id).res(d.result ?? {});
      pending.delete(d.id);
    }
    if (d.method === "Runtime.exceptionThrown")
      errors.push(
        d.params.exceptionDetails.exception?.description ??
          d.params.exceptionDetails.text,
      );
    if (d.method === "Runtime.consoleAPICalled" && d.params.type === "error")
      errors.push(d.params.args.map((a) => a.value ?? a.description).join(" "));
  });
  await send("Runtime.enable");
  await send("Page.enable");
  await send("Page.setDownloadBehavior", {
    behavior: "allow",
    downloadPath: downloads,
  });

  const finish = () => {
    chrome.kill();
    server.close();
    return { passed, failed };
  };
  return {
    base,
    send,
    evaluate,
    click,
    text,
    body,
    dlg,
    check,
    setFile,
    setValue,
    key,
    open,
    waitFor,
    sleep,
    errors,
    downloads,
    fixtures,
    samples,
    finish,
  };
}

/** Runs a test function with a fresh browser and reports its result. */
export async function run(name, fn) {
  const h = await start();
  console.log(`\n# ${name}`);
  try {
    await fn(h);
    h.check(
      h.errors.length === 0,
      `no console errors: ${h.errors.join(" | ")}`,
    );
  } catch (e) {
    h.check(false, `${name} stopped: ${e.message}`);
  }
  return h.finish();
}
