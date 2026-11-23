// The team server in the web app: who is signed in where, projects on the
// server opened into the store (with the memories and termbases they use),
// translations and new terms saved back as they are made, and what others
// change arriving through the live feed.
//
// A server project in the store is an ordinary Project with `remote` set,
// so the editor works on it as on any other; the store leaves it out of
// what the browser saves.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useStore, type SegmentsAt } from "./store";
import {
  ApiError,
  OfflineError,
  client,
  exchangeCode,
  follow,
  providerResult,
  providerSignInURL,
  serverInfo,
  setUpServer,
  signInWithPassword,
  type Assignment,
  type Client,
  type Connection,
  type CoreSettings,
  type FeedEvent,
  type Person,
  type Presence,
  type ServerInfo,
  type ServerProject,
  type ServerProjectSummary,
  type StoredSegment,
} from "./server";
import {
  DEFAULT_PREPARE,
  DEFAULT_QA,
  type Entry,
  type LangCode,
  type MemorySetting,
  type Project,
  type ProjectFile,
  type QaRule,
  type Termbase,
  type TranslationMemory,
} from "./data";
import { coreToRuns, runsToCore, type Segment } from "./editor/model";
import { core, extractDocx, extractXliff, loadCore } from "./core";
import { openRemoteMemory } from "./editor/memory";
import { fromCoreEntry, infoTypesFromCore, toCoreEntry } from "./termio";
import { go } from "./App";

const KEY = "interlinea.server";
const PENDING = "interlinea.server.pending";

function loadConnection(): Connection | null {
  try {
    const c = JSON.parse(localStorage.getItem(KEY) ?? "null") as Connection | null;
    if (c && new Date(c.expiresAt).getTime() > Date.now()) return c;
  } catch {
    // Storage is off or the value is not ours.
  }
  return null;
}

function keepConnection(c: Connection | null) {
  try {
    if (c) localStorage.setItem(KEY, JSON.stringify(c));
    else localStorage.removeItem(KEY);
  } catch {
    // Without storage the sign-in lasts until the page closes.
  }
}

/** The store's ID of a server project. */
export const remoteId = (n: number) => `srv-${n}`;
/** The server's number of a store project ID, or null for a local one. */
export function serverNumber(id: string): number | null {
  const m = /^srv-(\d+)$/.exec(id);
  return m ? Number(m[1]) : null;
}

/** The name a server memory goes by in the browser, apart from local memories. */
export const remoteMemoryName = (name: string) => `${name} (team server)`;
const termbaseId = (name: string) => `srv:${name}`;

export function errorText(e: unknown): string {
  if (e instanceof ApiError || e instanceof OfflineError) return e.message;
  return e instanceof Error ? e.message : String(e);
}

/**
 * The assignments of a project that cover a segment of a file in a
 * language, as the server decides it: an assignment without a file is
 * for every file, without a language for every language, without a range
 * for the whole file.
 */
export function covering(
  work: Assignment[],
  projectId: number,
  fileName: string,
  lang: LangCode,
  order: string[],
): (segId: string) => Assignment[] {
  const here = work.filter(
    (a) =>
      a.projectId === projectId &&
      (!a.file || a.file.toLowerCase() === fileName.toLowerCase()) &&
      (!a.targetLang || a.targetLang.toLowerCase() === lang.toLowerCase()),
  );
  return (segId) => {
    const pos = order.indexOf(segId);
    return here.filter((a) => {
      if (!a.fromSegment) return true;
      return pos >= order.indexOf(a.fromSegment) && pos <= order.indexOf(a.toSegment ?? "");
    });
  };
}

/** Whether a role lets someone change every segment of a project. */
export const editsAll = (role: string) => ["edit", "approve", "manage"].includes(role);

interface Remote {
  conn: Connection | null;
  info: ServerInfo | null;
  api: Client | null;
  /** The live feed is connected. */
  live: boolean;
  /** Translations not saved on the server yet. */
  unsaved: number;
  projects: ServerProjectSummary[] | null;
  /** The signed-in person's work, in every project. */
  work: Assignment[];
  people: Person[];
  error: string;
  check(url: string): Promise<ServerInfo>;
  signIn(url: string, username: string, password: string): Promise<void>;
  setup(url: string, a: Parameters<typeof setUpServer>[1]): Promise<void>;
  signInWithProvider(url: string): Promise<void>;
  signOut(): Promise<void>;
  refresh(): Promise<void>;
  /** Opens a server project into the store; resolves to its store ID. */
  open(n: number): Promise<string>;
  /** Opens a server termbase with its entries into the store; resolves to its store ID. */
  openTermbase(name: string): Promise<string>;
  /** Subscribes to the live feed; returns the function that unsubscribes. */
  listen(on: (e: FeedEvent) => void): () => void;
}

const Ctx = createContext<Remote | null>(null);

export function useRemote(): Remote {
  const r = useContext(Ctx);
  if (!r) throw new Error("useRemote outside RemoteProvider");
  return r;
}

/** The store's QA settings from the core's. */
export function qaOf(q: CoreSettings["qa"]) {
  return {
    ...DEFAULT_QA,
    disabled: q.disabled ?? [],
    enabled: q.enabled ?? [],
    uneditedFuzzy: q.uneditedFuzzy,
    rules: (q.rules ?? []) as QaRule[],
    dictionaries: q.dictionaries ?? {},
    words: q.words ?? [],
  };
}

function segmentOf(s: StoredSegment, local?: Segment): Segment {
  return {
    ...(local ?? {}),
    rev: (local?.rev ?? 0) + 1,
    id: s.id,
    source: coreToRuns(s.source ?? []),
    target: coreToRuns(s.target ?? []),
    status: s.status,
    locked: s.locked || undefined,
    lockedReason: undefined,
  };
}

/** An entry of a server termbase in the store. */
function entryOf(
  e: { id?: string; version: number; changedAt?: string; changedBy?: string; createdAt?: string; createdBy?: string } & Parameters<typeof fromCoreEntry>[0],
  tbId: string,
  ids: Map<string, string>,
  fallbackAt: string,
): Entry {
  const eid = `${tbId}:${e.id}`;
  return {
    id: eid,
    termbaseId: tbId,
    version: e.version,
    ...fromCoreEntry(e, ids, "approved"),
    history: [
      {
        id: `h-${eid}`,
        at: e.changedAt ?? e.createdAt ?? fallbackAt,
        by: e.changedBy ?? e.createdBy ?? "",
        summary: "On the team server",
      },
    ],
  };
}

/** Extension of a file name, lower case. */
const ext = (name: string) => /\.([^./]+)$/.exec(name)?.[1]?.toLowerCase() ?? "";

export function RemoteProvider({ children }: { children: ReactNode }) {
  const { state, dispatch, notify } = useStore();
  const [conn, setConn] = useState<Connection | null>(loadConnection);
  const [info, setInfo] = useState<ServerInfo | null>(null);
  const [live, setLive] = useState(false);
  const [projects, setProjects] = useState<ServerProjectSummary[] | null>(null);
  const [work, setWork] = useState<Assignment[]>([]);
  const [people, setPeople] = useState<Person[]>([]);
  const [error, setError] = useState("");
  const [unsaved, setUnsaved] = useState(0);
  const api = useMemo(() => (conn ? client(conn) : null), [conn]);
  const latest = useRef(state);
  latest.current = state;
  const listeners = useRef(new Set<(e: FeedEvent) => void>());

  // Segment versions as the server last gave them, and the segment objects
  // known to be on the server: a segment object not in `known` was changed
  // here and is saved.
  const versions = useRef(new Map<string, number>());
  const known = useRef(new WeakSet<Segment>());
  // Entries as the server has them: the objects (one not among them was
  // changed here) and, per termbase, the IDs (one missing was deleted here).
  const knownEntries = useRef(new WeakSet<Entry>());
  const serverEntries = useRef(new Map<string, Set<string>>());
  const sendingEntries = useRef(new Set<string>());
  const pending = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const failing = useRef(new Set<string>());
  // What the server has of each segment, to send only real changes.
  const sent = useRef(new Map<string, string>());
  const content = (sg: Segment) => JSON.stringify([runsToCore(sg.target), sg.status]);

  const use = useCallback((c: Connection | null) => {
    keepConnection(c);
    setConn(c);
    if (!c) {
      setProjects(null);
      setWork([]);
      setPeople([]);
      dispatch({ type: "remoteUnload" });
    }
  }, [dispatch]);

  const refresh = useCallback(async () => {
    if (!api) return;
    try {
      const [ps, ws, pp] = await Promise.all([api.projects(), api.myAssignments(), api.people()]);
      setProjects(ps);
      setWork(ws);
      setPeople(pp);
      setError("");
    } catch (e) {
      if (e instanceof ApiError && e.code === "signIn") {
        use(null);
        notify("The team server sign-in has ended. Sign in again in Settings.");
        return;
      }
      setError(errorText(e));
    }
  }, [api, use, notify]);

  // A provider sign-in comes back here with a code in the address.
  useEffect(() => {
    const r = providerResult(window.location.hash);
    if (!r) return;
    let url = "";
    try {
      url = sessionStorage.getItem(PENDING) ?? "";
      sessionStorage.removeItem(PENDING);
    } catch {
      // No session storage: the sign-in can't finish.
    }
    go({ page: "settings" });
    if (r.error) {
      notify(r.error);
      return;
    }
    if (r.code && url)
      void exchangeCode(url, r.code)
        .then((c) => {
          use(c);
          notify(`Signed in to ${new URL(url).host} as ${c.user.name}`);
        })
        .catch((e) => notify(errorText(e)));
  }, [use, notify]);

  useEffect(() => {
    if (!conn) {
      setInfo(null);
      return;
    }
    void serverInfo(conn.url).then(setInfo, (e) => setError(errorText(e)));
    void refresh();
  }, [conn, refresh]);

  // ---- Opening a server project ------------------------------------------

  const vkey = (n: number, file: string, lang: string, seg: string) => `${n}|${file}|${lang}|${seg}`;

  const materialize = useCallback(
    async (p: ServerProject, ws: Assignment[]): Promise<void> => {
      if (!api) return;
      const c = core() ?? (await loadCore());
      const s = p.settings;
      const id = remoteId(p.id);
      const team = await api.assignments(p.id).catch(() => [] as Assignment[]);
      const old = latest.current.projects.find((x) => x.id === id);
      const files: ProjectFile[] = [];
      const fileNames: Record<string, string> = {};
      for (const [i, f] of s.files.entries()) {
        const fid = `f${i + 1}`;
        fileNames[fid] = f.name;
        const before = old?.files.find((x) => x.name === f.name);
        const kind: ProjectFile["kind"] =
          f.role === "reference"
            ? "none"
            : ext(f.name) === "docx"
              ? "docx"
              : /^(xlf|xliff|sdlxliff|mqxliff)$/.test(ext(f.name))
                ? "xliff"
                : "text";
        const bytes = before?.bytes ?? (await api.fileBytes(p.id, f.name));
        const file: ProjectFile = { id: fid, name: f.name, role: f.role, words: 0, kind, bytes, segments: {} };
        if (f.role === "translatable") {
          // The file read here gives what the server doesn't keep: the
          // paragraphs for the preview, limits and notes of bilingual
          // files. The server's segments are the translation.
          let local = new Map<string, Segment>();
          if (!before) {
            try {
              if (kind === "docx") {
                const ex = await extractDocx(bytes, s.sourceLang);
                file.blocks = ex.blocks;
                local = new Map(ex.segments.map((x) => [x.id, x]));
              } else if (kind === "xliff") {
                const ex = await extractXliff(bytes);
                file.blocks = ex.blocks;
                local = new Map(ex.segments.map((x) => [x.id, x]));
              }
            } catch {
              // Read what the server has; the preview is missing.
            }
          } else {
            file.blocks = before.blocks;
            for (const segs of Object.values(before.segments)) for (const x of segs) local.set(x.id, x);
          }
          const langs = f.targetLang ? [f.targetLang] : s.targetLangs;
          for (const lang of langs) {
            const stored = await api.segments(p.id, f.name, lang);
            const order = stored.map((x) => x.id);
            const mine = covering(ws, p.id, f.name, lang, order);
            const others = covering(team, p.id, f.name, lang, order);
            file.segments[lang] = stored.map((x) => {
              versions.current.set(vkey(p.id, f.name, lang, x.id), x.version);
              const seg = segmentOf(x, local.get(x.id));
              sent.current.set(vkey(p.id, f.name, lang, x.id), content(seg));
              if (!seg.locked && !editsAll(p.access.role) && mine(x.id).length === 0) {
                const holder = others(x.id).find((a) => a.task === "translate") ?? others(x.id)[0];
                seg.locked = true;
                seg.lockedReason = holder
                  ? `${holder.task === "translate" ? "Translated" : "Reviewed"} by ${holder.userName}`
                  : "Not assigned to you";
              }
              known.current.add(seg);
              return seg;
            });
          }
          file.words = Object.values(file.segments)[0]?.reduce(
            (n, x) => n + x.source.filter((r) => typeof r === "string").join("").split(/\s+/).filter(Boolean).length,
            0,
          ) ?? 0;
        }
        files.push(file);
      }

      // Memories: the ones the project looks up in come whole from the
      // server; the ones it only updates are updated by the server.
      const memories: TranslationMemory[] = [];
      const settingsMem: Record<LangCode, MemorySetting[]> = {};
      for (const [lang, ms] of Object.entries(s.memories)) {
        settingsMem[lang] = ms.map((m) => ({ ...m, memory: remoteMemoryName(m.memory) }));
        for (const m of ms) {
          const name = remoteMemoryName(m.memory);
          if (memories.some((x) => x.name === name)) continue;
          const tm: TranslationMemory = {
            name,
            sourceLang: s.sourceLang,
            targetLang: lang,
            client: s.client,
            createdAt: p.createdAt,
            remote: true,
            projectId: m.memory.endsWith(`(${lang})`) && m.memory.startsWith(s.name) ? id : undefined,
          };
          memories.push(tm);
          // Only the units that match the project's segments come: matches
          // and the analysis need no more; concordance asks the server.
          if (m.lookup || m.concordance) {
            try {
              openRemoteMemory(c, tm, await api.memoryExtract(p.id, m.memory, lang));
            } catch {
              openRemoteMemory(c, tm, null);
            }
          } else openRemoteMemory(c, tm, null);
        }
      }

      // Terminology on the server: the project termbase with the masters'
      // entries layered under it, or the masters used directly.
      const termbases: Termbase[] = [];
      const entries: Entry[] = [];
      const t = s.terminology;
      if (t.location === "server" || t.mode === "direct") {
        let views: Awaited<ReturnType<Client["terminology"]>> = [];
        try {
          views = await api.terminology(p.id);
        } catch {
          views = [];
        }
        const host = conn ? new URL(conn.url).host : "";
        for (const v of views) {
          const infoTypes = infoTypesFromCore(v.definition.fields as never);
          const ids = new Map(infoTypes.map((i) => [i.name, i.id]));
          const add = (name: string, kind: "master" | "project") => {
            if (termbases.some((x) => x.id === termbaseId(name))) return;
            termbases.push({
              id: termbaseId(name),
              name,
              kind,
              location: "server",
              server: host,
              languages: v.definition.languages?.length ? v.definition.languages : [s.sourceLang, ...s.targetLangs],
              client: s.client,
              basedOn: kind === "project" ? (v.upstreams ?? []).map(termbaseId) : undefined,
              projectId: kind === "project" ? id : undefined,
              infoTypes,
              remote: true,
            });
          };
          add(v.name, v.kind === "project" ? "project" : "master");
          for (const e of v.entries) {
            add(e.termbase, e.termbase === v.name ? (v.kind === "project" ? "project" : "master") : "master");
            const eid = `${termbaseId(e.termbase)}:${e.id}`;
            if (entries.some((x) => x.id === eid)) continue;
            entries.push(entryOf(e, termbaseId(e.termbase), ids, p.createdAt));
          }
        }
      }
      remember(termbases, entries);

      const project: Project = {
        id,
        name: s.name,
        client: s.client || undefined,
        description: s.description || undefined,
        sourceLang: s.sourceLang,
        targetLangs: s.targetLangs,
        files,
        memories: settingsMem,
        projectMemory: s.projectMemory,
        setup: {
          mode: t.mode,
          masters: t.masters.map(termbaseId),
          writeTo: t.writeTo ? termbaseId(t.writeTo) : undefined,
          projectTermbaseId: t.termbase && t.location === "server" ? termbaseId(t.termbase) : undefined,
          location: "server",
          // The server layers the masters; nothing to take in here.
          autoUpdate: false,
        },
        context: s.analysis.context,
        qa: qaOf(s.qa),
        prepare: { ...DEFAULT_PREPARE, ...s.prepare },
        createdAt: p.createdAt,
        due: s.due || undefined,
        progress: 0,
        remote: { id: p.id, version: p.version, role: p.access.role, assigned: p.access.assigned, files: fileNames },
      };
      dispatch({ type: "remoteLoad", project, memories, termbases, entries });
    },
    [api, conn, dispatch],
  );

  /** Marks entries loaded from the server as being there. */
  const remember = useCallback((termbases: Termbase[], entries: Entry[]) => {
    for (const t of termbases) serverEntries.current.set(t.id, new Set());
    for (const e of entries) {
      knownEntries.current.add(e);
      serverEntries.current.get(e.termbaseId)?.add(e.id);
    }
  }, []);

  const openTermbase = useCallback(
    async (name: string): Promise<string> => {
      if (!api || !conn) throw new Error("Sign in to the team server first.");
      const info = await api.termbase(name);
      const infoTypes = infoTypesFromCore(info.definition.fields as never);
      const ids = new Map(infoTypes.map((i) => [i.name, i.id]));
      // A layered termbase without languages of its own has its masters'.
      let languages = info.definition.languages ?? [];
      if (!languages.length)
        for (const u of info.upstreams ?? []) {
          const up = await api.termbase(u).catch(() => null);
          for (const l of up?.definition.languages ?? []) if (!languages.includes(l)) languages = [...languages, l];
        }
      const tb: Termbase = {
        id: termbaseId(info.name),
        name: info.name,
        kind: info.kind,
        location: "server",
        server: new URL(conn.url).host,
        languages,
        basedOn: info.kind === "project" ? (info.upstreams ?? []).map(termbaseId) : undefined,
        infoTypes,
        remote: true,
        opened: true,
      };
      const entries: Entry[] = [];
      for (let offset = 0; ; offset += 1000) {
        const page = await api.entries(info.name, offset, 1000);
        for (const e of page.entries) entries.push(entryOf(e, tb.id, ids, ""));
        if (offset + page.entries.length >= page.total || page.entries.length === 0) break;
      }
      remember([tb], entries);
      dispatch({ type: "remoteTermbase", termbase: tb, entries });
      return tb.id;
    },
    [api, conn, dispatch, remember],
  );

  const open = useCallback(
    async (n: number) => {
      if (!api) throw new Error("Sign in to the team server first.");
      const [p, ws] = await Promise.all([api.project(n), api.myAssignments()]);
      setWork(ws);
      await materialize(p, ws);
      return remoteId(n);
    },
    [api, materialize],
  );

  // ---- Saving changes --------------------------------------------------

  const push = useCallback(
    async (project: Project, fileId: string, lang: LangCode, segId: string) => {
      if (!api || !project.remote) return;
      const n = project.remote.id;
      const file = project.remote.files[fileId]!;
      const k = vkey(n, file, lang, segId);
      const now = latest.current.projects.find((p) => p.id === project.id);
      const seg = now?.files.find((f) => f.id === fileId)?.segments[lang]?.find((s) => s.id === segId);
      if (!seg || known.current.has(seg)) return;
      if (sent.current.get(k) === content(seg)) {
        known.current.add(seg);
        return;
      }
      try {
        const saved = await api.saveSegment(n, file, lang, segId, runsToCore(seg.target), seg.status, versions.current.get(k) ?? 1);
        versions.current.set(k, saved.version);
        sent.current.set(k, content(seg));
        known.current.add(seg);
        failing.current.delete(k);
      } catch (e) {
        const at: SegmentsAt = { projectId: project.id, fileId, lang };
        if (e instanceof ApiError && (e.code === "conflict" || e.status === 403 || e.code === "locked")) {
          // Someone else saved it, or it isn't ours: the server's version
          // replaces ours.
          failing.current.delete(k);
          const fresh = await api.segments(n, file, lang).catch(() => null);
          const theirs = fresh?.find((x) => x.id === segId);
          if (theirs) {
            versions.current.set(k, theirs.version);
            const s2 = segmentOf(theirs, seg);
            if (e.status === 403) {
              s2.locked = true;
              s2.lockedReason = "Not assigned to you";
            }
            sent.current.set(k, content(s2));
            known.current.add(s2);
            dispatch({ type: "remoteSegments", at, segments: [s2] });
          }
          notify(
            e.code === "conflict"
              ? `${theirs?.changedBy ?? "Someone"} changed this segment on the server first; their translation is shown`
              : e.message,
          );
        } else {
          // Offline or the server failed: try again.
          failing.current.add(k);
          const t = setTimeout(() => void push(project, fileId, lang, segId), 5000);
          pending.current.set(k, t);
        }
      } finally {
        setUnsaved(failing.current.size);
      }
    },
    [api, dispatch, notify],
  );

  // Changed segments of server projects are saved shortly after the change.
  useEffect(() => {
    for (const p of state.projects) {
      if (!p.remote) continue;
      for (const f of p.files) {
        for (const [lang, segs] of Object.entries(f.segments)) {
          for (const sg of segs) {
            if (known.current.has(sg)) continue;
            const k = vkey(p.remote.id, p.remote.files[f.id]!, lang, sg.id);
            clearTimeout(pending.current.get(k));
            pending.current.set(
              k,
              setTimeout(() => {
                pending.current.delete(k);
                void push(p, f.id, lang, sg.id);
              }, 600),
            );
          }
        }
      }
    }
  }, [state.projects, push]);

  // Entries of server termbases changed here go to the server: new ones
  // are added, changed ones saved from the version they were made from,
  // deleted ones deleted.
  useEffect(() => {
    if (!api) return;
    const nameOf = (tbId: string) => state.termbases.find((t) => t.id === tbId && t.remote)?.name;
    const serverId = (e: Entry) => e.id.slice(e.termbaseId.length + 1);
    const fail = (what: string, tbId: string, err: unknown) => {
      notify(`${what} was not saved on the server: ${errorText(err)}`);
      const name = nameOf(tbId);
      // The server's version replaces ours.
      if (name) void openTermbaseQuiet(name);
    };
    const openTermbaseQuiet = (name: string) => openTermbase(name).catch(() => undefined);
    for (const e of state.entries) {
      if (knownEntries.current.has(e) || sendingEntries.current.has(e.id)) continue;
      const name = nameOf(e.termbaseId);
      if (!name) continue;
      const tb = state.termbases.find((t) => t.id === e.termbaseId)!;
      const onServer = serverEntries.current.get(e.termbaseId)?.has(e.id);
      sendingEntries.current.add(e.id);
      const core = toCoreEntry(e, tb);
      const done = (saved: { id?: string; version: number }) => {
        sendingEntries.current.delete(e.id);
        const id = `${e.termbaseId}:${saved.id}`;
        serverEntries.current.get(e.termbaseId)?.add(id);
        const next = { ...e, id, version: saved.version };
        knownEntries.current.add(next);
        dispatch({ type: "remoteEntry", localId: e.id, entry: next });
      };
      if (onServer)
        void api.updateEntry(name, { ...core, id: serverId(e) }, e.version ?? 1).then(done, (err) => {
          sendingEntries.current.delete(e.id);
          fail("The entry", e.termbaseId, err);
        });
      else
        void api.addEntry(name, { ...core, id: undefined }).then(done, (err) => {
          sendingEntries.current.delete(e.id);
          fail("The new entry", e.termbaseId, err);
        });
    }
    // Deleted here: gone on the server too.
    for (const [tbId, ids] of serverEntries.current) {
      const name = nameOf(tbId);
      if (!name) continue;
      const here = new Set(state.entries.filter((e) => e.termbaseId === tbId).map((e) => e.id));
      for (const id of ids) {
        if (here.has(id)) continue;
        ids.delete(id);
        void api.deleteEntry(name, id.slice(tbId.length + 1)).catch((err) => fail("Deleting the entry", tbId, err));
      }
    }
  }, [api, state.entries, state.termbases, dispatch, notify, openTermbase]);

  // ---- What others change ----------------------------------------------

  useEffect(() => {
    if (!conn || !api) return;
    const reloads = new Map<string, ReturnType<typeof setTimeout>>();
    const later = (key: string, ms: number, fn: () => void) => {
      clearTimeout(reloads.get(key));
      reloads.set(key, setTimeout(fn, ms));
    };
    const stop = follow(
      conn,
      (e) => {
        for (const l of listeners.current) l(e);
        const st = latest.current;
        switch (e.type) {
          case "signedOut":
            use(null);
            notify("The team server signed you out.");
            return;
          case "reset":
            for (const p of st.projects) if (p.remote) void open(p.remote.id).catch(() => undefined);
            later("list", 300, () => void refresh());
            return;
          case "account":
            later("list", 300, () => void refresh());
            return;
          case "project": {
            later("list", 500, () => void refresh());
            const c = e.change;
            const p = st.projects.find((x) => x.remote?.id === c.projectId);
            if (!p?.remote) return;
            if (c.kind === "project-deleted") {
              dispatch({ type: "remoteUnload", projectId: p.id });
              notify(`${c.by} deleted the project “${c.project}” on the server`);
              return;
            }
            if (c.kind === "segment-changed" && c.file && c.targetLang && c.segment) {
              const k = vkey(p.remote.id, c.file, c.targetLang, c.segment);
              if ((versions.current.get(k) ?? 0) >= (c.version ?? 0)) return;
              const fileId = Object.entries(p.remote.files).find(([, n]) => n === c.file)?.[0];
              const lang = c.targetLang;
              if (!fileId) return;
              later(`seg|${k}`, 150, () => {
                void api.segments(p.remote!.id, c.file!, lang).then((fresh) => {
                  const now = latest.current.projects.find((x) => x.id === p.id);
                  const segs = now?.files.find((f) => f.id === fileId)?.segments[lang] ?? [];
                  const changed: Segment[] = [];
                  for (const x of fresh) {
                    const kk = vkey(p.remote!.id, c.file!, lang, x.id);
                    const mine = segs.find((s) => s.id === x.id);
                    if ((versions.current.get(kk) ?? 0) >= x.version) continue;
                    // A translation being typed here is kept; saving it
                    // then meets the newer version and says so.
                    if (mine && !known.current.has(mine)) continue;
                    versions.current.set(kk, x.version);
                    const s2 = segmentOf(x, mine);
                    sent.current.set(kk, content(s2));
                    if (mine?.lockedReason) {
                      s2.locked = true;
                      s2.lockedReason = mine.lockedReason;
                    }
                    known.current.add(s2);
                    changed.push(s2);
                  }
                  if (changed.length)
                    dispatch({ type: "remoteSegments", at: { projectId: p.id, fileId, lang }, segments: changed });
                });
              });
              return;
            }
            // Settings, files or work changed: open the project again.
            later(`project|${p.id}`, 300, () => void open(p.remote!.id).catch(() => undefined));
            return;
          }
          case "memory": {
            // Others' confirmed translations: load the memory again.
            for (const p of st.projects) {
              if (!p.remote) continue;
              const lang = Object.entries(p.memories).find(([, ms]) =>
                ms.some((m) => m.memory === remoteMemoryName(e.memory) && (m.lookup || m.concordance)),
              )?.[0];
              if (!lang) continue;
              later(`mem|${e.memory}`, 2000, () => {
                const c = core();
                const tm = latest.current.memories.find((m) => m.name === remoteMemoryName(e.memory));
                if (!c || !tm) return;
                void api.memoryExtract(p.remote!.id, e.memory, lang).then((b) => openRemoteMemory(c, tm, b), () => undefined);
              });
            }
            return;
          }
          case "termbase":
            for (const p of st.projects) {
              if (!p.remote) continue;
              const ids = [...p.setup.masters, p.setup.projectTermbaseId, p.setup.writeTo];
              if (ids.includes(termbaseId(e.termbase)))
                later(`project|${p.id}`, 1000, () => void open(p.remote!.id).catch(() => undefined));
            }
            return;
        }
      },
      setLive,
    );
    return () => {
      stop();
      for (const t of reloads.values()) clearTimeout(t);
      setLive(false);
    };
  }, [conn, api, use, notify, open, refresh, dispatch]);

  const signIn = useCallback(
    async (url: string, username: string, password: string) => use(await signInWithPassword(url, username, password)),
    [use],
  );
  const setup = useCallback(
    async (url: string, a: Parameters<typeof setUpServer>[1]) => use(await setUpServer(url, a)),
    [use],
  );
  const signInWithProvider = useCallback(async (url: string) => {
    const back = `${window.location.origin}${window.location.pathname}`;
    const to = await providerSignInURL(url, back);
    try {
      sessionStorage.setItem(PENDING, url);
    } catch {
      // The sign-in can't be finished without session storage.
    }
    window.location.assign(to);
  }, []);
  const signOut = useCallback(async () => {
    try {
      await api?.signOut();
    } catch {
      // Signed out here either way.
    }
    use(null);
  }, [api, use]);
  const listen = useCallback((on: (e: FeedEvent) => void) => {
    listeners.current.add(on);
    return () => {
      listeners.current.delete(on);
    };
  }, []);

  const value = useMemo<Remote>(
    () => ({
      conn,
      info,
      api,
      live,
      unsaved,
      projects,
      work,
      people,
      error,
      check: serverInfo,
      signIn,
      setup,
      signInWithProvider,
      signOut,
      refresh,
      open,
      openTermbase,
      listen,
    }),
    [conn, info, api, live, unsaved, projects, work, people, error, signIn, setup, signInWithProvider, signOut, refresh, open, openTermbase, listen],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

/**
 * Who else has a segment of this file and language open, and marks the
 * active segment as open by the signed-in person while it is.
 */
export function usePresence(project: Project | undefined, fileId: string, lang: LangCode, activeId: string): Presence[] {
  const { api, conn, listen } = useRemote();
  const [present, setPresent] = useState<Presence[]>([]);
  const n = project?.remote?.id;
  const file = n ? project!.remote!.files[fileId] : undefined;
  const load = useCallback(() => {
    if (!api || !n || !file) return;
    void api
      .presence(n)
      .then((ps) => setPresent(ps.filter((x) => x.userId !== conn?.user.id && x.file === file && x.targetLang.toLowerCase() === lang.toLowerCase())))
      .catch(() => undefined);
  }, [api, n, file, lang, conn?.user.id]);
  useEffect(() => {
    if (!api || !n || !file || !activeId) return;
    const touch = () => void api.touch(n, file, lang, activeId).catch(() => undefined);
    touch();
    // Marks last two minutes on the server: renewed well before.
    const t = setInterval(touch, 45000);
    return () => clearInterval(t);
  }, [api, n, file, lang, activeId]);
  useEffect(() => {
    if (!api || !n) return;
    load();
    const off = listen((e) => {
      if (e.type === "presence" && e.projectId === n) load();
    });
    const t = setInterval(load, 30000);
    return () => {
      off();
      clearInterval(t);
    };
  }, [api, n, load, listen]);
  // Closing the editor takes the mark away.
  useEffect(() => {
    if (!api || !n) return;
    return () => void api.leave(n).catch(() => undefined);
  }, [api, n]);
  return present;
}

/**
 * Concordance in a server project's memories, asked of the server (the
 * browser has only the units that match the project's segments). Null
 * for a project on this computer.
 */
export function useServerConcordance(
  project: Project | undefined,
  lang: LangCode,
  text: string,
  limit: number,
): { memory: string; unit: { source: Segment["source"]; target: Segment["target"]; createdBy?: string; createdAt?: string; attributes?: Record<string, string> }; side: "source" | "target" }[] | null {
  const { api } = useRemote();
  const n = project?.remote?.id;
  const [hits, setHits] = useState<ReturnType<typeof useServerConcordance>>(null);
  useEffect(() => {
    if (!api || !n) {
      setHits(null);
      return;
    }
    const q = text.trim();
    if (q.length < 2) {
      setHits([]);
      return;
    }
    let live = true;
    const t = setTimeout(() => {
      void Promise.all([api.projectConcordance(n, lang, q, false, limit), api.projectConcordance(n, lang, q, true, limit)]).then(
        ([src, tgt]) => {
          if (!live) return;
          const both = [
            ...src.map((h) => ({ ...h, side: "source" as const })),
            ...tgt.map((h) => ({ ...h, side: "target" as const })),
          ];
          setHits(
            both.slice(0, limit).map((h) => ({
              memory: remoteMemoryName(h.memory),
              side: h.side,
              unit: { ...h.unit, source: coreToRuns(h.unit.source ?? []), target: coreToRuns(h.unit.target ?? []) },
            })),
          );
        },
        () => live && setHits([]),
      );
    }, 250);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [api, n, lang, text, limit]);
  return n ? hits ?? [] : null;
}
