import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  clearAll,
  load,
  save,
  STATE_VERSION,
  takeResetRequest,
} from "./persist";
import { forgetMemories, preloadMemories } from "./editor/memory";
import {
  BASE_SNAPSHOTS,
  BRANCHED_AT,
  DEFAULT_INFO_TYPES,
  INITIAL_ENTRIES,
  INITIAL_PROJECTS,
  INITIAL_TERMBASES,
  USER,
  type Commit,
  type Entry,
  type InfoType,
  type LangCode,
  type Location,
  type Project,
  type SetupMode,
  type Termbase,
} from "./data";
import { applyMerge, type Change, type Decision } from "./merge";
import {
  confirmedShare,
  normalize,
  segmentText,
  sourceKey,
  type Block,
  type Run,
  type Segment,
  type SegmentStatus,
} from "./editor/model";

function withSegments(
  state: State,
  projectId: string,
  f: (segments: Segment[]) => Segment[],
): State {
  return {
    ...state,
    projects: state.projects.map((p) => {
      if (p.id !== projectId || !p.segments) return p;
      const segments = f(p.segments);
      return { ...p, segments, progress: confirmedShare(segments) };
    }),
  };
}

export interface State {
  termbases: Termbase[];
  entries: Entry[];
  projects: Project[];
  baseSnapshots: Record<string, Entry>;
  branchedAt: Record<string, string>;
  gitWords: boolean;
  /** Editor: put the best memory match into an empty segment when it opens (Trados does this by default). */
  autoInsert: boolean;
  /** Editor: confirming fills identical segments further on (auto-propagation). */
  autoPropagate: boolean;
  /** Context matches: the segment before must match (Trados) or before and after (memoQ). */
  contextRule: "previous" | "both";
  toast: { id: number; text: string } | null;
}

export interface NewProjectInput {
  name: string;
  file: string;
  words: number;
  sourceLang: LangCode;
  targetLangs: LangCode[];
  client?: string;
  mode: SetupMode;
  masters: string[];
  location: Location;
  /** Document text when the prototype could read it; becomes the segments. */
  text: string | null;
  /** Units, structure and bytes from the core's file filter; used instead of text. */
  extracted?: {
    segments: Segment[];
    blocks: Block[];
    bytes: Uint8Array;
    format: "docx" | "xliff";
    /** The Trados project package the file came from. */
    sdlPackage?: { bytes: Uint8Array; name: string; path: string };
  };
}

/** The result of an import the user confirmed; worked out with the core's plan. */
export interface ImportInput {
  termbaseId: string;
  /** Set when the import creates the termbase. */
  create?: Termbase;
  languages: LangCode[];
  infoTypes: InfoType[];
  adds: Entry[];
  updates: {
    entryId: string;
    content: Pick<Entry, "concept" | "terms" | "langFields">;
    summary: string;
  }[];
  now: string;
}

export interface NewTermInput {
  projectId: string;
  source: string;
  target: string;
  note?: string;
  segment: string;
}

type Action =
  | { type: "hydrate"; state: SavedState }
  | { type: "resetAll" }
  | { type: "createProject"; input: NewProjectInput; id: string; now: string }
  | {
      type: "merge";
      projectId: string;
      masterId: string;
      changes: Change[];
      decisions: Record<string, Decision>;
      now: string;
    }
  | { type: "updateFromMaster"; projectId: string; now: string }
  | { type: "addInfoType"; termbaseId: string; infoType: InfoType }
  | { type: "createMaster"; termbase: Termbase }
  | {
      type: "setField";
      entryId: string;
      field: string;
      value: string;
      now: string;
    }
  | {
      type: "setTermField";
      entryId: string;
      langCode: LangCode;
      termIndex: number;
      field: string;
      value: string;
      now: string;
    }
  | { type: "setGitWords"; value: boolean }
  | {
      type: "setEditorOption";
      option: "autoInsert" | "autoPropagate";
      value: boolean;
    }
  | { type: "setContextRule"; value: "previous" | "both" }
  | {
      type: "confirm";
      projectId: string;
      segmentId: string;
      warnings: { kind: string; text: string }[];
    }
  | { type: "ignoreIssue"; projectId: string; segmentId: string; text: string }
  | {
      type: "setTarget";
      projectId: string;
      segmentId: string;
      target: Run[];
      status: SegmentStatus;
      origin?: Segment["origin"];
    }
  | { type: "addTerm"; input: NewTermInput; id: string; now: string }
  | {
      type: "updateTermbase";
      termbaseId: string;
      patch: Partial<Pick<Termbase, "name" | "languages" | "infoTypes">>;
    }
  | {
      type: "removeLanguage";
      termbaseId: string;
      langCode: LangCode;
      now: string;
    }
  | {
      type: "removeInfoType";
      termbaseId: string;
      infoTypeId: string;
      now: string;
    }
  | { type: "deleteTermbase"; termbaseId: string }
  | { type: "createEntry"; entry: Entry }
  | {
      type: "saveEntry";
      entryId: string;
      content: Pick<Entry, "concept" | "terms" | "langFields">;
      summary: string;
      now: string;
    }
  | { type: "deleteEntries"; entryIds: string[] }
  | {
      type: "mergeEntries";
      keepId: string;
      removeIds: string[];
      content: Pick<Entry, "concept" | "terms" | "langFields">;
      summary: string;
      now: string;
    }
  | { type: "importEntries"; input: ImportInput }
  | { type: "toast"; text: string }
  | { type: "clearToast" };

const initial: State = {
  termbases: INITIAL_TERMBASES,
  entries: INITIAL_ENTRIES,
  projects: INITIAL_PROJECTS,
  baseSnapshots: BASE_SNAPSHOTS,
  branchedAt: BRANCHED_AT,
  gitWords: false,
  autoInsert: true,
  autoPropagate: true,
  contextRule: "previous",
  toast: null,
};

let toastSeq = 0;
let commitSeq = 0;

function commit(at: string, summary: string): Commit {
  return {
    id: `c-${Date.now().toString(36)}-${++commitSeq}`,
    at,
    by: USER,
    summary,
  };
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "createProject": {
      const { input, id, now } = action;
      const termbases = [...state.termbases];
      let projectTermbaseId: string | undefined;
      const masters = input.masters
        .map((mid) => termbases.find((t) => t.id === mid)!)
        .filter(Boolean);
      const langs = [input.sourceLang, ...input.targetLangs];
      // A master that lacks a project language gains it, as memoQ does.
      for (const m of masters) {
        const missing = langs.filter((l) => !m.languages.includes(l));
        if (missing.length) {
          const i = termbases.findIndex((t) => t.id === m.id);
          termbases[i] = { ...m, languages: [...m.languages, ...missing] };
        }
      }
      if (input.mode !== "direct") {
        projectTermbaseId = `tb-${id}`;
        termbases.push({
          id: projectTermbaseId,
          name: `${input.name} (project)`,
          kind: "project",
          location: input.location,
          server:
            input.location === "server"
              ? (masters.find((m) => m.server)?.server ??
                "terms.kanzlei-weber.example")
              : undefined,
          languages: langs,
          client: input.client,
          basedOn: input.mode === "layered" ? input.masters : [],
          projectId: id,
          infoTypes: masters[0]?.infoTypes ?? DEFAULT_INFO_TYPES,
        });
      }
      const project: Project = {
        id,
        name: input.name,
        file: input.file,
        words: input.words,
        sourceLang: input.sourceLang,
        targetLangs: input.targetLangs,
        client: input.client,
        setup: {
          mode: input.mode,
          masters: input.masters,
          projectTermbaseId,
          location: input.location,
        },
        createdAt: now,
        // A bilingual file can arrive partly or fully translated.
        progress: input.extracted
          ? confirmedShare(input.extracted.segments)
          : 0,
        segments: input.extracted
          ? input.extracted.segments
          : input.text
            ? segmentText(input.text).map((src, i) => ({
                id: `s${i + 1}`,
                source: [src],
                target: [],
                status: "new" as const,
              }))
            : undefined,
        blocks: input.extracted?.blocks,
        original: input.extracted
          ? {
              bytes: input.extracted.bytes,
              format: input.extracted.format,
              sdlPackage: input.extracted.sdlPackage,
            }
          : undefined,
      };
      return {
        ...state,
        termbases,
        projects: [project, ...state.projects],
        branchedAt: { ...state.branchedAt, [id]: now },
      };
    }
    case "merge": {
      const project = state.projects.find((p) => p.id === action.projectId)!;
      const ptb = project.setup.projectTermbaseId!;
      const masterEntries = state.entries.filter(
        (e) => e.termbaseId === action.masterId,
      );
      const projectEntries = state.entries.filter((e) => e.termbaseId === ptb);
      const rest = state.entries.filter(
        (e) => e.termbaseId !== action.masterId && e.termbaseId !== ptb,
      );
      const res = applyMerge(
        action.changes,
        action.decisions,
        action.masterId,
        masterEntries,
        projectEntries,
        { by: USER, projectName: project.name, at: action.now },
      );
      // Merged entries become the new starting point for later changes.
      const baseSnapshots = { ...state.baseSnapshots };
      for (const pe of res.projectEntries) {
        const m = res.masterEntries.find((e) => e.id === pe.base?.entryId);
        if (m) baseSnapshots[pe.id] = m;
      }
      return {
        ...state,
        entries: [...rest, ...res.masterEntries, ...res.projectEntries],
        baseSnapshots,
      };
    }
    case "updateFromMaster": {
      return {
        ...state,
        branchedAt: { ...state.branchedAt, [action.projectId]: action.now },
      };
    }
    case "createMaster": {
      return { ...state, termbases: [...state.termbases, action.termbase] };
    }
    case "addInfoType": {
      return {
        ...state,
        termbases: state.termbases.map((t) =>
          t.id === action.termbaseId
            ? { ...t, infoTypes: [...t.infoTypes, action.infoType] }
            : t,
        ),
      };
    }
    case "setField": {
      return {
        ...state,
        entries: state.entries.map((e) => {
          if (e.id !== action.entryId) return e;
          const label =
            state.termbases
              .find((t) => t.id === e.termbaseId)
              ?.infoTypes.find((i) => i.id === action.field)?.name ??
            action.field;
          const commit: Commit = {
            id: `c-${Date.now().toString(36)}`,
            at: action.now,
            by: USER,
            summary: action.value ? `Set ${label}` : `Cleared ${label}`,
          };
          return {
            ...e,
            concept: { ...e.concept, [action.field]: action.value },
            version: e.version + 1,
            history: [...e.history, commit],
          };
        }),
      };
    }
    case "setTermField": {
      return {
        ...state,
        entries: state.entries.map((e) => {
          if (e.id !== action.entryId) return e;
          const list = [...(e.terms[action.langCode] ?? [])];
          const term = list[action.termIndex];
          if (!term) return e;
          list[action.termIndex] = {
            ...term,
            fields: { ...term.fields, [action.field]: action.value },
          };
          const label =
            state.termbases
              .find((t) => t.id === e.termbaseId)
              ?.infoTypes.find((i) => i.id === action.field)?.name ??
            action.field;
          const commit: Commit = {
            id: `c-${Date.now().toString(36)}`,
            at: action.now,
            by: USER,
            summary: `Set ${label} of “${term.text}”`,
          };
          return {
            ...e,
            terms: { ...e.terms, [action.langCode]: list },
            version: e.version + 1,
            history: [...e.history, commit],
          };
        }),
      };
    }
    case "setGitWords":
      return { ...state, gitWords: action.value };
    case "setEditorOption":
      return { ...state, [action.option]: action.value };
    case "setContextRule":
      return { ...state, contextRule: action.value };
    case "setTarget":
      return withSegments(state, action.projectId, (segments) =>
        segments.map((sg) =>
          sg.id === action.segmentId
            ? {
                ...sg,
                target: normalize(action.target),
                status: action.status,
                origin:
                  action.status === "new"
                    ? undefined
                    : (action.origin ?? sg.origin),
                // Saved warnings belong to the confirmed text; an edit makes them stale.
                warnings:
                  action.status === "confirmed" ? sg.warnings : undefined,
              }
            : sg,
        ),
      );
    case "confirm":
      return withSegments(state, action.projectId, (segments) => {
        const done = segments.find((sg) => sg.id === action.segmentId)!;
        const key = sourceKey(done.source);
        const ignored = new Set(done.ignored ?? []);
        return segments.map((sg) => {
          if (sg.id === done.id)
            return {
              ...sg,
              status: "confirmed" as const,
              warnings: action.warnings.filter((w) => !ignored.has(w.text)),
            };
          // Auto-propagation fills identical segments that are not confirmed yet;
          // confirmed ones are left alone, as in Trados' default.
          if (
            state.autoPropagate &&
            sg.status !== "confirmed" &&
            sourceKey(sg.source) === key
          ) {
            return {
              ...sg,
              target: done.target,
              status: "confirmed" as const,
              origin: { kind: "repetition" as const, from: done.id },
              warnings: undefined,
            };
          }
          return sg;
        });
      });
    case "ignoreIssue":
      return withSegments(state, action.projectId, (segments) =>
        segments.map((sg) =>
          sg.id === action.segmentId
            ? {
                ...sg,
                ignored: [...(sg.ignored ?? []), action.text],
                warnings: sg.warnings?.filter((w) => w.text !== action.text),
              }
            : sg,
        ),
      );
    case "addTerm": {
      const { input, id, now } = action;
      const p = state.projects.find((x) => x.id === input.projectId)!;
      const termbaseId =
        p.setup.mode === "direct"
          ? p.setup.masters[0]!
          : p.setup.projectTermbaseId!;
      const tb = state.termbases.find((t) => t.id === termbaseId)!;
      const entry: Entry = {
        id,
        termbaseId,
        version: 1,
        concept: {},
        terms: {
          [p.sourceLang]: [{ text: input.source, status: "proposed" }],
          [p.targetLangs[0]!]: [
            {
              text: input.target,
              status: "proposed",
              fields: input.note ? { note: input.note } : undefined,
            },
          ],
        },
        history: [
          {
            id: `c-${id}`,
            at: now,
            by: USER,
            summary: "Added while translating",
            project: p.name,
            segment: input.segment,
          },
        ],
      };
      // A termbase gains the project's languages the first time they are used.
      const languages = [
        ...new Set([...tb.languages, p.sourceLang, p.targetLangs[0]!]),
      ];
      return {
        ...state,
        entries: [...state.entries, entry],
        termbases: state.termbases.map((t) =>
          t.id === termbaseId ? { ...t, languages } : t,
        ),
      };
    }
    case "updateTermbase":
      return {
        ...state,
        termbases: state.termbases.map((t) =>
          t.id === action.termbaseId ? { ...t, ...action.patch } : t,
        ),
      };
    case "removeLanguage": {
      const name = action.langCode;
      return {
        ...state,
        termbases: state.termbases.map((t) =>
          t.id === action.termbaseId
            ? {
                ...t,
                languages: t.languages.filter((l) => l !== action.langCode),
              }
            : t,
        ),
        entries: state.entries.map((e) => {
          if (
            e.termbaseId !== action.termbaseId ||
            (!e.terms[action.langCode] && !e.langFields?.[action.langCode])
          )
            return e;
          const terms = { ...e.terms };
          delete terms[action.langCode];
          const langFields = e.langFields ? { ...e.langFields } : undefined;
          if (langFields) delete langFields[action.langCode];
          return {
            ...e,
            terms,
            langFields,
            version: e.version + 1,
            history: [
              ...e.history,
              commit(action.now, `Removed the language ${name}`),
            ],
          };
        }),
      };
    }
    case "removeInfoType": {
      const tb = state.termbases.find((t) => t.id === action.termbaseId)!;
      const it = tb.infoTypes.find((i) => i.id === action.infoTypeId);
      const drop = (r?: Record<string, string>) => {
        if (!r || !(action.infoTypeId in r)) return r;
        const out = { ...r };
        delete out[action.infoTypeId];
        return out;
      };
      return {
        ...state,
        termbases: state.termbases.map((t) =>
          t.id === action.termbaseId
            ? {
                ...t,
                infoTypes: t.infoTypes.filter(
                  (i) => i.id !== action.infoTypeId,
                ),
              }
            : t,
        ),
        entries: state.entries.map((e) => {
          if (e.termbaseId !== action.termbaseId) return e;
          const concept = drop(e.concept)!;
          const terms = Object.fromEntries(
            Object.entries(e.terms).map(([l, ts]) => [
              l,
              ts.map((t) => ({ ...t, fields: drop(t.fields) })),
            ]),
          );
          const langFields = e.langFields
            ? Object.fromEntries(
                Object.entries(e.langFields).map(([l, f]) => [l, drop(f)!]),
              )
            : undefined;
          const had =
            concept !== e.concept ||
            JSON.stringify(terms) !== JSON.stringify(e.terms) ||
            JSON.stringify(langFields) !== JSON.stringify(e.langFields);
          return had
            ? {
                ...e,
                concept,
                terms,
                langFields,
                version: e.version + 1,
                history: [
                  ...e.history,
                  commit(
                    action.now,
                    `Removed the field ${it?.name ?? action.infoTypeId}`,
                  ),
                ],
              }
            : e;
        }),
      };
    }
    case "deleteTermbase":
      return {
        ...state,
        termbases: state.termbases.filter((t) => t.id !== action.termbaseId),
        entries: state.entries.filter(
          (e) => e.termbaseId !== action.termbaseId,
        ),
      };
    case "createEntry":
      return { ...state, entries: [...state.entries, action.entry] };
    case "saveEntry":
      return {
        ...state,
        entries: state.entries.map((e) =>
          e.id === action.entryId
            ? {
                ...e,
                ...action.content,
                version: e.version + 1,
                history: [...e.history, commit(action.now, action.summary)],
              }
            : e,
        ),
      };
    case "deleteEntries": {
      const ids = new Set(action.entryIds);
      return { ...state, entries: state.entries.filter((e) => !ids.has(e.id)) };
    }
    case "mergeEntries": {
      const ids = new Set(action.removeIds);
      const removed = state.entries.filter((e) => ids.has(e.id));
      return {
        ...state,
        entries: state.entries
          .filter((e) => !ids.has(e.id))
          .map((e) =>
            e.id === action.keepId
              ? {
                  ...e,
                  ...action.content,
                  version: e.version + 1,
                  // The merged entry keeps the history of all of them, as in MultiTerm.
                  history: [...e.history, ...removed.flatMap((r) => r.history)]
                    .sort((a, b) => a.at.localeCompare(b.at))
                    .concat(commit(action.now, action.summary)),
                }
              : e,
          ),
      };
    }
    case "importEntries": {
      const { input } = action;
      const updates = new Map(input.updates.map((u) => [u.entryId, u]));
      let termbases = input.create
        ? [...state.termbases, input.create]
        : state.termbases;
      termbases = termbases.map((t) =>
        t.id === input.termbaseId
          ? {
              ...t,
              languages: [...new Set([...t.languages, ...input.languages])],
              infoTypes: [...t.infoTypes, ...input.infoTypes],
            }
          : t,
      );
      const entries = state.entries.map((e) => {
        const u = updates.get(e.id);
        return u
          ? {
              ...e,
              ...u.content,
              version: e.version + 1,
              history: [...e.history, commit(input.now, u.summary)],
            }
          : e;
      });
      return { ...state, termbases, entries: [...entries, ...input.adds] };
    }
    case "hydrate":
      return { ...initial, ...action.state, toast: null };
    case "resetAll":
      return { ...initial, toast: null };
    case "toast":
      return { ...state, toast: { id: ++toastSeq, text: action.text } };
    case "clearToast":
      return { ...state, toast: null };
  }
}

interface Store {
  state: State;
  dispatch: (a: Action) => void;
  notify: (text: string) => void;
}

const Ctx = createContext<Store | null>(null);

/** What is saved between visits: everything but the passing toast. */
export type SavedState = Omit<State, "toast">;

interface SavedRecord {
  version: number;
  state: SavedState;
}

function savable(state: State): SavedRecord {
  const { toast: _toast, ...rest } = state;
  return { version: STATE_VERSION, state: rest };
}

function looksValid(s: unknown): s is SavedState {
  const x = s as Partial<SavedState> | null;
  return (
    !!x &&
    Array.isArray(x.termbases) &&
    Array.isArray(x.entries) &&
    Array.isArray(x.projects) &&
    typeof x.baseSnapshots === "object" &&
    typeof x.branchedAt === "object"
  );
}

// Loaded once per page, also when React runs effects twice in development.
let boot: Promise<SavedState | null> | null = null;

function loadSaved(): Promise<SavedState | null> {
  boot ??= (async () => {
    if (takeResetRequest()) {
      await clearAll();
      forgetMemories();
      return null;
    }
    await preloadMemories();
    const rec = await load<SavedRecord>("state");
    return rec && rec.version === STATE_VERSION && looksValid(rec.state)
      ? rec.state
      : null;
  })();
  return boot;
}

/** Deletes the saved work and starts again from the sample data. */
export async function resetSavedWork(
  dispatch: (a: Action) => void,
): Promise<void> {
  await clearAll();
  forgetMemories();
  dispatch({ type: "resetAll" });
}

export function StoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initial);
  const [ready, setReady] = useState(false);
  const latest = useRef(state);
  latest.current = state;
  const notify = useCallback(
    (text: string) => dispatch({ type: "toast", text }),
    [],
  );
  const value = useMemo(() => ({ state, dispatch, notify }), [state, notify]);

  useEffect(() => {
    let live = true;
    void loadSaved().then((saved) => {
      if (!live) return;
      if (saved) dispatch({ type: "hydrate", state: saved });
      setReady(true);
    });
    return () => {
      live = false;
    };
  }, []);

  // Saves shortly after each change, and right away when the page is hidden
  // or closed.
  useEffect(() => {
    if (!ready) return;
    const t = setTimeout(() => void save("state", savable(state)), 300);
    return () => clearTimeout(t);
  }, [state, ready]);
  useEffect(() => {
    if (!ready) return;
    const flush = () => void save("state", savable(latest.current));
    const onHide = () => document.visibilityState === "hidden" && flush();
    window.addEventListener("pagehide", flush);
    document.addEventListener("visibilitychange", onHide);
    return () => {
      window.removeEventListener("pagehide", flush);
      document.removeEventListener("visibilitychange", onHide);
    };
  }, [ready]);

  if (!ready)
    return (
      <p className="boot" role="status">
        Opening your work…
      </p>
    );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useStore(): Store {
  const s = useContext(Ctx);
  if (!s) throw new Error("useStore outside StoreProvider");
  return s;
}

// Wording: the same mechanics, labelled for translators or in git terms.
const WORDS = {
  plain: {
    projectChanges: "Project changes",
    reviewMerge: "Review and merge",
    mergeInto: "Merge into",
    merged: "Merged",
    updateFromMaster: "Update from master",
    updated: "Project updated from master",
    history: "History",
    basedOn: "Based on",
    projectTermbase: "Project termbase",
    useProject: "Use project version",
    keepMaster: "Keep master version",
    keepBoth: "Keep both",
    skip: "Leave out",
    accept: "Include",
    newEntry: "New entry",
    changedEntry: "Changed",
    conflict: "Conflict",
    change: "change",
    changes: "changes",
  },
  git: {
    projectChanges: "Branch diff",
    reviewMerge: "Diff and merge",
    mergeInto: "Merge into",
    merged: "Merged",
    updateFromMaster: "Pull from master",
    updated: "Pulled from master",
    history: "Log",
    basedOn: "Branched from",
    projectTermbase: "Branch",
    useProject: "Use ours",
    keepMaster: "Use theirs",
    keepBoth: "Union",
    skip: "Drop",
    accept: "Pick",
    newEntry: "Added",
    changedEntry: "Modified",
    conflict: "Conflict",
    change: "commit",
    changes: "commits",
  },
} as const;

export type WordKey = keyof (typeof WORDS)["plain"];

export function useWords(): (k: WordKey) => string {
  const { state } = useStore();
  const table = state.gitWords ? WORDS.git : WORDS.plain;
  return (k) => table[k];
}
