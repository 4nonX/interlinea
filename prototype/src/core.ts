// The Interlinea core (Go) compiled to WebAssembly: the same Word filter the
// desktop app and the server use. Loaded on first use; about 1.3 MB
// compressed. Build it with `make wasm` in the repository root.

import { coreToRuns, runsToCore, type Block, type CoreRun, type Segment } from "./editor/model";

export type { Block };

interface CoreExtraction {
  units: { id: string; block: string; source: CoreRun[] }[];
  blocks: Block[];
}

interface CoreApi {
  extract(bytes: Uint8Array, lang: string): { ok: true; value: string } | { ok: false; error: string };
  merge(bytes: Uint8Array, lang: string, target: string, translations: string): { ok: true; value: Uint8Array } | { ok: false; error: string };
}

declare global {
  interface Window {
    interlineaCore?: CoreApi;
    interlineaCoreReady?: () => void;
    Go?: new () => { importObject: WebAssembly.Imports; run(instance: WebAssembly.Instance): Promise<void> };
  }
}

let loading: Promise<CoreApi> | null = null;

function base(): string {
  return new URL("./", document.baseURI).href;
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`Could not load ${src}`));
    document.head.append(s);
  });
}

export function loadCore(): Promise<CoreApi> {
  if (window.interlineaCore) return Promise.resolve(window.interlineaCore);
  loading ??= (async () => {
    if (!window.Go) await loadScript(`${base()}core/wasm_exec.js`);
    const res = await fetch(`${base()}core/interlinea.wasm`);
    if (!res.ok) throw new Error(`The core is missing (${res.status}). Run "make wasm" in the repository.`);
    // instantiateStreaming needs the application/wasm type, which not every
    // host sends; compiling from the bytes works everywhere.
    const bytes = await res.arrayBuffer();
    const go = new window.Go!();
    const ready = new Promise<void>((resolve) => (window.interlineaCoreReady = resolve));
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    void go.run(instance);
    await ready;
    return window.interlineaCore!;
  })().catch((e) => {
    loading = null;
    throw e;
  });
  return loading;
}

export interface Extracted {
  segments: Segment[];
  blocks: Block[];
}

/** Extracts the units of a Word document as editor segments. */
export async function extractDocx(bytes: Uint8Array, lang: string): Promise<Extracted> {
  const core = await loadCore();
  const r = core.extract(bytes, lang);
  if (!r.ok) throw new Error(r.error);
  const ex = JSON.parse(r.value) as CoreExtraction;
  return {
    segments: ex.units.map((u) => ({ id: u.id, block: u.block, source: coreToRuns(u.source), target: [], status: "new" as const })),
    blocks: ex.blocks,
  };
}

/**
 * Writes the translated document. Segments without a translation keep their
 * source text, as in other CAT tools.
 */
export async function mergeDocx(bytes: Uint8Array, lang: string, target: string, segments: Segment[]): Promise<Uint8Array> {
  const core = await loadCore();
  const tr: Record<string, CoreRun[]> = {};
  for (const s of segments) {
    if (s.target.length && s.target.some((r) => typeof r !== "string" || r.trim())) tr[s.id] = runsToCore(s.target);
  }
  const r = core.merge(bytes, lang, target, JSON.stringify(tr));
  if (!r.ok) throw new Error(r.error);
  return r.value;
}

/** Offers bytes as a download. */
export function download(bytes: Uint8Array, name: string, type: string) {
  const url = URL.createObjectURL(new Blob([new Uint8Array(bytes)], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

export const DOCX_TYPE = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
