import type { LocalProvider } from "./editor/provider";
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
  takeStartRequest,
} from "./persist";
import {
  forgetMemories,
  preloadMemories,
  renameSavedMemory,
} from "./editor/memory";
import { migrateV1 } from "./migrate";
import { fmtTime } from "./format";
import { forgetDictionaries } from "./editor/spelling";
import {
  DEFAULT_INFO_TYPES,
  projectProgress,
  type Commit,
  type DeletedEntry,
  type Entry,
  type EntrySnapshot,
  type Removal,
  type InfoType,
  type LangCode,
  type Project,
  type SpellDictionary,
  type Template,
  type Termbase,
  type TranslationMemory,
  DEFAULT_QA,
  DEFAULT_USER_NAME,
} from "./data";
import { SAMPLE } from "./sample";
import { applyMerge, type Change, type Decision } from "./merge";
import {
  normalize,
  sourceKey,
  type Run,
  type Segment,
  type SegmentComment,
  type SegmentStatus,
} from "./editor/model";

/** Where a segment list is: a file of a project, in a target language. */
export interface SegmentsAt {
  projectId: string;
  fileId: string;
  lang: LangCode;
}

function withSegments(
  state: State,
  at: SegmentsAt,
  f: (segments: Segment[]) => Segment[],
): State {
  return {
    ...state,
    projects: state.projects.map((p) => {
      if (p.id !== at.projectId) return p;
      const files = p.files.map((file) => {
        const segs = file.id === at.fileId ? file.segments[at.lang] : undefined;
        return segs
          ? { ...file, segments: { ...file.segments, [at.lang]: f(segs) } }
          : file;
      });
      const next = { ...p, files };
      return { ...next, progress: projectProgress(next) };
    }),
  };
}

export interface State {
  termbases: Termbase[];
  entries: Entry[];
  projects: Project[];
  memories: TranslationMemory[];
  templates: Template[];
  dictionaries: SpellDictionary[];
  /** Translation services asked from this browser, for work on this computer. */
  providers: LocalProvider[];
  baseSnapshots: Record<string, Entry>;
  branchedAt: Record<string, string>;
  /** Entries deleted from termbases kept in this browser, newest first. */
  deleted: DeletedEntry[];
  /** Master entries the project termbases kept in this browser leave out. */
  removals: Removal[];
  gitWords: boolean;
  /** Editor: put the best memory match into an empty segment when it opens (Trados does this by default). */
  autoInsert: boolean;
  /** Editor: offer the terms and memory words that complete a word being typed. */
  autoComplete: boolean;
  /** Editor: confirming fills identical segments further on (auto-propagation). */
  autoPropagate: boolean;
  /** Context matches for new projects: the segment before must match (Trados) or before and after (memoQ). */
  contextRule: "previous" | "both";
  /** The name the person's work on this computer is signed with. */
  userName: string;
  toast: { id: number; text: string } | null;
}

/** A project's settings or files changed, with what that changes elsewhere. */
export interface ProjectChange {
  project: Project;
  /** Masters that gain the project languages they lack. */
  addLanguages: string[];
  /** Project memories for new target languages. */
  addMemories: TranslationMemory[];
  /** Project memories of target languages the project no longer has. */
  removeMemories: string[];
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
  lang: LangCode;
  source: string;
  target: string;
  note?: string;
  segment: string;
}

type Action =
  | { type: "hydrate"; state: SavedState }
  | { type: "setUserName"; name: string }
  | { type: "restoreVersion"; entryId: string; commitId: string; now: string }
  | { type: "restoreDeleted"; entryId: string; now: string }
  | { type: "takeFromMaster"; termbaseId: string; masterEntryId: string; id: string; now: string }
  | { type: "leaveOut"; termbaseId: string; masterId: string; entryId: string; now: string }
  | { type: "bringBack"; termbaseId: string; masterId: string; entryId: string }
  | { type: "resetAll" }
  | { type: "createProject"; change: ProjectChange; now: string }
  | { type: "updateProject"; change: ProjectChange }
  | { type: "deleteProject"; projectId: string }
  | { type: "createMemory"; memory: TranslationMemory }
  | { type: "deleteMemory"; name: string }
  | { type: "saveTemplate"; template: Template }
  | { type: "addDictionary"; dictionary: SpellDictionary }
  | { type: "deleteDictionary"; name: string }
  | { type: "deleteTemplate"; templateId: string }
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
      option: "autoInsert" | "autoPropagate" | "autoComplete";
      value: boolean;
    }
  | { type: "setContextRule"; value: "previous" | "both" }
  | {
      type: "confirm";
      at: SegmentsAt;
      segmentId: string;
      warnings: { kind: string; text: string }[];
    }
  | { type: "ignoreIssue"; at: SegmentsAt; segmentId: string; text: string }
  | { type: "setComments"; at: SegmentsAt; segmentId: string; comments: SegmentComment[] }
  | { type: "saveProvider"; provider: LocalProvider }
  | { type: "deleteProvider"; name: string }
  | {
      /** Translations changed by find and replace: they are drafts again. */
      type: "replaceTargets";
      at: SegmentsAt;
      changes: { segmentId: string; target: Run[] }[];
    }
  | {
      type: "setTarget";
      at: SegmentsAt;
      segmentId: string;
      target: Run[];
      status: SegmentStatus;
      origin?: Segment["origin"];
    }
  | { type: "addTerm"; input: NewTermInput; id: string; now: string }
  | {
      type: "updateTermbase";
      termbaseId: string;
      patch: Partial<Pick<Termbase, "name" | "languages" | "infoTypes" | "rules">>;
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
  | { type: "clearToast" }
  | {
      type: "remoteLoad";
      project: Project;
      memories: TranslationMemory[];
      termbases: Termbase[];
      entries: Entry[];
    }
  | { type: "remoteSegments"; at: SegmentsAt; segments: Segment[] }
  | { type: "remoteTermbase"; termbase: Termbase; entries: Entry[] }
  | { type: "remoteProject"; projectId: string; patch: Partial<Project> }
  | { type: "remoteEntry"; localId: string; entry: Entry }
  | { type: "remoteUnload"; projectId?: string };

const initial: State = {
  termbases: [],
  entries: [],
  projects: [],
  memories: [],
  templates: [],
  dictionaries: [],
  providers: [],
  baseSnapshots: {},
  branchedAt: {},
  deleted: [],
  removals: [],
  gitWords: false,
  autoInsert: true,
  autoComplete: true,
  autoPropagate: true,
  contextRule: "previous",
  userName: DEFAULT_USER_NAME,
  toast: null,
};

/** Whether the sample project, or anything of it, is among the person's work. */
function hasSample(state: Pick<State, "projects" | "termbases" | "memories">): boolean {
  return (
    state.projects.some((p) => SAMPLE.projects.some((x) => x.id === p.id)) ||
    state.termbases.some((t) => SAMPLE.termbases.some((x) => x.id === t.id)) ||
    state.memories.some((m) => SAMPLE.memories.some((x) => x.name === m.name))
  );
}

/** The state with the sample project and what it works with added. */
function withSample(state: State): State {
  if (hasSample(state)) return state;
  return {
    ...state,
    termbases: [...state.termbases, ...SAMPLE.termbases],
    entries: [...state.entries, ...SAMPLE.entries],
    projects: [...SAMPLE.projects, ...state.projects],
    memories: [...state.memories, ...SAMPLE.memories],
    baseSnapshots: { ...state.baseSnapshots, ...SAMPLE.baseSnapshots },
    branchedAt: { ...state.branchedAt, ...SAMPLE.branchedAt },
  };
}

let toastSeq = 0;
let commitSeq = 0;

function commit(by: string, at: string, summary: string): Commit {
  return {
    id: `c-${Date.now().toString(36)}-${++commitSeq}`,
    at,
    by,
    summary,
  };
}

/** Whether an entry is in a termbase loaded from the team server. */
export function remoteEntry(state: Pick<State, "termbases">, e: Entry): boolean {
  return !!state.termbases.find((t) => t.id === e.termbaseId)?.remote;
}

/** Takes out what came from the team server: everything, or one project's. */
function withoutRemote(state: State, projectId?: string): State {
  const gone = state.projects.filter((p) => p.remote && (!projectId || p.id === projectId));
  if (!gone.length && projectId) return state;
  const projects = state.projects.filter((p) => !gone.includes(p));
  // Memories and termbases stay while another loaded project uses them.
  const usedMemories = new Set(
    projects.filter((p) => p.remote).flatMap((p) => Object.values(p.memories).flat().map((m) => m.memory)),
  );
  const usedTermbases = new Set(
    projects.filter((p) => p.remote).flatMap((p) => [...p.setup.masters, p.setup.projectTermbaseId ?? ""]),
  );
  // Termbases opened on their own stay while only a project goes.
  const termbases = state.termbases.filter(
    (t) => !t.remote || usedTermbases.has(t.id) || (!!projectId && t.opened),
  );
  const kept = new Set(termbases.map((t) => t.id));
  return {
    ...state,
    projects,
    memories: state.memories.filter((m) => !m.remote || usedMemories.has(m.name)),
    termbases,
    entries: state.entries.filter((e) => kept.has(e.termbaseId)),
  };
}

/** The content of an entry, as a version of it is kept. */
function snapshotOf(e: Entry): EntrySnapshot {
  return { concept: e.concept, terms: e.terms, langFields: e.langFields };
}

// Actions that take entries away without deleting them: what a server
// sent leaves the page, or the browser's work is replaced as a whole.
const NOT_A_DELETION = new Set<Action["type"]>([
  "hydrate",
  "resetAll",
  "remoteLoad",
  "remoteUnload",
  "remoteTermbase",
  "remoteEntry",
]);

/**
 * What every change to the termbases kept in this browser leaves behind,
 * whichever action made it: the entry's content with the change that
 * produced it, so a version can be read and restored, and an entry that
 * is gone while its termbase is still there, so it can be brought back.
 */
function keepTrack(prev: State, next: State, action: Action): State {
  if (next.entries === prev.entries && next.termbases === prev.termbases)
    return next;
  const local = new Set(
    next.termbases.filter((t) => !t.remote).map((t) => t.id),
  );
  const before = new Map(prev.entries.map((e) => [e.id, e]));
  let changed = false;
  const entries = next.entries.map((e) => {
    const was = before.get(e.id);
    if (!local.has(e.termbaseId) || was === e) return e;
    const history = [...e.history];
    let stamped = false;
    // What the entry held until now, where its last change did not keep it
    // (entries from before versions were kept, the sample's, imported ones).
    const at = was ? was.history.length - 1 : -1;
    if (was && at >= 0 && history[at] && history[at].id === was.history[at]!.id && !history[at].snapshot && history.length > was.history.length) {
      history[at] = { ...history[at], snapshot: snapshotOf(was) };
      stamped = true;
    }
    const last = history[history.length - 1];
    if (last && !last.snapshot) {
      history[history.length - 1] = { ...last, snapshot: snapshotOf(e) };
      stamped = true;
    }
    if (!stamped) return e;
    changed = true;
    return { ...e, history };
  });
  let deleted = next.deleted;
  let removals = next.removals;
  if (!NOT_A_DELETION.has(action.type)) {
    const now = new Set(next.entries.map((e) => e.id));
    const gone = prev.entries.filter(
      (e) => local.has(e.termbaseId) && !now.has(e.id),
    );
    if (gone.length) {
      const at = new Date().toISOString();
      const summary =
        action.type === "mergeEntries"
          ? "Merged into another entry"
          : action.type === "merge"
            ? "Deleted by a merge"
            : "Deleted";
      deleted = [
        ...gone.map((entry) => ({ entry, at, by: prev.userName, summary })),
        ...deleted,
      ];
    }
  }
  // A termbase that is gone takes what was kept for it along.
  if (next.termbases !== prev.termbases) {
    if (deleted.some((d) => !local.has(d.entry.termbaseId)))
      deleted = deleted.filter((d) => local.has(d.entry.termbaseId));
    if (removals.some((x) => !local.has(x.termbaseId) || !local.has(x.masterId)))
      removals = removals.filter(
        (x) => local.has(x.termbaseId) && local.has(x.masterId),
      );
  }
  return changed || deleted !== next.deleted || removals !== next.removals
    ? { ...next, entries: changed ? entries : next.entries, deleted, removals }
    : next;
}

function reducer(state: State, action: Action): State {
  return keepTrack(state, reduce(state, action), action);
}

function reduce(state: State, action: Action): State {
  switch (action.type) {
    case "remoteLoad": {
      const { project, memories, termbases, entries } = action;
      const tbIds = new Set(termbases.map((t) => t.id));
      const names = new Set(memories.map((m) => m.name));
      const others = state.projects.filter((p) => p.id !== project.id);
      return {
        ...state,
        projects: [{ ...project, progress: projectProgress(project) }, ...others],
        memories: [...state.memories.filter((m) => !names.has(m.name)), ...memories],
        termbases: [...state.termbases.filter((t) => !tbIds.has(t.id)), ...termbases],
        entries: [...state.entries.filter((e) => !tbIds.has(e.termbaseId)), ...entries],
      };
    }
    case "remoteTermbase": {
      const t = action.termbase;
      const before = state.termbases.find((x) => x.id === t.id);
      return {
        ...state,
        termbases: [...state.termbases.filter((x) => x.id !== t.id), { ...before, ...t }],
        entries: [...state.entries.filter((e) => e.termbaseId !== t.id), ...action.entries],
      };
    }
    case "remoteSegments": {
      const byId = new Map(action.segments.map((sg) => [sg.id, sg]));
      return withSegments(state, action.at, (segs) => segs.map((sg) => byId.get(sg.id) ?? sg));
    }
    case "remoteProject":
      return {
        ...state,
        projects: state.projects.map((p) => (p.id === action.projectId ? { ...p, ...action.patch } : p)),
      };
    case "remoteEntry":
      return {
        ...state,
        entries: state.entries.map((e) => (e.id === action.localId ? action.entry : e)),
      };
    case "remoteUnload":
      return withoutRemote(state, action.projectId);
    case "createProject": {
      const { change, now } = action;
      const p = change.project;
      const langs = [p.sourceLang, ...p.targetLangs];
      let termbases = withLanguages(
        state.termbases,
        change.addLanguages,
        langs,
      );
      let setup = p.setup;
      if (setup.mode !== "direct") {
        const masters = setup.masters
          .map((mid) => termbases.find((t) => t.id === mid))
          .filter((t): t is Termbase => !!t);
        const id = `tb-${p.id}`;
        termbases = [
          ...termbases,
          {
            id,
            name: `${p.name} (project)`,
            kind: "project",
            location: setup.location,
            server:
              setup.location === "server"
                ? masters.find((m) => m.server)?.server
                : undefined,
            languages: langs,
            client: p.client,
            basedOn: setup.mode === "layered" ? setup.masters : [],
            projectId: p.id,
            infoTypes: masters[0]?.infoTypes ?? DEFAULT_INFO_TYPES,
          },
        ];
        setup = { ...setup, projectTermbaseId: id };
      }
      const project = { ...p, setup, progress: projectProgress(p) };
      return {
        ...state,
        termbases,
        memories: [...state.memories, ...change.addMemories],
        projects: [project, ...state.projects],
        branchedAt: { ...state.branchedAt, [p.id]: now },
      };
    }
    case "updateProject": {
      const { change } = action;
      const p = change.project;
      const langs = [p.sourceLang, ...p.targetLangs];
      const termbases = withLanguages(
        state.termbases,
        change.addLanguages,
        langs,
      ).map((t) => {
        if (t.id !== p.setup.projectTermbaseId) return t;
        // The project termbase follows the project's languages and masters.
        return {
          ...t,
          languages: [...new Set([...t.languages, ...langs])],
          basedOn: p.setup.mode === "layered" ? p.setup.masters : t.basedOn,
          client: p.client,
        };
      });
      const removed = new Set(change.removeMemories);
      return {
        ...state,
        termbases,
        memories: [
          ...state.memories.filter((m) => !removed.has(m.name)),
          ...change.addMemories,
        ],
        projects: state.projects.map((x) =>
          x.id === p.id ? { ...p, progress: projectProgress(p) } : x,
        ),
      };
    }
    case "deleteProject": {
      const p = state.projects.find((x) => x.id === action.projectId);
      if (!p) return state;
      const ptb = p.setup.projectTermbaseId;
      const gone = new Set(
        state.entries.filter((e) => e.termbaseId === ptb).map((e) => e.id),
      );
      const baseSnapshots = { ...state.baseSnapshots };
      for (const id of gone) delete baseSnapshots[id];
      const branchedAt = { ...state.branchedAt };
      delete branchedAt[p.id];
      return {
        ...state,
        projects: state.projects.filter((x) => x.id !== p.id),
        termbases: state.termbases.filter((t) => t.id !== ptb),
        entries: state.entries.filter((e) => !gone.has(e.id)),
        memories: state.memories.filter((m) => m.projectId !== p.id),
        baseSnapshots,
        branchedAt,
      };
    }
    case "createMemory":
      return { ...state, memories: [...state.memories, action.memory] };
    case "deleteMemory":
      return {
        ...state,
        memories: state.memories.filter((m) => m.name !== action.name),
      };
    case "saveTemplate":
      return {
        ...state,
        templates: state.templates.some((t) => t.id === action.template.id)
          ? state.templates.map((t) =>
              t.id === action.template.id ? action.template : t,
            )
          : [...state.templates, action.template],
      };
    case "addDictionary":
      return {
        ...state,
        dictionaries: [
          ...state.dictionaries.filter(
            (d) => d.name !== action.dictionary.name,
          ),
          action.dictionary,
        ],
      };
    case "deleteDictionary":
      return {
        ...state,
        dictionaries: state.dictionaries.filter((d) => d.name !== action.name),
      };
    case "deleteTemplate":
      return {
        ...state,
        templates: state.templates.filter((t) => t.id !== action.templateId),
      };
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
        { by: state.userName, projectName: project.name, at: action.now },
      );
      // Merged entries become the new starting point for later changes.
      const baseSnapshots = { ...state.baseSnapshots };
      for (const pe of res.projectEntries) {
        const m = res.masterEntries.find((e) => e.id === pe.base?.entryId);
        if (m) baseSnapshots[pe.id] = m;
      }
      // The master's entries the merge deleted are no longer left out anywhere.
      const removed = new Set(res.removed);
      return {
        ...state,
        entries: [...rest, ...res.masterEntries, ...res.projectEntries],
        baseSnapshots,
        removals: removed.size
          ? state.removals.filter(
              (x) => !(x.masterId === action.masterId && removed.has(x.entryId)),
            )
          : state.removals,
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
            by: state.userName,
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
            by: state.userName,
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
      return withSegments(state, action.at, (segments) =>
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
    case "saveProvider":
      return {
        ...state,
        providers: [...state.providers.filter((p) => p.name.toLowerCase() !== action.provider.name.toLowerCase()), action.provider].sort((x, y) => x.name.localeCompare(y.name)),
      };
    case "deleteProvider":
      return { ...state, providers: state.providers.filter((p) => p.name !== action.name) };
    case "setComments":
      return withSegments(state, action.at, (segments) =>
        segments.map((sg) => (sg.id === action.segmentId ? { ...sg, comments: action.comments.length ? action.comments : undefined } : sg)),
      );
    case "replaceTargets": {
      const to = new Map(action.changes.map((c) => [c.segmentId, c.target]));
      return withSegments(state, action.at, (segments) =>
        segments.map((sg) => {
          const target = to.get(sg.id);
          if (!target || sg.locked) return sg;
          const text = normalize(target);
          const empty = !text.some((r) => typeof r !== "string" || r.trim());
          // An open editor shows the new text, and saved warnings belonged to the old one.
          return { ...sg, target: text, status: empty ? ("new" as const) : ("draft" as const), origin: empty ? undefined : sg.origin, warnings: undefined, rev: (sg.rev ?? 0) + 1 };
        }),
      );
    }
    case "confirm":
      return withSegments(state, action.at, (segments) => {
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
            !sg.locked &&
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
      return withSegments(state, action.at, (segments) =>
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
          ? (p.setup.writeTo ?? p.setup.masters[0]!)
          : p.setup.projectTermbaseId!;
      const tb = state.termbases.find((t) => t.id === termbaseId)!;
      const entry: Entry = {
        id,
        termbaseId,
        version: 1,
        concept: {},
        terms: {
          [p.sourceLang]: [{ text: input.source, status: "proposed" }],
          [input.lang]: [
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
            by: state.userName,
            summary: "Added while translating",
            project: p.name,
            segment: input.segment,
          },
        ],
      };
      // A termbase gains the project's languages the first time they are used.
      const languages = [
        ...new Set([...tb.languages, p.sourceLang, input.lang]),
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
              commit(state.userName, action.now, `Removed the language ${name}`),
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
                // A field that is gone is in no review rule either.
                rules: t.rules?.minorFields?.includes(action.infoTypeId)
                  ? {
                      ...t.rules,
                      minorFields: t.rules.minorFields.filter(
                        (f) => f !== action.infoTypeId,
                      ),
                    }
                  : t.rules,
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
                    state.userName,
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
                history: [...e.history, commit(state.userName, action.now, action.summary)],
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
                    .concat(commit(state.userName, action.now, action.summary)),
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
              history: [...e.history, commit(state.userName, input.now, u.summary)],
            }
          : e;
      });
      return { ...state, termbases, entries: [...entries, ...input.adds] };
    }
    case "hydrate":
      return { ...initial, ...action.state, toast: null };
    case "resetAll":
      return { ...initial, toast: null };
    case "restoreVersion": {
      return {
        ...state,
        entries: state.entries.map((e) => {
          if (e.id !== action.entryId) return e;
          const v = e.history.find((c) => c.id === action.commitId);
          if (!v?.snapshot) return e;
          return {
            ...e,
            ...v.snapshot,
            version: e.version + 1,
            history: [
              ...e.history,
              commit(state.userName, action.now, `Restored the version of ${fmtTime(v.at)}`),
            ],
          };
        }),
      };
    }
    case "restoreDeleted": {
      const d = state.deleted.find((x) => x.entry.id === action.entryId);
      if (!d || !state.termbases.some((t) => t.id === d.entry.termbaseId))
        return state;
      const entry: Entry = {
        ...d.entry,
        version: d.entry.version + 1,
        history: [
          ...d.entry.history,
          commit(state.userName, action.now, "Brought back"),
        ],
      };
      return {
        ...state,
        entries: [...state.entries, entry],
        deleted: state.deleted.filter((x) => x.entry.id !== action.entryId),
      };
    }
    case "takeFromMaster": {
      // The master's entry, copied into the project to be changed there.
      const m = state.entries.find((e) => e.id === action.masterEntryId);
      const tb = state.termbases.find((t) => t.id === action.termbaseId);
      if (!m || !tb) return state;
      const copy: Entry = {
        ...m,
        id: action.id,
        termbaseId: tb.id,
        version: 1,
        base: { termbaseId: m.termbaseId, entryId: m.id, version: m.version },
        history: [
          commit(state.userName, action.now, "Taken from the master to change"),
        ],
      };
      return {
        ...state,
        entries: [...state.entries, copy],
        baseSnapshots: { ...state.baseSnapshots, [copy.id]: m },
      };
    }
    case "leaveOut": {
      if (
        state.removals.some(
          (x) => x.termbaseId === action.termbaseId && x.masterId === action.masterId && x.entryId === action.entryId,
        )
      )
        return state;
      return {
        ...state,
        removals: [
          ...state.removals,
          { termbaseId: action.termbaseId, masterId: action.masterId, entryId: action.entryId, at: action.now, by: state.userName },
        ],
      };
    }
    case "bringBack":
      return {
        ...state,
        removals: state.removals.filter(
          (x) => !(x.termbaseId === action.termbaseId && x.masterId === action.masterId && x.entryId === action.entryId),
        ),
      };
    case "setUserName":
      return { ...state, userName: action.name.trim() || DEFAULT_USER_NAME };
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
  // What came from the team server is the server's; the browser keeps
  // only the person's own work.
  const { toast: _toast, ...rest } = withoutRemote(state);
  return { version: STATE_VERSION, state: rest };
}

function looksValid(s: unknown): s is SavedState {
  const x = s as Partial<SavedState> | null;
  return (
    !!x &&
    Array.isArray(x.termbases) &&
    Array.isArray(x.entries) &&
    Array.isArray(x.projects) &&
    Array.isArray(x.memories) &&
    Array.isArray(x.templates) &&
    typeof x.baseSnapshots === "object" &&
    typeof x.branchedAt === "object"
  );
}

/**
 * Settings added since the state was saved get their defaults: the
 * dictionaries list, and the rules, dictionaries and words of each
 * project's and template's checks.
 */
function withDefaults(s: SavedState): SavedState {
  const qa = <T extends { qa: Partial<typeof DEFAULT_QA> }>(x: T) => ({
    ...x,
    qa: { ...DEFAULT_QA, ...x.qa },
  });
  return {
    ...s,
    userName: s.userName || DEFAULT_USER_NAME,
    deleted: s.deleted ?? [],
    removals: s.removals ?? [],
    dictionaries: s.dictionaries ?? [],
    providers: s.providers ?? [],
    autoComplete: s.autoComplete ?? true,
    // What the browser keeps is on this computer: termbases saved with a
    // server that never existed are shown as what they are.
    termbases: s.termbases.map((t) => {
      const { extraEntries: _extra, ...rest } = t as Termbase & {
        extraEntries?: number;
      };
      return { ...rest, location: "local" as const, server: undefined };
    }),
    projects: s.projects
      .map(qa)
      .map((p) => ({ ...p, setup: { ...p.setup, location: "local" as const } })),
    templates: s.templates.map(qa),
  };
}

/** Masters that gain the languages they lack, as the user chose. */
function withLanguages(
  termbases: Termbase[],
  ids: string[],
  langs: LangCode[],
): Termbase[] {
  return termbases.map((t) => {
    if (!ids.includes(t.id)) return t;
    const missing = langs.filter((l) => !t.languages.includes(l));
    return missing.length
      ? { ...t, languages: [...t.languages, ...missing] }
      : t;
  });
}

// Loaded once per page, also when React runs effects twice in development.
let boot: Promise<SavedState | null> | null = null;

function loadSaved(): Promise<SavedState | null> {
  boot ??= (async () => {
    const start = takeStartRequest();
    const sampled = (s: SavedState | null): SavedState | null => {
      if (!start.sample) return s;
      const { toast: _toast, ...rest } = withSample({
        ...initial,
        ...s,
        toast: null,
      });
      return rest;
    };
    if (start.reset) {
      await clearAll();
      forgetMemories();
      forgetDictionaries();
      return sampled(null);
    }
    const rec = await load<SavedRecord>("state");
    let state: SavedState | null = null;
    if (rec?.version === 1) {
      // Work saved before projects had several files, languages and
      // memories: moved into the current shape, nothing dropped.
      const m = migrateV1(
        rec.state as unknown as Parameters<typeof migrateV1>[0],
      );
      for (const [from, to] of m.renamedMemories)
        await renameSavedMemory(from, to);
      state = m.state;
    } else if (rec?.version === STATE_VERSION && looksValid(rec.state)) {
      state = withDefaults(rec.state);
    }
    await preloadMemories();
    return sampled(state);
  })();
  return boot;
}

/** Deletes everything this browser keeps: projects, termbases, memories and settings. */
export async function resetSavedWork(
  dispatch: (a: Action) => void,
): Promise<void> {
  await clearAll();
  forgetMemories();
  forgetDictionaries();
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

const localViews = new WeakMap<State, State>();

/** The state without what came from the team server: the person's own work. */
export function localView(state: State): State {
  let v = localViews.get(state);
  if (!v) {
    v = { ...withoutRemote(state), toast: state.toast };
    localViews.set(state, v);
  }
  return v;
}

/**
 * The store for pages about the person's own work (their memories,
 * termbases, local projects): server projects and what they use are left
 * out, so they are not offered for local work.
 */
export function useLocalStore(): Store {
  const s = useStore();
  return useMemo(() => ({ ...s, state: localView(s.state) }), [s]);
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
