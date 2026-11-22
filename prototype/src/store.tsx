import { createContext, useCallback, useContext, useMemo, useReducer, type ReactNode } from "react";
import {
  BASE_SNAPSHOTS, BRANCHED_AT, DEFAULT_INFO_TYPES, INITIAL_ENTRIES, INITIAL_PROJECTS, INITIAL_TERMBASES, USER,
  type Commit, type Entry, type InfoType, type LangCode, type Location, type Project, type SetupMode, type Termbase,
} from "./data";
import { applyMerge, type Change, type Decision } from "./merge";
import { normalize, segmentText, wordCount, type Run, type Segment, type SegmentStatus } from "./editor/model";

export interface State {
  termbases: Termbase[];
  entries: Entry[];
  projects: Project[];
  baseSnapshots: Record<string, Entry>;
  branchedAt: Record<string, string>;
  gitWords: boolean;
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
}

export interface NewTermInput {
  projectId: string;
  source: string;
  target: string;
  note?: string;
  segment: string;
}

type Action =
  | { type: "createProject"; input: NewProjectInput; id: string; now: string }
  | { type: "merge"; projectId: string; masterId: string; changes: Change[]; decisions: Record<string, Decision>; now: string }
  | { type: "updateFromMaster"; projectId: string; now: string }
  | { type: "addInfoType"; termbaseId: string; infoType: InfoType }
  | { type: "createMaster"; termbase: Termbase }
  | { type: "setField"; entryId: string; field: string; value: string; now: string }
  | { type: "setTermField"; entryId: string; langCode: LangCode; termIndex: number; field: string; value: string; now: string }
  | { type: "setGitWords"; value: boolean }
  | { type: "setTarget"; projectId: string; segmentId: string; target: Run[]; status: SegmentStatus; origin?: Segment["origin"] }
  | { type: "addTerm"; input: NewTermInput; id: string; now: string }
  | { type: "toast"; text: string }
  | { type: "clearToast" };

const initial: State = {
  termbases: INITIAL_TERMBASES,
  entries: INITIAL_ENTRIES,
  projects: INITIAL_PROJECTS,
  baseSnapshots: BASE_SNAPSHOTS,
  branchedAt: BRANCHED_AT,
  gitWords: false,
  toast: null,
};

let toastSeq = 0;

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "createProject": {
      const { input, id, now } = action;
      const termbases = [...state.termbases];
      let projectTermbaseId: string | undefined;
      const masters = input.masters.map((mid) => termbases.find((t) => t.id === mid)!).filter(Boolean);
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
          server: input.location === "server" ? masters.find((m) => m.server)?.server ?? "terms.kanzlei-weber.example" : undefined,
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
        setup: { mode: input.mode, masters: input.masters, projectTermbaseId, location: input.location },
        createdAt: now,
        progress: 0,
        segments: input.text ? segmentText(input.text).map((src, i) => ({ id: `s${i + 1}`, source: [src], target: [], status: "new" as const })) : undefined,
      };
      return { ...state, termbases, projects: [project, ...state.projects], branchedAt: { ...state.branchedAt, [id]: now } };
    }
    case "merge": {
      const project = state.projects.find((p) => p.id === action.projectId)!;
      const ptb = project.setup.projectTermbaseId!;
      const masterEntries = state.entries.filter((e) => e.termbaseId === action.masterId);
      const projectEntries = state.entries.filter((e) => e.termbaseId === ptb);
      const rest = state.entries.filter((e) => e.termbaseId !== action.masterId && e.termbaseId !== ptb);
      const res = applyMerge(action.changes, action.decisions, action.masterId, masterEntries, projectEntries, { by: USER, projectName: project.name, at: action.now });
      // Merged entries become the new starting point for later changes.
      const baseSnapshots = { ...state.baseSnapshots };
      for (const pe of res.projectEntries) {
        const m = res.masterEntries.find((e) => e.id === pe.base?.entryId);
        if (m) baseSnapshots[pe.id] = m;
      }
      return { ...state, entries: [...rest, ...res.masterEntries, ...res.projectEntries], baseSnapshots };
    }
    case "updateFromMaster": {
      return { ...state, branchedAt: { ...state.branchedAt, [action.projectId]: action.now } };
    }
    case "createMaster": {
      return { ...state, termbases: [...state.termbases, action.termbase] };
    }
    case "addInfoType": {
      return {
        ...state,
        termbases: state.termbases.map((t) => (t.id === action.termbaseId ? { ...t, infoTypes: [...t.infoTypes, action.infoType] } : t)),
      };
    }
    case "setField": {
      return {
        ...state,
        entries: state.entries.map((e) => {
          if (e.id !== action.entryId) return e;
          const label = state.termbases.find((t) => t.id === e.termbaseId)?.infoTypes.find((i) => i.id === action.field)?.name ?? action.field;
          const commit: Commit = { id: `c-${Date.now().toString(36)}`, at: action.now, by: USER, summary: action.value ? `Set ${label}` : `Cleared ${label}` };
          return { ...e, concept: { ...e.concept, [action.field]: action.value }, version: e.version + 1, history: [...e.history, commit] };
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
          list[action.termIndex] = { ...term, fields: { ...term.fields, [action.field]: action.value } };
          const label = state.termbases.find((t) => t.id === e.termbaseId)?.infoTypes.find((i) => i.id === action.field)?.name ?? action.field;
          const commit: Commit = { id: `c-${Date.now().toString(36)}`, at: action.now, by: USER, summary: `Set ${label} of “${term.text}”` };
          return { ...e, terms: { ...e.terms, [action.langCode]: list }, version: e.version + 1, history: [...e.history, commit] };
        }),
      };
    }
    case "setGitWords":
      return { ...state, gitWords: action.value };
    case "setTarget": {
      return {
        ...state,
        projects: state.projects.map((p) => {
          if (p.id !== action.projectId || !p.segments) return p;
          const segments = p.segments.map((sg) =>
            sg.id === action.segmentId ? { ...sg, target: normalize(action.target), status: action.status, origin: action.origin ?? sg.origin } : sg,
          );
          const total = segments.reduce((n, sg) => n + wordCount(sg.source), 0) || 1;
          const done = segments.filter((sg) => sg.status === "confirmed").reduce((n, sg) => n + wordCount(sg.source), 0);
          return { ...p, segments, progress: done / total };
        }),
      };
    }
    case "addTerm": {
      const { input, id, now } = action;
      const p = state.projects.find((x) => x.id === input.projectId)!;
      const termbaseId = p.setup.mode === "direct" ? p.setup.masters[0]! : p.setup.projectTermbaseId!;
      const tb = state.termbases.find((t) => t.id === termbaseId)!;
      const entry: Entry = {
        id,
        termbaseId,
        version: 1,
        concept: {},
        terms: {
          [p.sourceLang]: [{ text: input.source, status: "proposed" }],
          [p.targetLangs[0]!]: [{ text: input.target, status: "proposed", fields: input.note ? { note: input.note } : undefined }],
        },
        history: [{ id: `c-${id}`, at: now, by: USER, summary: "Added while translating", project: p.name, segment: input.segment }],
      };
      // A termbase gains the project's languages the first time they are used.
      const languages = [...new Set([...tb.languages, p.sourceLang, p.targetLangs[0]!])];
      return {
        ...state,
        entries: [...state.entries, entry],
        termbases: state.termbases.map((t) => (t.id === termbaseId ? { ...t, languages } : t)),
      };
    }
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

export function StoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initial);
  const notify = useCallback((text: string) => dispatch({ type: "toast", text }), []);
  const value = useMemo(() => ({ state, dispatch, notify }), [state, notify]);
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
