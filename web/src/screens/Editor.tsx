import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import {
  AlertTriangle,
  Languages,
  MessageSquare,
  ArrowLeft,
  BarChart3,
  Check,
  CircleDashed,
  Copy,
  Database,
  Download,
  Eye,
  Upload,
  GitMerge,
  Keyboard,
  Lock,
  PencilLine,
  Plus,
  Repeat2,
  Search,
  Replace,
  Scissors,
  Flag,
  Combine,
  Settings2,
  Tag,
  X,
} from "lucide-react";
import { fileTargets, lang, type LangCode } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges, incomingFromMaster } from "../merge";
import {
  coreToRuns,
  isTag,
  missingTags,
  plain,
  runsToCore,
  sourceKey,
  wordCount,
  type CoreRun,
  type Run,
  type Segment,
  type SegmentComment,
  type SegmentIssue,
  type IssueCategory,
  type IssueSeverity,
  ISSUE_CATEGORIES,
  ISSUE_SEVERITIES,
  type TmUnit,
} from "../editor/model";
import type { QaIssue, TermHit } from "../editor/match";
import { buildIndex, recognizeTerms } from "../termio";
import {
  bandOf,
  scoreDescription,
  scoreLabel,
  scoredLookup,
  type ScoredMatch,
} from "../editor/analysis";
import {
  concordanceAll,
  fromMem,
  openRefs,
  persistMemory,
  storeConfirmed,
  unitOf,
} from "../editor/memory";
import { dictionaryLoaded, ensureDictionary } from "../editor/spelling";
import {
  analyzeFiles,
  checkSegment,
  memoryRefs,
  projectMemoryOf,
  type MemoryRef,
} from "../project";
import { go } from "../App";
import { Preview } from "../editor/Preview";
import { countIn, replaceInRuns, searchPattern } from "../editor/replace";
import { Modal } from "./TermbaseDialogs";
import { AnalysisTable } from "../components/AnalysisTable";
import {
  core,
  download,
  loadCore,
  documentFormat,
  DOCUMENT_TYPE,
  mergeDocx,
  mergeXliff,
  reviewCsv,
  reviewOf,
  type Quality,
  keepsItsSegments,
  returnPackageName,
  returnSdlPackage,
  SDL_RETURN_TYPE,
  tm,
  XLIFF_TYPE,
  type CoreApi,
} from "../core";
import {
  TargetEditor,
  chipText,
  read as readRuns,
  render as renderRuns,
  tagDescription,
  type TargetEditorHandle,
} from "../editor/TargetEditor";
import { InLang, LangChip, LocationBadge, StatusBadge } from "../components/ui";
import { fmtDate, fmtNum, plural } from "../format";
import { usePresence, useResegment, useSegmentComments, useSegmentIssues, useServerConcordance, useTranslationServices } from "../remote";

/** Up to two letters of a name, for presence markers. */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return ((parts[0]?.[0] ?? "") + (parts.length > 1 ? (parts[parts.length - 1]![0] ?? "") : "")).toUpperCase() || "?";
}

const STATUS_TEXT: Record<Segment["status"], string> = {
  new: "Not translated",
  draft: "Draft",
  confirmed: "Confirmed",
};

type Filter = "all" | "todo" | "warnings";

/** Text as a search compares it: without case, and with letters in one form. */
export const foldForSearch = (t: string) => t.normalize("NFKC").toLocaleLowerCase();
type Result = { kind: "tm"; m: ScoredMatch } | { kind: "term"; h: TermHit };

export function Editor({
  projectId,
  fileId,
  lang: wantedLang,
}: {
  projectId: string;
  fileId?: string;
  lang?: LangCode;
}) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const project = state.projects.find((p) => p.id === projectId);
  // The file and language the address names, else the first file to translate.
  const workable = project
    ? project.files.filter(
        (f) => f.role === "translatable" && Object.keys(f.segments).length > 0,
      )
    : [];
  const file = workable.find((f) => f.id === fileId) ?? workable[0];
  const targets = file && project ? fileTargets(file, project) : [];
  const target =
    wantedLang && targets.includes(wantedLang)
      ? wantedLang
      : (targets[0] ?? project?.targetLangs[0] ?? "en-GB");
  const at = useMemo(
    () => ({ projectId, fileId: file?.id ?? "", lang: target }),
    [projectId, file?.id, target],
  );
  const segments = useMemo(
    () => file?.segments[target] ?? [],
    [file?.segments, target],
  );
  const [activeId, setActiveId] = useState(
    () =>
      segments.find((s) => s.status !== "confirmed" && !s.locked)?.id ??
      segments[0]?.id ??
      "",
  );
  const [filter, setFilter] = useState<Filter>("all");
  // Text the segments shown must hold, in the source or the translation.
  const [find, setFind] = useState("");
  const [replacing, setReplacing] = useState(false);
  // Ctrl+H opens find and replace, wherever the keyboard is on the page.
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && e.code === "KeyH") {
        e.preventDefault();
        setReplacing(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const [capture, setCapture] = useState<{
    source: string;
    target: string;
  } | null>(null);
  const [lookupText, setLookupText] = useState<string | null>(null);
  const [selected, setSelected] = useState(0);
  const [showPreview, setShowPreview] = useState(false);
  const [saving, setSaving] = useState(false);
  const analysisDialog = useRef<HTMLDialogElement>(null);
  const memoryDialog = useRef<HTMLDialogElement>(null);
  const tmxInput = useRef<HTMLInputElement>(null);
  const [announcement, setAnnouncement] = useState("");
  const editor = useRef<TargetEditorHandle>(null);
  // A locked segment has no editor; its read-only view takes the focus so
  // the keyboard still moves between segments.
  const lockedView = useRef<HTMLDivElement>(null);
  const sourceSelection = useRef("");

  const source = project?.sourceLang ?? "de-DE";
  const termbases = useMemo(() => {
    if (!project) return [];
    const ids = [
      ...project.setup.masters,
      ...(project.setup.projectTermbaseId
        ? [project.setup.projectTermbaseId]
        : []),
    ];
    return ids
      .map((id) => state.termbases.find((t) => t.id === id)!)
      .filter(Boolean);
  }, [project, state.termbases]);
  const writeTermbase = project
    ? state.termbases.find(
        (t) =>
          t.id ===
          (project.setup.mode === "direct"
            ? (project.setup.writeTo ?? project.setup.masters[0])
            : project.setup.projectTermbaseId),
      )
    : undefined;

  // The memories this language of the project uses, in the core: the
  // project memory first, then the chosen ones, each with its penalty.
  const [ready, setReady] = useState(() => core() !== null);
  const [memVersion, setMemVersion] = useState(0);
  useEffect(() => {
    if (ready) return;
    let live = true;
    loadCore().then(
      () => live && setReady(true),
      (e: unknown) =>
        live &&
        notify(
          `The translation memory could not be loaded: ${e instanceof Error ? e.message : String(e)}`,
        ),
    );
    return () => {
      live = false;
    };
  }, [ready, notify]);
  const c = ready ? core() : null;
  const refs = useMemo(() => {
    if (!c || !project)
      return { lookup: [], concordance: [], update: [] } as Record<
        "lookup" | "concordance" | "update",
        MemoryRef[]
      >;
    const open = (u: "lookup" | "concordance" | "update") =>
      openRefs(
        c,
        memoryRefs(project, target, u, state.memories),
        state.memories,
        state.projects,
      );
    return {
      lookup: open("lookup"),
      concordance: open("concordance"),
      update: open("update"),
    };
    // Opening needs the project's memory settings, not its segments.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    c,
    project?.id,
    project?.memories,
    project?.projectMemory,
    target,
    state.memories,
  ]);
  const memories = refs.lookup;

  // A layered project set to take in master changes does so when it opens.
  useEffect(() => {
    if (
      !project ||
      project.setup.mode !== "layered" ||
      !project.setup.autoUpdate
    )
      return;
    const since = state.branchedAt[project.id] ?? project.createdAt;
    const incoming = project.setup.masters.reduce((n, mid) => {
      const inc = incomingFromMaster(
        state.entries.filter((e) => e.termbaseId === mid),
        since,
      );
      return n + inc.created.length + inc.changed.length;
    }, 0);
    if (!incoming) return;
    dispatch({
      type: "updateFromMaster",
      projectId: project.id,
      now: new Date().toISOString(),
    });
    notify(
      `${w("updated")}: ${plural(incoming, "master change", "master changes")} taken in`,
    );
    // Once per opening of the editor.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project?.id]);

  const active = segments.find((s) => s.id === activeId);
  const index = segments.findIndex((s) => s.id === activeId);
  // On a team server: who else has which segment open.
  const present = usePresence(project, file?.id ?? "", target, activeId);
  const commenting = useSegmentComments(project, file?.id ?? "", target);
  // Scored review: the issues a reviewer marks, and the file's score from them.
  const reviewing = useSegmentIssues(project, file?.id ?? "", target);
  const [quality, setQuality] = useState<Quality | null>(null);
  const issuesKey = segments.map((x) => (x.issues ?? []).map((i) => i.id + i.severity).join(",")).join("|");
  useEffect(() => {
    let live = true;
    if (!file) return;
    void reviewOf([{ name: file.name, lang: target, segments }]).then((r) => live && setQuality(r.total));
    return () => {
      live = false;
    };
    // The score follows the issues, not every keystroke in a translation.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [file?.id, target, issuesKey, segments.length]);
  async function saveReview() {
    if (!project) return;
    const files = project.files
      .filter((f) => f.role === "translatable")
      .flatMap((f) => Object.entries(f.segments).map(([l, segs]) => ({ name: f.name, lang: l, segments: segs })));
    const name = `${project.name} review.csv`;
    download(new TextEncoder().encode(await reviewCsv(files)), name, "text/csv");
    notify(`Saved ${name}`);
  }
  // Splitting and joining: in documents and in plain XLIFF files. Files of
  // Trados and memoQ keep their segments, as those programs keep their
  // own record of each.
  const cut = useResegment(project, file?.id ?? "", target);
  const [splitting, setSplitting] = useState(false);
  const [kept, setKept] = useState(false);
  useEffect(() => {
    let live = true;
    setKept(false);
    if (file?.kind !== "xliff") return;
    if (file.packagePath) setKept(true);
    else if (file.bytes) void keepsItsSegments(file.bytes).then((yes) => live && setKept(yes));
    return () => {
      live = false;
    };
  }, [file?.id, file?.kind, file?.bytes, file?.packagePath]);
  const after = segments[index + 1];
  const cuttable = file?.kind === "docx" || file?.kind === "xliff";
  const fixed = kept || !!active?.file?.fixed;
  const cuts = cuttable && !fixed && !!active && !active.locked;
  const joins =
    cuts && !!after && !after.locked && !after.file?.fixed &&
    ((!!active.split && active.split.unit === after.split?.unit) || (!!active.block && active.block === after.block));
  // A segment that was split or joined has another name: the one in its place is the open one.
  const place = useRef(0);
  useEffect(() => {
    if (index >= 0) place.current = index;
    else if (activeId && segments.length) setActiveId(segments[Math.min(place.current, segments.length - 1)]!.id);
  }, [index, activeId, segments]);
  async function join() {
    if (!active) return;
    const no = await cut("join", active.id);
    if (no) notify(no);
    setAnnouncement(no ?? "Joined with the next segment");
    editor.current?.focus();
  }
  const services = useTranslationServices(project, target);
  const here = useMemo(() => {
    const m = new Map<string, typeof present>();
    for (const x of present) m.set(x.segment, [...(m.get(x.segment) ?? []), x]);
    return m;
  }, [present]);
  // Term recognition runs in the core; the index is rebuilt when an entry changes.
  const termIndex = useMemo(() => {
    if (!c || !project) return null;
    const name = `editor:${project.id}`;
    buildIndex(c, name, termbases, state.entries, state.removals);
    // A new object each rebuild, so hits and checks recompute.
    return { name };
  }, [c, project, termbases, state.entries, state.removals]);
  const hits = useMemo((): TermHit[] => {
    if (!active || !c || !termIndex) return [];
    const out: TermHit[] = [];
    for (const h of recognizeTerms(
      c,
      termIndex.name,
      plain(active.source),
      source,
      target,
    )) {
      const entry = state.entries.find((e) => e.id === h.entryId);
      const tb = termbases.find((t) => t.id === h.source);
      if (!entry || !tb) continue;
      const own = (code: string) =>
        Object.entries(entry.terms)
          .filter(([l]) => l === code || primaryOf(l) === primaryOf(code))
          .flatMap(([, ts]) => ts);
      const term = own(source).find((t) => t.text === h.term.text) ?? {
        text: h.term.text,
        status: "approved" as const,
      };
      const targets = h.targets.map(
        (ct) =>
          own(target).find((t) => t.text === ct.text) ?? {
            text: ct.text,
            status: (ct.status as TermHit["term"]["status"]) || "approved",
          },
      );
      out.push({
        entry,
        termbase: tb,
        term,
        start: h.start,
        end: h.end,
        targets,
      });
    }
    return out;
  }, [active, c, termIndex, source, target, state.entries, termbases]);
  const context = project?.context ?? state.contextRule;
  const matches = useMemo(
    () =>
      active && c
        ? scoredLookup(c, memories, segments, index, context, 70, 3)
        : [],
    // memVersion: the memories gained units.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [active, c, memories, index, segments, context, memVersion],
  );
  // The core's checks, as the project switches them.
  // The spelling dictionary of this language, loaded into the core once.
  const dictName = project?.qa.dictionaries[target] ?? "";
  const [dictReady, setDictReady] = useState("");
  useEffect(() => {
    if (!c || !dictName) return;
    if (dictionaryLoaded(dictName)) {
      setDictReady(dictName);
      return;
    }
    let live = true;
    ensureDictionary(c, dictName).then(
      (ok) => {
        if (!live) return;
        if (ok) setDictReady(dictName);
        else
          notify(
            `The dictionary ${dictName} is no longer saved in this browser, so spelling is not checked. Add it again under Settings.`,
          );
      },
      (e: unknown) =>
        live &&
        notify(
          `The dictionary ${dictName} could not be loaded: ${e instanceof Error ? e.message : String(e)}`,
        ),
    );
    return () => {
      live = false;
    };
  }, [c, dictName, notify]);
  const issues = useMemo((): QaIssue[] => {
    if (!active || !c || !project || !plain(active.target).trim()) return [];
    return checkSegment(
      c,
      segments,
      active.id,
      source,
      target,
      project.qa,
      termIndex?.name,
      dictReady === dictName ? dictName : "",
    ).map((i) => ({
      kind: i.kind,
      found: i.found,
      text:
        i.kind === "tags-missing"
          ? `${i.message} Press F9 to insert the next one.`
          : i.kind === "spelling" && i.suggestions?.length
            ? `${i.message} Perhaps ${i.suggestions.slice(0, 3).join(", ")}.`
            : i.message,
    }));
  }, [
    active,
    segments,
    c,
    project,
    termIndex,
    source,
    target,
    dictReady,
    dictName,
  ]);
  const allIssues = useMemo<QaIssue[]>(() => {
    if (!active) return [];
    const extra: QaIssue[] = [];
    // Unedited fuzzy match: a common check in memoQ, Phrase and Trados Verify.
    if (
      project?.qa.uneditedFuzzy &&
      active.origin?.kind === "tm" &&
      active.origin.score < 100 &&
      plain(active.target) === active.origin.inserted
    ) {
      extra.push({
        kind: "unedited",
        text: `The ${active.origin.score}% match is unchanged. Check what differs from the memory source.`,
      });
    }
    const ignored = new Set(active.ignored ?? []);
    return [...issues, ...extra].filter((q) => !ignored.has(q.text));
  }, [active, issues, project?.qa.uneditedFuzzy]);

  // The numbered results list: memory matches first, then terms (as in memoQ).
  const results: Result[] = [
    ...matches.map((m) => ({ kind: "tm" as const, m })),
    ...hits
      .filter((h) => h.targets.some((t) => t.status !== "forbidden"))
      .map((h) => ({ kind: "term" as const, h })),
  ];
  // What may complete a word being typed: the terms found for the segment,
  // then the longer words of its memory matches.
  const completions = useMemo(() => {
    const out: string[] = [];
    for (const h of hits) for (const t of h.targets) if (t.status !== "forbidden") out.push(t.text);
    for (const m of matches) for (const w of plain(m.unit.target).match(/[\p{L}\p{N}][\p{L}\p{N}\p{M}'’-]{4,}/gu) ?? []) out.push(w);
    return out;
  }, [hits, matches]);
  // A server project's concordance runs on the server.
  const serverHits = useServerConcordance(project, target, lookupText ?? "", 8);
  const concordanceHits = useMemo(() => {
    if (!lookupText || lookupText.trim().length < 2 || !c) return [];
    const q = lookupText.trim().toLocaleLowerCase();
    const found = serverHits
      ? serverHits.flatMap((h) => {
          const ref = refs.concordance.find((r) => r.name === h.memory);
          return ref ? [{ unit: fromMem({ ...h.unit, source: runsToCore(h.unit.source), target: runsToCore(h.unit.target) }, ref), side: h.side }] : [];
        })
      : concordanceAll(c, refs.concordance, lookupText, 8);
    return found.map((h) => {
      const text = plain(h.side === "source" ? h.unit.source : h.unit.target);
      const at = text.toLocaleLowerCase().indexOf(q);
      return { ...h, start: Math.max(0, at), end: at >= 0 ? at + q.length : 0 };
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookupText, refs.concordance, c, memVersion, serverHits]);

  // Track a selection made in the active source text, for "Add term" and concordance.
  useEffect(() => {
    const on = () => {
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed) return;
      const node =
        sel.anchorNode instanceof Element
          ? sel.anchorNode
          : sel.anchorNode?.parentElement;
      if (node?.closest(".seg.is-active .seg-source"))
        sourceSelection.current = sel.toString().trim();
    };
    document.addEventListener("selectionchange", on);
    return () => document.removeEventListener("selectionchange", on);
  }, []);

  // When a segment opens: insert the best match into an empty target (Trados'
  // default), then tell screen readers where they are.
  useEffect(() => {
    if (!active) return;
    if (active.locked) lockedView.current?.focus();
    setSelected(0);
    sourceSelection.current = "";
    const parts = [
      `Segment ${index + 1} of ${segments.length}`,
      STATUS_TEXT[active.status],
    ];
    const best = matches[0];
    if (active.locked) parts.push(active.lockedReason ?? "Locked in the file");
    if (
      state.autoInsert &&
      best &&
      !active.locked &&
      active.status === "new" &&
      !plain(active.target).trim()
    ) {
      insertMatch(best);
      parts.push(`${scoreDescription(best.score, best.context)} inserted`);
    } else if (best) {
      parts.push(
        `${plural(matches.length, "memory match", "memory matches")}, best ${scoreLabel(best.score, best.context)}`,
      );
    }
    if (hits.length) parts.push(plural(hits.length, "term", "terms"));
    if (active.warnings?.length)
      parts.push(plural(active.warnings.length, "warning", "warnings"));
    setAnnouncement(parts.join(". ") + ".");
  }, [activeId, ready]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!project) {
    return (
      <div className="page page-narrow">
        <div className="empty">
          <p className="empty-title">Project not found</p>
          <a className="btn btn-small" href="#/projects">
            Back to projects
          </a>
        </div>
      </div>
    );
  }
  const projectMemory = projectMemoryOf(project, target, state.memories);
  const analysisMemories = memories.filter((m) => !m.project);

  const sought = foldForSearch(find.trim());
  const visible = segments.filter(
    (s) =>
      s.id === activeId ||
      ((filter === "all" || (filter === "todo" ? s.status !== "confirmed" : (s.warnings?.length ?? 0) > 0)) &&
        (!sought || foldForSearch(plain(s.source)).includes(sought) || foldForSearch(plain(s.target)).includes(sought))),
  );
  const found = sought ? segments.filter((s) => foldForSearch(plain(s.source)).includes(sought) || foldForSearch(plain(s.target)).includes(sought)).length : 0;
  const withWarnings = segments.filter((s) => s.warnings?.length).length;
  const totalWords = segments.reduce((n, s) => n + wordCount(s.source), 0);
  const doneWords = segments
    .filter((s) => s.status === "confirmed")
    .reduce((n, s) => n + wordCount(s.source), 0);
  const pendingChanges =
    project.setup.mode === "layered" && project.setup.projectTermbaseId
      ? project.setup.masters.reduce((n, mid) => {
          const m = state.termbases.find((t) => t.id === mid)!;
          return (
            n +
            computeChanges(
              state.entries.filter(
                (e) => e.termbaseId === project.setup.projectTermbaseId,
              ),
              m,
              state.entries.filter((e) => e.termbaseId === mid),
              state.baseSnapshots,
              state.branchedAt[project.id] ?? project.createdAt,
            ).length
          );
        }, 0)
      : 0;

  function setTarget(
    runs: Run[],
    status: Segment["status"] = "draft",
    origin?: Segment["origin"],
  ) {
    if (!active || active.locked) return;
    const empty = !plain(runs).trim() && !runs.some(isTag);
    dispatch({
      type: "setTarget",
      at,
      segmentId: active.id,
      target: runs,
      status: empty ? "new" : status,
      origin,
    });
  }

  function activate(id: string) {
    setActiveId(id);
    setCapture(null);
  }

  function move(delta: number) {
    const next = segments[index + delta];
    if (next) activate(next.id);
  }

  function confirmAndNext() {
    if (!active) return;
    if (active.locked) {
      setAnnouncement(
        active.lockedReason
          ? `${active.lockedReason}: this segment is not yours to change.`
          : "This segment is locked in the file and cannot be changed.",
      );
      return;
    }
    if (!plain(active.target).trim()) {
      setAnnouncement(
        "The target is empty. Type a translation or insert a match first.",
      );
      return;
    }
    const key = sourceKey(active.source);
    const repetitions = state.autoPropagate
      ? segments.filter(
          (s) =>
            s.id !== active.id &&
            s.status !== "confirmed" &&
            !s.locked &&
            sourceKey(s.source) === key,
        )
      : [];
    const warnings = allIssues.map((q) => ({ kind: q.kind, text: q.text }));
    dispatch({ type: "confirm", at, segmentId: active.id, warnings });
    // Confirmed translations go into the memories the project updates.
    if (c && refs.update.length) {
      const unit = unitOf(
        segments.map((s) => (s.id === active.id ? active : s)),
        index,
        project!.name,
        state.userName,
        new Date().toISOString(),
      );
      void storeConfirmed(c, refs.update, unit).then((kept) => {
        setMemVersion((v) => v + 1);
        if (!kept)
          notify(
            "The memory is kept only until you close this page: this browser does not allow saving",
          );
      });
    }

    const said = [
      warnings.length
        ? `Confirmed with ${plural(warnings.length, "warning", "warnings")}`
        : "Confirmed",
    ];
    if (repetitions.length) {
      said.push(
        `${plural(repetitions.length, "repetition", "repetitions")} filled: segment ${repetitions.map((r) => segments.indexOf(r) + 1).join(", ")}`,
      );
      notify(
        `Also filled ${plural(repetitions.length, "repetition", "repetitions")} (segment ${repetitions.map((r) => segments.indexOf(r) + 1).join(", ")})`,
      );
    }
    const skip = new Set([active.id, ...repetitions.map((r) => r.id)]);
    const open = (s: Segment) =>
      s.status !== "confirmed" && !s.locked && !skip.has(s.id);
    const next = segments.slice(index + 1).find(open) ?? segments.find(open);
    if (next) activate(next.id);
    else {
      notify("All segments confirmed");
      said.push("All segments are confirmed");
    }
    // The next segment's own announcement follows this one.
    setTimeout(() => setAnnouncement((a) => `${said.join(". ")}. ${a}`), 0);
  }

  function insertMatch(m: ScoredMatch) {
    editor.current?.setRuns(m.unit.target);
    setTarget(m.unit.target, "draft", {
      kind: "tm",
      score: m.score,
      inserted: plain(m.unit.target),
      context: m.context,
    });
  }

  async function bytesOf(f: NonNullable<typeof file>): Promise<Uint8Array> {
    if (f.bytes) return f.bytes;
    if (!f.url) throw new Error(`${f.name} has no original file to save into.`);
    return new Uint8Array(
      await (await fetch(new URL(f.url, document.baseURI))).arrayBuffer(),
    );
  }

  async function saveDocument() {
    if (!project || !file) return;
    setSaving(true);
    try {
      const open = segments.filter(
        (s) => s.status !== "confirmed" && !s.locked,
      ).length;
      const pending = open
        ? ` ${plural(open, "segment is", "segments are")} not confirmed yet.`
        : "";
      if (file.kind === "xliff") {
        const pkg = project.sdlPackage;
        if (pkg && file.packagePath) {
          // A return package holds every file of the package, translated.
          const translated: Record<string, Uint8Array> = {};
          for (const f of project.files) {
            if (f.kind !== "xliff" || !f.packagePath || !f.targetLang) continue;
            const segs = f.segments[f.targetLang] ?? [];
            translated[f.packagePath] = await mergeXliff(
              await bytesOf(f),
              segs,
            );
          }
          const rpx = await returnSdlPackage(pkg.bytes, pkg.name, translated);
          const name = returnPackageName(pkg.name);
          download(rpx, name, SDL_RETURN_TYPE);
          notify(
            `Saved the return package ${name} with ${plural(Object.keys(translated).length, "file", "files")}.${pending}`,
          );
          return;
        }
        // A bilingual file goes back under its own name, for the tool it came from.
        const out = await mergeXliff(await bytesOf(file), segments);
        const name = file.name.split("/").pop() ?? file.name;
        download(out, name, XLIFF_TYPE);
        notify(`Saved ${name}.${pending}`);
        return;
      }
      const out = await mergeDocx(
        await bytesOf(file),
        source,
        target,
        segments,
        file.name,
      );
      const own = file.name.split("/").pop() ?? file.name;
      const dot = own.lastIndexOf(".");
      const name = `${own.slice(0, dot)}_${target}${own.slice(dot)}`;
      download(out, name, DOCUMENT_TYPE[documentFormat(file.name) ?? "docx"]);
      const empty = segments.filter((s) => !plain(s.target).trim()).length;
      notify(
        empty
          ? `Saved ${name}. ${plural(empty, "segment", "segments")} without a translation kept the ${lang(source).name} text.`
          : `Saved ${name}`,
      );
    } catch (e) {
      notify(
        `Could not save the document: ${e instanceof Error ? e.message : String(e)}`,
      );
    } finally {
      setSaving(false);
    }
  }

  function insertTerm(h: TermHit) {
    const t = h.targets.find((x) => x.status !== "forbidden");
    if (t) editor.current?.insertText(t.text);
  }

  function insertResult(r: Result | undefined) {
    if (!r) return;
    if (r.kind === "tm") insertMatch(r.m);
    else insertTerm(r.h);
  }

  function insertNextTag() {
    if (!active) return;
    const next = missingTags(active.source, active.target)[0];
    if (next) editor.current?.insertTag(next);
    else setAnnouncement("All tags are already in the target.");
  }

  function openCapture() {
    setLookupText(null);
    setCapture({
      source: sourceSelection.current,
      target: editor.current?.selectedText() ?? "",
    });
  }

  function openLookup() {
    setCapture(null);
    setLookupText(
      editor.current?.selectedText() || sourceSelection.current || "",
    );
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const mod = e.ctrlKey || e.metaKey;
    const digit = /^Digit([1-9])$/.exec(e.code);
    if (mod && e.key === "Enter") {
      e.preventDefault();
      confirmAndNext();
    } else if (e.altKey && e.key === "ArrowDown") {
      e.preventDefault();
      move(1);
    } else if (e.altKey && e.key === "ArrowUp") {
      e.preventDefault();
      move(-1);
    } else if (mod && !e.shiftKey && digit) {
      // Ctrl+1 to 9 inserts a numbered result in Trados, memoQ, Phrase and Smartcat.
      const r = results[Number(digit[1]) - 1];
      if (r) {
        e.preventDefault();
        insertResult(r);
      }
    } else if (mod && e.key === "ArrowDown" && results.length) {
      e.preventDefault();
      setSelected((i) => Math.min(i + 1, results.length - 1));
    } else if (mod && e.key === "ArrowUp" && results.length) {
      e.preventDefault();
      setSelected((i) => Math.max(i - 1, 0));
    } else if (mod && e.code === "Space" && results.length) {
      e.preventDefault();
      insertResult(results[Math.min(selected, results.length - 1)]);
    } else if (mod && e.key === "Insert") {
      e.preventDefault();
      if (active) editor.current?.setRuns(active.source);
    } else if (mod && !e.shiftKey && e.code === "KeyE") {
      e.preventDefault();
      openCapture();
    } else if ((mod && !e.shiftKey && e.code === "KeyK") || e.key === "F3") {
      e.preventDefault();
      openLookup();
    } else if (e.key === "F9") {
      e.preventDefault();
      insertNextTag();    } else if (e.altKey && e.shiftKey && e.code === "KeyS" && cuts) {
      e.preventDefault();
      setSplitting(true);
    } else if (e.altKey && e.shiftKey && e.code === "KeyJ" && joins) {
      e.preventDefault();
      void join();
    }
  }

  // Announce the highlighted result when it moves with Ctrl+arrows.
  const highlighted = results[Math.min(selected, results.length - 1)];

  return (
    <div className="editor">
      <header className="editor-bar">
        <a className="btn btn-quiet btn-small" href="#/projects">
          <ArrowLeft size={14} aria-hidden="true" />
          Projects
        </a>
        <div className="editor-title">
          <h1 id="page-title" tabIndex={-1}>
            {project.name}
          </h1>
          <span className="editor-meta">
            {workable.length > 1 ? (
              <>
                <label htmlFor="ed-file" className="sr-only">
                  File
                </label>
                <select
                  id="ed-file"
                  className="select-small"
                  value={file?.id ?? ""}
                  onChange={(e) =>
                    go({
                      page: "editor",
                      projectId,
                      fileId: e.target.value,
                      lang: target,
                    })
                  }
                >
                  {workable.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name}
                    </option>
                  ))}
                </select>
              </>
            ) : (
              <span>{file?.name ?? project.files[0]?.name}</span>
            )}
            <span className="lang-pair">
              <LangChip code={source} /> →{" "}
              {targets.length > 1 ? (
                <>
                  <label htmlFor="ed-lang" className="sr-only">
                    Target language
                  </label>
                  <select
                    id="ed-lang"
                    className="select-small"
                    value={target}
                    onChange={(e) =>
                      go({
                        page: "editor",
                        projectId,
                        fileId: file?.id,
                        lang: e.target.value,
                      })
                    }
                  >
                    {targets.map((t) => (
                      <option key={t} value={t}>
                        {lang(t).name}
                      </option>
                    ))}
                  </select>
                </>
              ) : (
                <LangChip code={target} />
              )}
            </span>
          </span>
        </div>
        <div
          className="editor-progress"
          aria-label={`${Math.round((doneWords / (totalWords || 1)) * 100)} percent confirmed`}
        >
          <span className="progress-track" aria-hidden="true">
            <span
              className="progress-fill"
              style={{ width: `${(doneWords / (totalWords || 1)) * 100}%` }}
            />
          </span>
          <span className="progress-text">
            {plural(doneWords, "word", "words")} of{" "}
            {totalWords.toLocaleString("en-GB")} confirmed
          </span>
        </div>
        <div
          className="segmented segmented-small"
          role="radiogroup"
          aria-label="Show segments"
        >
          <label className="segment">
            <input
              type="radio"
              name="segfilter"
              checked={filter === "all"}
              onChange={() => setFilter("all")}
            />
            <span>All</span>
          </label>
          <label className="segment">
            <input
              type="radio"
              name="segfilter"
              checked={filter === "todo"}
              onChange={() => setFilter("todo")}
            />
            <span>To do</span>
          </label>
          <label className="segment">
            <input
              type="radio"
              name="segfilter"
              checked={filter === "warnings"}
              onChange={() => setFilter("warnings")}
            />
            <span>Warnings{withWarnings > 0 && ` (${withWarnings})`}</span>
          </label>
        </div>
        <label className="editor-find">
          <span className="sr-only">Find in the segments</span>
          <input type="search" placeholder="Find in the segments" value={find} onChange={(e) => setFind(e.target.value)} />
          {sought && (
            <span className="cell-sub" role="status">
              {found === 0 ? "Nothing found" : plural(found, "segment", "segments")}
            </span>
          )}
        </label>
        <div className="editor-actions">
          {pendingChanges > 0 && (
            <a className="btn btn-small" href={`#/merge/${project.id}`}>
              <GitMerge size={14} aria-hidden="true" />
              {w("reviewMerge")} ({pendingChanges})
            </a>
          )}
          <a className="btn btn-small" href={`#/project/${project.id}`}>
            <Settings2 size={14} aria-hidden="true" />
            Project settings
          </a>
          <button type="button" className="btn btn-small" onClick={() => setReplacing(true)} disabled={segments.length === 0}>
            <Replace size={14} aria-hidden="true" />
            Replace
          </button>
          <button
            type="button"
            className="btn btn-small"
            onClick={() => memoryDialog.current?.showModal()}
            disabled={!c}
          >
            <Database size={14} aria-hidden="true" />
            Memories
          </button>
          <button
            type="button"
            className="btn btn-small"
            onClick={() => analysisDialog.current?.showModal()}
            disabled={segments.length === 0}
          >
            <BarChart3 size={14} aria-hidden="true" />
            Analysis
          </button>
          {file?.blocks && file.kind === "docx" && (
            <button
              type="button"
              className="btn btn-small"
              aria-pressed={showPreview}
              onClick={() => setShowPreview((v) => !v)}
            >
              <Eye size={14} aria-hidden="true" />
              Preview
            </button>
          )}
          {file &&
            (file.bytes || file.url) &&
            (file.kind === "docx" || file.kind === "xliff") && (
              <button
                type="button"
                className="btn btn-small"
                onClick={saveDocument}
                disabled={saving}
              >
                <Download size={14} aria-hidden="true" />
                {saving
                  ? "Saving…"
                  : project.sdlPackage && file.packagePath
                    ? "Save return package"
                    : file.kind === "xliff"
                      ? "Save XLIFF"
                      : "Save translated document"}
              </button>
            )}
        </div>
      </header>

      {splitting && active && (
        <SplitDialog
          segment={active}
          lang={source}
          onClose={() => {
            setSplitting(false);
            editor.current?.focus();
          }}
          onSplit={async (before) => {
            const no = await cut("split", active.id, before);
            if (no) return no;
            setSplitting(false);
            setAnnouncement("Split in two segments");
            return undefined;
          }}
        />
      )}
      {replacing && (
        <ReplaceDialog
          segments={segments}
          lang={target}
          initial={find.trim()}
          onClose={() => setReplacing(false)}
          onReplace={(changes, places) => {
            dispatch({ type: "replaceTargets", at, changes });
            setReplacing(false);
            const said = `Replaced ${plural(places, "place", "places")} in ${plural(changes.length, "segment", "segments")}`;
            notify(said);
            setAnnouncement(said);
          }}
        />
      )}
      <dialog
        ref={memoryDialog}
        className="dialog"
        aria-labelledby="memory-title"
      >
        <form method="dialog" className="dialog-body">
          <h2 id="memory-title" className="dialog-title">
            Translation memories
          </h2>
          <p className="help">
            Memories for {lang(target).name}, highest priority first. Matches
            are scored in the core, with each memory's penalty taken off.{" "}
            <a href={`#/project/${project.id}`}>
              Change them in the project settings
            </a>
            .
          </p>
          {c && (
            <table className="table">
              <caption className="sr-only">Memories of this project</caption>
              <thead>
                <tr>
                  <th scope="col">Memory</th>
                  <th scope="col">Used for</th>
                  <th scope="col" className="num">
                    Units
                  </th>
                  <th scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {[
                  ...new Set(
                    [...refs.lookup, ...refs.concordance, ...refs.update].map(
                      (m) => m.name,
                    ),
                  ),
                ].map((name) => {
                  const look = refs.lookup.find((m) => m.name === name);
                  const uses = [
                    look
                      ? `lookup${look.penalty ? ` (penalty ${look.penalty})` : ""}`
                      : "",
                    refs.concordance.some((m) => m.name === name)
                      ? "concordance"
                      : "",
                    refs.update.some((m) => m.name === name)
                      ? "updated on confirm"
                      : "",
                  ].filter(Boolean);
                  const isProject = projectMemory?.name === name;
                  return (
                    <tr key={name}>
                      <th scope="row" className="cell-title">
                        {name}
                        {isProject && (
                          <span className="muted"> (project memory)</span>
                        )}
                      </th>
                      <td>{uses.join(", ")}</td>
                      <td className="num">{fmtNum(tm.count(c, name))}</td>
                      <td className="row">
                        {isProject && !project?.remote && (
                          <button
                            type="button"
                            className="btn btn-small"
                            onClick={() => tmxInput.current?.click()}
                          >
                            <Upload size={14} aria-hidden="true" />
                            Import TMX
                          </button>
                        )}
                        <button
                          type="button"
                          className="btn btn-quiet btn-small"
                          onClick={() =>
                            download(
                              tm.exportTMX(c, name),
                              `${name.replace(/[^\p{L}\p{N}]+/gu, "_")}.tmx`,
                              "application/x-tmx+xml",
                            )
                          }
                          disabled={tm.count(c, name) === 0}
                        >
                          <Download size={14} aria-hidden="true" />
                          Export TMX
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {c &&
            refs.lookup.length +
              refs.concordance.length +
              refs.update.length ===
              0 && (
              <p className="muted">
                This language of the project uses no memory, so the editor shows
                no matches.
              </p>
            )}
          <input
            ref={tmxInput}
            type="file"
            accept=".tmx,application/x-tmx+xml,text/xml"
            hidden
            onChange={async (e) => {
              const f = e.target.files?.[0];
              e.target.value = "";
              if (!f || !c || !projectMemory) return;
              try {
                const st = tm.importTMX(
                  c,
                  projectMemory.name,
                  new Uint8Array(await f.arrayBuffer()),
                );
                setMemVersion((v) => v + 1);
                const kept = await persistMemory(c, projectMemory.name);
                notify(
                  `Imported ${plural(st.stored, "unit", "units")} from ${f.name}${st.skipped ? `; ${st.skipped} without both languages skipped` : ""}${kept ? "" : ". It is kept only until you close this page: this browser does not allow saving"}`,
                );
              } catch (err) {
                notify(
                  `Could not import ${f.name}: ${err instanceof Error ? err.message : String(err)}`,
                );
              }
            }}
          />
          <div className="row">
            <button className="btn btn-primary btn-small">Close</button>
          </div>
        </form>
      </dialog>

      <dialog
        ref={analysisDialog}
        className="dialog"
        aria-labelledby="analysis-title"
      >
        <form method="dialog" className="dialog-body">
          <h2 id="analysis-title" className="dialog-title">
            Analysis
          </h2>
          <p className="help">
            {plural(
              project.files.filter((f) => f.segments[target]).length,
              "file",
              "files",
            )}{" "}
            in {lang(target).name} against{" "}
            {analysisMemories.map((m) => m.name).join(" and ") || "no memory"},
            counted in this order: context matches first, then repetitions within
            a file and across files, then the best match. Below 50% counts as
            new. The project memory is left out: it holds this project's own
            translations.
          </p>
          {c ? (
            <DialogAnalysis
              c={c}
              project={project}
              target={target}
              memories={analysisMemories}
            />
          ) : (
            <p className="muted">Loading the memory…</p>
          )}
          <div className="row">
            <button className="btn btn-primary btn-small">Close</button>
          </div>
        </form>
      </dialog>

      <p className="sr-only" role="status" aria-live="polite">
        {announcement}
      </p>

      {segments.length === 0 ? (
        <div className="empty">
          <p className="empty-title">No text to translate</p>
          <p>
            The document has no readable text here (for example a scan, or a
            format only the full product imports). Text recognition for scans
            comes with the file filters.
          </p>
        </div>
      ) : (
        <div
          className={`editor-body${showPreview && file?.blocks && file.kind === "docx" ? " with-preview" : ""}`}
        >
          <ol className="segments" aria-label="Segments">
            {visible.map((s) => {
              const n = segments.indexOf(s) + 1;
              const isActive = s.id === activeId;
              return (
                <li
                  key={s.id}
                  className={`seg seg-${s.status}${isActive ? " is-active" : ""}${s.locked ? " is-locked" : ""}`}
                  aria-current={isActive ? "true" : undefined}
                >
                  <span className="seg-num">
                    {n}
                    {(here.get(s.id) ?? []).map((x) => (
                      <span
                        key={x.userId}
                        className="seg-presence"
                        title={`${x.userName} has this segment open`}
                        aria-label={`${x.userName} has this segment open`}
                      >
                        {initials(x.userName)}
                      </span>
                    ))}
                  </span>
                  <div
                    className="seg-source"
                    lang={source}
                    onClick={() => !isActive && activate(s.id)}
                  >
                    <SourceText runs={s.source} hits={isActive ? hits : []} />
                  </div>
                  <div className="seg-target">
                    {isActive && s.locked ? (
                      <div
                        className="seg-target-view seg-locked"
                        lang={target}
                        ref={lockedView}
                        tabIndex={0}
                        onKeyDown={onKeyDown}
                        aria-label={`Segment ${n}: ${plain(s.target) || "not translated"}. ${s.lockedReason ?? "Locked in the file"}`}
                      >
                        <span className="seg-target-text">
                          {s.target.length ? (
                            <RunsView runs={s.target} />
                          ) : (
                            <span className="missing">Not translated</span>
                          )}
                        </span>
                        <span className="seg-locked-note">
                          <Lock size={13} aria-hidden="true" />{" "}
                          {s.lockedReason ?? "Locked in the file"}
                        </span>
                      </div>
                    ) : isActive ? (
                      <>
                        <TargetEditor
                          key={`${s.id}:${s.rev ?? 0}`}
                          ref={editor}
                          initial={s.target}
                          lang={target}
                          label={`Translation of segment ${n}, ${lang(target).name}`}
                          describedBy="seg-help"
                          onChange={(runs) => setTarget(runs)}
                          onKeyDown={onKeyDown}
                          completions={state.autoComplete ? completions : undefined}
                        />
                        {s.maxLength ? (
                          <LengthCounter
                            length={[...plain(s.target)].length}
                            max={s.maxLength}
                          />
                        ) : null}
                      </>
                    ) : (
                      <button
                        type="button"
                        className="seg-target-view"
                        lang={target}
                        onClick={() => activate(s.id)}
                        aria-label={`Segment ${n}: ${plain(s.target) || "not translated"}. ${STATUS_TEXT[s.status]}${s.warnings?.length ? `, ${plural(s.warnings.length, "warning", "warnings")}` : ""}. Edit`}
                      >
                        <span className="seg-target-text">
                          {s.target.length ? (
                            <RunsView runs={s.target} />
                          ) : (
                            <span className="missing">Not translated</span>
                          )}
                        </span>
                      </button>
                    )}
                  </div>
                  <SegmentStatusCell s={s} />
                </li>
              );
            })}
          </ol>

          {showPreview && file?.blocks && file.kind === "docx" && (
            <section
              className="preview-pane"
              aria-label="Preview of the translated document"
            >
              <p className="preview-note">
                Layout from the document. Untranslated text appears in the
                source language. The saved file is exact.
              </p>
              <Preview
                blocks={file.blocks}
                segments={segments}
                activeId={activeId}
                sourceLang={source}
                targetLang={target}
                onPick={activate}
              />
            </section>
          )}

          <aside className="results" aria-label="Translation results">
            <p id="seg-help" className="sr-only">
              Control Enter confirms and moves on. Control and a number inserts
              a numbered result. Control E adds a term. Control K searches the
              memory. F9 inserts the next tag.
            </p>
            {active && (
              <>
                <Section title="Translation results" count={results.length}>
                  {results.length === 0 ? (
                    <p className="muted">
                      No memory matches or terms for this segment.
                    </p>
                  ) : (
                    <ol className="result-list">
                      {results.map((r, i) => (
                        <li
                          key={
                            r.kind === "tm" ? `tm-${i}` : `t-${r.h.entry.id}`
                          }
                        >
                          <button
                            type="button"
                            className={`result${r === highlighted ? " is-selected" : ""}`}
                            onClick={() => insertResult(r)}
                            aria-keyshortcuts={
                              i < 9 ? `Control+${i + 1}` : undefined
                            }
                          >
                            <span className="result-key" aria-hidden="true">
                              {i + 1}
                            </span>
                            {r.kind === "tm" ? (
                              <MatchBody
                                m={r.m}
                                source={source}
                                target={target}
                              />
                            ) : (
                              <TermBody
                                h={r.h}
                                source={source}
                                target={target}
                              />
                            )}
                          </button>
                        </li>
                      ))}
                    </ol>
                  )}
                  {services.names.length > 0 && !active.locked && (
                    <ServiceSuggestions
                      key={active.id}
                      names={services.names}
                      lang={target}
                      ask={(name) => services.ask(name, active.source)}
                      onInsert={(runs) => editor.current?.setRuns(runs)}
                    />
                  )}
                </Section>

                <Section title="Checks" count={allIssues.length}>
                  {allIssues.length === 0 ? (
                    <p className="muted">
                      {plain(active.target).trim()
                        ? "No problems found."
                        : "Checks run as you type and again when you confirm."}
                    </p>
                  ) : (
                    <ul className="issues">
                      {allIssues.map((q) => (
                        <li
                          key={q.text}
                          className={`issue issue-${q.kind === "term-forbidden" ? "forbidden" : q.kind}`}
                        >
                          <AlertTriangle size={14} aria-hidden="true" />
                          <span className="issue-text">{q.text}</span>
                          {q.kind === "spelling" && q.found && (
                            <button
                              type="button"
                              className="btn btn-quiet btn-tiny"
                              onClick={() => {
                                const qa = {
                                  ...project.qa,
                                  words: [
                                    ...new Set([...project.qa.words, q.found!]),
                                  ],
                                };
                                dispatch({
                                  type: "updateProject",
                                  change: {
                                    project: { ...project, qa },
                                    addLanguages: [],
                                    addMemories: [],
                                    removeMemories: [],
                                  },
                                });
                                notify(
                                  `“${q.found}” is spelled right in this project from now on`,
                                );
                              }}
                              aria-label={`Add ${q.found} to the project's words`}
                            >
                              Add to the project's words
                            </button>
                          )}
                          <button
                            type="button"
                            className="btn btn-quiet btn-tiny"
                            onClick={() =>
                              dispatch({
                                type: "ignoreIssue",
                                at,
                                segmentId: active.id,
                                text: q.text,
                              })
                            }
                            aria-label={`Ignore: ${q.text}`}
                          >
                            Ignore
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  {(active.ignored?.length ?? 0) > 0 && (
                    <p className="help">
                      {plural(active.ignored!.length, "warning", "warnings")}{" "}
                      ignored for this segment.
                    </p>
                  )}
                </Section>

                {lookupText !== null && (
                  <Section title="Memory search">
                    <form
                      className="lookup"
                      role="search"
                      onSubmit={(e) => e.preventDefault()}
                      onKeyDown={(e) => {
                        if (e.key === "Escape") {
                          e.preventDefault();
                          setLookupText(null);
                          editor.current?.focus();
                        }
                      }}
                    >
                      <label htmlFor="lookup-q" className="sr-only">
                        Search the memory
                      </label>
                      <Search size={14} aria-hidden="true" />
                      <input
                        id="lookup-q"
                        autoFocus
                        value={lookupText}
                        onChange={(e) => setLookupText(e.target.value)}
                        placeholder="Words in either language"
                      />
                      <button
                        type="button"
                        className="btn btn-quiet btn-tiny"
                        aria-label="Close memory search"
                        onClick={() => {
                          setLookupText(null);
                          editor.current?.focus();
                        }}
                      >
                        <X size={14} aria-hidden="true" />
                      </button>
                    </form>
                    {lookupText.trim().length >= 2 && (
                      <>
                        <p className="sr-only" role="status">
                          {plural(concordanceHits.length, "result", "results")}
                        </p>
                        {concordanceHits.length === 0 ? (
                          <p className="muted">
                            Nothing in the memory contains “{lookupText.trim()}
                            ”.
                          </p>
                        ) : (
                          <ul className="lookup-list">
                            {concordanceHits.map((c, i) => (
                              <li key={i}>
                                <span lang={source}>
                                  {c.side === "source" ? (
                                    <Highlight
                                      text={plain(c.unit.source)}
                                      start={c.start}
                                      end={c.end}
                                    />
                                  ) : (
                                    plain(c.unit.source)
                                  )}
                                </span>
                                <span lang={target} className="lookup-target">
                                  {c.side === "target" ? (
                                    <Highlight
                                      text={plain(c.unit.target)}
                                      start={c.start}
                                      end={c.end}
                                    />
                                  ) : (
                                    plain(c.unit.target)
                                  )}
                                </span>
                                <span className="result-meta">
                                  {unitMeta(c.unit)}
                                </span>
                              </li>
                            ))}
                          </ul>
                        )}
                      </>
                    )}
                  </Section>
                )}

                <Section title="Review" count={active.issues?.length}>
                  <SegmentIssues
                    key={active.id}
                    issues={active.issues ?? []}
                    me={reviewing.me}
                    deletesAll={reviewing.deletesAll}
                    quality={quality}
                    onAdd={(issue) => reviewing.add(active.id, issue)}
                    onDelete={(id) => reviewing.remove(active.id, id)}
                    onSave={saveReview}
                  />
                </Section>

                <Section title="Comments" count={active.comments?.length}>
                  <SegmentComments
                    key={active.id}
                    comments={active.comments ?? []}
                    me={commenting.me}
                    deletesAll={commenting.deletesAll}
                    onAdd={(text) => commenting.add(active.id, text)}
                    onDelete={(id) => commenting.remove(active.id, id)}
                  />
                </Section>

                <Section title="Add a term">
                  {capture ? (
                    <CaptureForm
                      key={`${capture.source}|${capture.target}`}
                      initial={capture}
                      sourceLang={source}
                      targetLang={target}
                      termbaseName={writeTermbase?.name ?? ""}
                      location={writeTermbase?.location ?? "local"}
                      onCancel={() => {
                        setCapture(null);
                        editor.current?.focus();
                      }}
                      onSave={(src, tgt, note) => {
                        dispatch({
                          type: "addTerm",
                          id: `t-${Date.now().toString(36)}`,
                          now: new Date().toISOString(),
                          input: {
                            projectId,
                            lang: target,
                            source: src,
                            target: tgt,
                            note,
                            segment: plain(active.source),
                          },
                        });
                        notify(
                          `Added “${src}” to ${writeTermbase?.name ?? "the termbase"}`,
                        );
                        setCapture(null);
                        editor.current?.focus();
                      }}
                    />
                  ) : (
                    <div className="capture-intro">
                      <p className="muted">
                        Select the term in the source and its translation in the
                        target, then add it. The sentence is saved with it.
                      </p>
                      <div className="tool-row">
                        <button
                          type="button"
                          className="btn btn-small"
                          onClick={openCapture}
                        >
                          <Plus size={14} aria-hidden="true" />
                          Add term
                          <kbd>Ctrl+E</kbd>
                        </button>
                        <button
                          type="button"
                          className="btn btn-small"
                          onClick={openLookup}
                        >
                          <Search size={14} aria-hidden="true" />
                          Search memory
                          <kbd>Ctrl+K</kbd>
                        </button>
                      </div>
                    </div>
                  )}
                </Section>

                <div className="tool-row">
                  <button
                    type="button"
                    className="btn btn-small"
                    onClick={() =>
                      active && editor.current?.setRuns(active.source)
                    }
                  >
                    <Copy size={14} aria-hidden="true" />
                    Copy source
                    <kbd>Ctrl+Ins</kbd>
                  </button>
                  <button
                    type="button"
                    className="btn btn-small"
                    onClick={insertNextTag}
                    disabled={
                      missingTags(active.source, active.target).length === 0
                    }
                  >
                    <Tag size={14} aria-hidden="true" />
                    Next tag
                    <kbd>F9</kbd>
                  </button>
                  <button
                    type="button"
                    className="btn btn-primary btn-small"
                    onClick={confirmAndNext}
                  >
                    <Check size={14} aria-hidden="true" />
                    Confirm
                    <kbd>Ctrl+Enter</kbd>
                  </button>
                </div>

                {cuttable && (
                  <div className="tool-row">
                    <button type="button" className="btn btn-small" onClick={() => setSplitting(true)} disabled={!cuts}>
                      <Scissors size={14} aria-hidden="true" />
                      Split
                      <kbd>Alt+Shift+S</kbd>
                    </button>
                    <button type="button" className="btn btn-small" onClick={() => void join()} disabled={!joins}>
                      <Combine size={14} aria-hidden="true" />
                      Join with next
                      <kbd>Alt+Shift+J</kbd>
                    </button>
                    {(fixed || active.joined || active.split) && (
                      <p className="help cut-note">
                        {fixed
                          ? "The program this file comes from keeps its own record of every segment, so its segments stay as they are."
                          : active.split && active.joined
                            ? `A part of ${active.joined.length} segments of the file that were joined.`
                            : active.joined
                              ? `Joined from ${active.joined.length} segments of the file. Split it where they met to have them apart again.`
                              : "A part of a segment of the file that was split."}
                      </p>
                    )}
                  </div>
                )}

                <details className="shortcuts">
                  <summary>
                    <Keyboard size={14} aria-hidden="true" />
                    Keyboard shortcuts
                  </summary>
                  <dl>
                    <dt>Ctrl+Enter</dt>
                    <dd>
                      Confirm, fill repetitions and go to the next open segment
                    </dd>
                    <dt>Alt+↓ / Alt+↑</dt>
                    <dd>Next / previous segment</dd>
                    <dt>Ctrl+1 to 9</dt>
                    <dd>
                      Insert result 1 to 9 (a memory match replaces the target,
                      a term goes in at the caret)
                    </dd>
                    <dt>Ctrl+↓ / Ctrl+↑, Ctrl+Space</dt>
                    <dd>
                      Move through the results, insert the highlighted one
                    </dd>
                    <dt>Ctrl+Insert</dt>
                    <dd>Copy source to target</dd>
                    <dt>F9</dt>
                    <dd>Insert the next missing tag</dd>
                    <dt>Ctrl+E</dt>
                    <dd>Add a term from the selections</dd>
                    <dt>Ctrl+H</dt>
                    <dd>Find and replace in the translations</dd>
                    <dt>Alt+Shift+S, Alt+Shift+J</dt>
                    <dd>
                      Split the segment at any place, join it with the next one
                      of its paragraph
                    </dd>
                    <dt>Ctrl+K or F3</dt>
                    <dd>Search the memory for the selection</dd>
                  </dl>
                  <p className="help">
                    These are the shortcuts usual in translation tools.
                  </p>
                </details>
              </>
            )}
          </aside>
        </div>
      )}
    </div>
  );
}

function unitMeta(u: TmUnit & { memory?: MemoryRef }): string {
  const parts = [
    u.memory
      ? `${u.memory.name}${u.memory.project ? " (project memory)" : ""}`
      : "Memory",
  ];
  if (u.memory?.penalty) parts.push(`penalty ${u.memory.penalty}`);
  if (u.project && u.project !== u.memory?.name) parts.push(u.project);
  if (u.by) parts.push(u.by);
  if (u.at) parts.push(fmtDate(u.at));
  return parts.join(", ");
}

/** How a bilingual file's state reads when it says more than the editor's status. */
const FILE_STATE_TEXT: Partial<
  Record<NonNullable<Segment["file"]>["state"], string>
> = {
  reviewed: "Approved by a reviewer in the file",
  final: "Signed off in the file",
  rejected: "Rejected by a reviewer in the file",
};

function SegmentStatusCell({ s }: { s: Segment }) {
  const warnings = s.warnings ?? [];
  const fileNote =
    s.file && JSON.stringify(runsToCore(s.target)) === s.file.target
      ? FILE_STATE_TEXT[s.file.state]
      : undefined;
  const title = [
    STATUS_TEXT[s.status],
    s.locked ? (s.lockedReason ?? "Locked in the file") : "",
    fileNote ?? "",
    ...(s.notes ?? []).map((n) => `Note: ${n}`),
    ...(s.comments ?? []).map((c) => `${c.by}: ${c.text}`),
    ...warnings.map((x) => x.text),
  ]
    .filter(Boolean)
    .join("\n");
  return (
    <span className="seg-status" title={title}>
      {s.status === "confirmed" ? (
        <Check size={16} aria-label="Confirmed" />
      ) : s.status === "draft" ? (
        <PencilLine size={15} aria-label="Draft" />
      ) : (
        <CircleDashed size={15} aria-label="Not translated" />
      )}
      {s.locked && <Lock size={13} aria-label={s.lockedReason ?? "Locked in the file"} />}
      {fileNote && (
        <span className={`seg-file-state seg-file-${s.file!.state}`}>
          {s.file!.state === "rejected"
            ? "Rejected"
            : s.file!.state === "final"
              ? "Signed off"
              : "Approved"}
        </span>
      )}
      {s.file?.match && !s.origin ? (
        <span
          className="seg-file-match"
          title={`${s.file.match}% match in the file`}
        >
          {s.file.match}%
        </span>
      ) : null}
      {(s.issues?.length ?? 0) > 0 && (
        <span className="seg-comments seg-issues">
          <Flag size={13} aria-hidden="true" />
          <span className="sr-only">{plural(s.issues!.length, "issue marked in review", "issues marked in review")}</span>
          <span aria-hidden="true">{s.issues!.length}</span>
        </span>
      )}
      {(s.comments?.length ?? 0) > 0 && (
        <span className="seg-comments">
          <MessageSquare size={13} aria-hidden="true" />
          <span className="sr-only">{plural(s.comments!.length, "comment", "comments")}</span>
          <span aria-hidden="true">{s.comments!.length}</span>
        </span>
      )}
      {warnings.length > 0 && (
        <span className="seg-warn">
          <AlertTriangle size={13} aria-hidden="true" />
          <span className="sr-only">
            {plural(warnings.length, "warning", "warnings")}
          </span>
          <span aria-hidden="true">{warnings.length}</span>
        </span>
      )}
      {s.origin?.kind === "tm" && (
        <ScoreBadge
          origin={s.origin}
          edited={plain(s.target) !== s.origin.inserted}
        />
      )}
      {s.origin?.kind === "repetition" && (
        <span className="seg-origin">
          <Repeat2 size={13} aria-label="Repetition" />
        </span>
      )}
    </span>
  );
}

/**
 * The match score next to a segment: shown once a match was inserted, not
 * for what the memories would offer. Those are in the results panel when
 * the segment is opened.
 */
function ScoreBadge({
  origin,
  edited,
}: {
  origin: { score: number; context?: boolean };
  edited: boolean;
}) {
  const label = scoreLabel(origin.score, origin.context);
  const desc = `${scoreDescription(origin.score, origin.context)}${edited ? ", edited" : ""}`;
  return (
    <span
      className={`score band-${bandOf(origin.score, origin.context)}${edited ? " is-edited" : ""}`}
      title={desc}
    >
      <span aria-hidden="true">{label}</span>
      <span className="sr-only">{desc}</span>
    </span>
  );
}

function DialogAnalysis({
  c,
  project,
  target,
  memories,
}: {
  c: CoreApi;
  project: NonNullable<
    ReturnType<typeof useStore>["state"]["projects"][number]
  >;
  target: LangCode;
  memories: MemoryRef[];
}) {
  // Computed when the dialog renders, against the memories as they are now.
  const r = useMemo(
    () =>
      analyzeFiles(
        c,
        project.files,
        project.sourceLang,
        target,
        memories,
        project.context,
      ),
    [c, project.files, project.sourceLang, target, memories, project.context],
  );
  return (
    <AnalysisTable rows={r.rows} total={r.total} caption="Match analysis" />
  );
}

function MatchBody({
  m,
  source,
  target,
}: {
  m: ScoredMatch;
  source: string;
  target: string;
}) {
  return (
    <>
      <span
        className={`result-score score band-${bandOf(m.score, m.context)}`}
        title={scoreDescription(m.score, m.context)}
      >
        <span aria-hidden="true">{scoreLabel(m.score, m.context)}</span>
        <span className="sr-only">{scoreDescription(m.score, m.context)}</span>
      </span>
      <span className="result-body">
        <span className="result-source" lang={source}>
          {m.diff.map((d, k) => (
            <span key={k}>
              {d.space && " "}
              {d.changed ? <mark>{d.text}</mark> : d.text}
            </span>
          ))}
        </span>
        <span className="result-target" lang={target}>
          <RunsView runs={m.unit.target} />
        </span>
        <span className="result-meta">{unitMeta(m.unit)}</span>
      </span>
    </>
  );
}

function TermBody({
  h,
  source,
  target,
}: {
  h: TermHit;
  source: string;
  target: string;
}) {
  return (
    <>
      <span className="result-score result-term">
        <Tag size={13} aria-hidden="true" />
        <span className="sr-only">Term</span>
      </span>
      <span className="result-body">
        <span className="result-source">
          <InLang code={source}>{h.term.text}</InLang>
        </span>
        <span className="result-target">
          {h.targets.map((t, k) => (
            <span key={t.text}>
              {k > 0 && ", "}
              <span
                className={
                  t.status === "forbidden" ? "term-forbidden" : undefined
                }
              >
                <InLang code={target}>{t.text}</InLang>
              </span>
              {t.status !== "approved" && (
                <>
                  {" "}
                  <StatusBadge status={t.status} />
                </>
              )}
            </span>
          ))}
        </span>
        <span className="result-meta">{h.termbase.name}</span>
      </span>
    </>
  );
}

function Highlight({
  text,
  start,
  end,
}: {
  text: string;
  start: number;
  end: number;
}) {
  return (
    <>
      {text.slice(0, start)}
      <mark>{text.slice(start, end)}</mark>
      {text.slice(end)}
    </>
  );
}

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count?: number;
  children: ReactNode;
}) {
  return (
    <section className="results-section" aria-label={title}>
      <h2 className="results-title">
        {title}
        {count !== undefined && count > 0 && (
          <span className="results-count">{count}</span>
        )}
      </h2>
      {children}
    </section>
  );
}

export function RunsView({ runs }: { runs: Run[] }) {
  return (
    <>
      {runs.map((r, i) =>
        typeof r === "string" ? (
          <span key={i}>{r}</span>
        ) : (
          <span
            key={i}
            className={`tag-chip tag-${r.kind}`}
            role="img"
            aria-label={tagDescription(r)}
            title={r.desc}
          >
            {chipText(r)}
          </span>
        ),
      )}
    </>
  );
}

/** Source text with tags as chips and recognised terms underlined. */
function primaryOf(code: string): string {
  return code.split("-")[0]!.toLowerCase();
}

function SourceText({ runs, hits }: { runs: Run[]; hits: TermHit[] }) {
  const out: ReactNode[] = [];
  let offset = 0;
  runs.forEach((r, i) => {
    if (typeof r !== "string") {
      out.push(
        <span
          key={`t${i}`}
          className={`tag-chip tag-${r.kind}`}
          role="img"
          aria-label={tagDescription(r)}
          title={r.desc}
        >
          {chipText(r)}
        </span>,
      );
      return;
    }
    const start = offset;
    const end = offset + r.length;
    let pos = start;
    for (const h of hits.filter((x) => x.start < end && x.end > start)) {
      const a = Math.max(h.start, start);
      const b = Math.min(h.end, end);
      if (a > pos)
        out.push(
          <span key={`${i}-${pos}`}>{r.slice(pos - start, a - start)}</span>,
        );
      const tgt = h.targets
        .filter((t) => t.status !== "forbidden")
        .map((t) => t.text)
        .join(", ");
      out.push(
        <span
          key={`${i}-h${a}`}
          className="term-hit"
          title={`${h.termbase.name}: ${tgt || "no translation"}`}
        >
          {r.slice(a - start, b - start)}
        </span>,
      );
      pos = b;
    }
    if (pos < end)
      out.push(<span key={`${i}-${pos}`}>{r.slice(pos - start)}</span>);
    offset = end;
  });
  return <>{out}</>;
}

function CaptureForm({
  initial,
  sourceLang,
  targetLang,
  termbaseName,
  location,
  onCancel,
  onSave,
}: {
  initial: { source: string; target: string };
  sourceLang: string;
  targetLang: string;
  termbaseName: string;
  location: "local" | "server";
  onCancel: () => void;
  onSave: (source: string, target: string, note?: string) => void;
}) {
  const [src, setSrc] = useState(initial.source);
  const [tgt, setTgt] = useState(initial.target);
  const [note, setNote] = useState("");
  const first = useRef<HTMLInputElement>(null);
  const second = useRef<HTMLInputElement>(null);
  useEffect(() => {
    (initial.source ? second.current : first.current)?.focus();
  }, [initial.source]);
  const valid = src.trim() && tgt.trim();
  return (
    <form
      className="capture"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid) onSave(src.trim(), tgt.trim(), note.trim() || undefined);
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          onCancel();
        }
      }}
    >
      <label htmlFor="cap-src">{lang(sourceLang).name}</label>
      <input
        id="cap-src"
        ref={first}
        lang={sourceLang}
        value={src}
        onChange={(e) => setSrc(e.target.value)}
        placeholder="Select it in the source, or type"
      />
      <label htmlFor="cap-tgt">{lang(targetLang).name}</label>
      <input
        id="cap-tgt"
        ref={second}
        lang={targetLang}
        value={tgt}
        onChange={(e) => setTgt(e.target.value)}
        placeholder="Select it in your translation, or type"
      />
      <label htmlFor="cap-note">
        Note <span className="optional">optional</span>
      </label>
      <input
        id="cap-note"
        value={note}
        onChange={(e) => setNote(e.target.value)}
      />
      <p className="help">
        Saved as proposed to {termbaseName}{" "}
        <LocationBadge location={location} />, with this sentence.
      </p>
      <div className="row">
        <button
          type="submit"
          className="btn btn-primary btn-small"
          disabled={!valid}
        >
          Add term
        </button>
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

/** Characters used against the file's limit for the translation. */
function LengthCounter({ length, max }: { length: number; max: number }) {
  const over = length > max;
  return (
    <span
      className={`length-counter${over ? " is-over" : ""}`}
      aria-live="polite"
    >
      {length} of {max} characters
      {over ? `, ${length - max} too many` : ""}
    </span>
  );
}


/**
 * Find and replace in the translations of the open file. Locked segments
 * stay as they are, tags stay where they are, and a segment that changes
 * is a draft again.
 */
/**
 * Suggestions from translation services for the open segment: asked for
 * one at a time, as asking sends the segment to the service.
 */
function ServiceSuggestions({
  names,
  lang: targetLang,
  ask,
  onInsert,
}: {
  names: string[];
  lang: LangCode;
  ask: (name: string) => Promise<{ target: CoreRun[]; tagsLost: boolean }>;
  onInsert: (runs: Run[]) => void;
}) {
  const [busy, setBusy] = useState("");
  const [got, setGot] = useState<{ name: string; runs: Run[]; tagsLost: boolean } | null>(null);
  const [problem, setProblem] = useState("");
  return (
    <div className="service-suggestions">
      <div className="row">
        {names.map((name) => (
          <button
            key={name}
            type="button"
            className="btn btn-small"
            disabled={!!busy}
            onClick={() => {
              setBusy(name);
              setProblem("");
              ask(name).then(
                (r) => {
                  setGot({ name, runs: coreToRuns(r.target), tagsLost: r.tagsLost });
                  setBusy("");
                },
                (e) => {
                  setProblem(e instanceof Error ? e.message : String(e));
                  setBusy("");
                },
              );
            }}
          >
            <Languages size={14} aria-hidden="true" />
            {busy === name ? `Asking ${name}…` : `Suggest with ${name}`}
          </button>
        ))}
      </div>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      {got && (
        <div className="suggestion" role="status">
          <p lang={targetLang}>{plain(got.runs)}</p>
          <p className="result-meta">
            From {got.name}
            {got.tagsLost ? ", without the source's tags: put them in by hand" : ""}
          </p>
          <div className="row">
            <button type="button" className="btn btn-small" onClick={() => onInsert(got.runs)}>
              Insert
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

const categoryName = (c: string) => ISSUE_CATEGORIES.find(([id]) => id === c)?.[1] ?? c;
const severityName = (v: string) => ISSUE_SEVERITIES.find(([id]) => id === v)?.[1] ?? v;
const severityPoints = (v: string) => ISSUE_SEVERITIES.find(([id]) => id === v)?.[2] ?? 0;

/** What a file's score says, in a sentence. */
export function qualityText(q: Quality): string {
  if (q.issues === 0) return `No issues marked in ${plural(q.words, "word", "words")}.`;
  const parts = [
    q.minor ? `${q.minor} minor` : "",
    q.major ? `${q.major} major` : "",
    q.critical ? `${q.critical} critical` : "",
  ].filter(Boolean);
  return `${plural(q.issues, "issue", "issues")} (${parts.join(", ")}), ${plural(q.penalty, "penalty point", "penalty points")} in ${plural(q.words, "word", "words")}.`;
}

/** The issues a reviewer marked on the open segment, a form for a new one, and the file's score. */
function SegmentIssues({
  issues,
  me,
  deletesAll,
  quality,
  onAdd,
  onDelete,
  onSave,
}: {
  issues: SegmentIssue[];
  me: string;
  deletesAll: boolean;
  quality: Quality | null;
  onAdd: (issue: { category: IssueCategory; severity: IssueSeverity; note?: string }) => Promise<unknown>;
  onDelete: (id: string) => Promise<unknown>;
  onSave: () => Promise<unknown>;
}) {
  const [category, setCategory] = useState<IssueCategory>("accuracy");
  const [severity, setSeverity] = useState<IssueSeverity>("minor");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const add = () => {
    if (busy) return;
    setBusy(true);
    void onAdd({ category, severity, note: note.trim() || undefined }).then(() => {
      setNote("");
      setBusy(false);
    });
  };
  return (
    <>
      {issues.length === 0 ? (
        <p className="muted">No issues marked on this segment.</p>
      ) : (
        <ul className="comment-list">
          {issues.map((i) => (
            <li key={i.id}>
              <p className="comment-text">
                <strong>{categoryName(i.category)}</strong>, {severityName(i.severity).toLowerCase()} ({plural(severityPoints(i.severity), "point", "points")})
                {i.note ? `: ${i.note}` : ""}
              </p>
              <p className="result-meta">
                {i.by}, {fmtDate(i.at)}
                {(deletesAll || i.by === me) && (
                  <button
                    type="button"
                    className="link-button"
                    aria-label={`Delete the issue ${categoryName(i.category)}, ${severityName(i.severity).toLowerCase()}`}
                    onClick={() => void onDelete(i.id)}
                  >
                    Delete
                  </button>
                )}
              </p>
            </li>
          ))}
        </ul>
      )}
      <div className="field-grid issue-form">
        <div className="field">
          <label htmlFor="issue-category">Kind of issue</label>
          <select id="issue-category" value={category} onChange={(e) => setCategory(e.target.value as IssueCategory)}>
            {ISSUE_CATEGORIES.map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="issue-severity">Severity</label>
          <select id="issue-severity" value={severity} onChange={(e) => setSeverity(e.target.value as IssueSeverity)}>
            {ISSUE_SEVERITIES.map(([id, name, points]) => (
              <option key={id} value={id}>
                {name} ({plural(points, "point", "points")})
              </option>
            ))}
          </select>
        </div>
      </div>
      <label className="field">
        <span className="sr-only">What is wrong, if you want to say</span>
        <input
          id="issue-note"
          maxLength={1000}
          placeholder="What is wrong (optional)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              e.stopPropagation();
              add();
            }
          }}
        />
      </label>
      <button type="button" className="btn btn-small" disabled={busy} onClick={add}>
        <Flag size={14} aria-hidden="true" />
        Mark issue
      </button>
      {quality && (
        <div className="quality" role="status">
          <p className="quality-score">
            Score of this file <strong>{quality.score.toFixed(1)}</strong>
            <span className="muted"> of 100</span>
          </p>
          <p className="help">
            {qualityText(quality)} The score is 100 less the penalty points in percent of the words.
          </p>
          <button type="button" className="link-button" onClick={() => void onSave()}>
            Save the project's review as CSV
          </button>
        </div>
      )}
    </>
  );
}

/** The comments on the open segment, and a field for a new one. */
function SegmentComments({
  comments,
  me,
  deletesAll,
  onAdd,
  onDelete,
}: {
  comments: SegmentComment[];
  me: string;
  deletesAll: boolean;
  onAdd: (text: string) => Promise<unknown>;
  onDelete: (id: string) => Promise<unknown>;
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const add = () => {
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    void onAdd(t).then(() => {
      setText("");
      setBusy(false);
    });
  };
  return (
    <>
      {comments.length === 0 ? (
        <p className="muted">No comments on this segment.</p>
      ) : (
        <ul className="comment-list">
          {comments.map((c) => (
            <li key={c.id}>
              <p className="comment-text">{c.text}</p>
              <p className="result-meta">
                {c.by}, {fmtDate(c.at)}
                {(deletesAll || c.by === me) && (
                  <button type="button" className="link-button" aria-label={`Delete the comment “${c.text}”`} onClick={() => void onDelete(c.id)}>
                    Delete
                  </button>
                )}
              </p>
            </li>
          ))}
        </ul>
      )}
      <label className="field">
        <span className="sr-only">A comment on this segment</span>
        <textarea
          rows={2}
          maxLength={2000}
          placeholder="A comment for those working on the project"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              e.stopPropagation();
              add();
            }
          }}
        />
      </label>
      <button type="button" className="btn btn-small" disabled={!text.trim() || busy} onClick={add}>
        <MessageSquare size={14} aria-hidden="true" />
        Add comment
      </button>
    </>
  );
}

/**
 * Asks where a segment is split: the caret in its source is the place.
 * onSplit resolves to why it could not be done, or to nothing.
 */
function SplitDialog({
  segment,
  lang: sourceLang,
  onClose,
  onSplit,
}: {
  segment: Segment;
  lang: LangCode;
  onClose: () => void;
  onSplit: (before: Run[]) => Promise<string | undefined>;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [parts, setParts] = useState<[Run[], Run[]]>([[], segment.source]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const el = box.current!;
    renderRuns(el, segment.source);
    const stop = (e: Event) => e.preventDefault();
    for (const t of ["beforeinput", "paste", "cut", "drop"]) el.addEventListener(t, stop);
    // The caret divides the source: what is before it and what is after.
    const where = () => {
      const sel = window.getSelection();
      if (!sel?.rangeCount || !el.contains(sel.focusNode)) return;
      const side = (first: boolean) => {
        const r = document.createRange();
        r.selectNodeContents(el);
        if (first) r.setEnd(sel.focusNode!, sel.focusOffset);
        else r.setStart(sel.focusNode!, sel.focusOffset);
        const d = document.createElement("div");
        d.append(r.cloneContents());
        return readRuns(d, true);
      };
      setParts([side(true), side(false)]);
    };
    document.addEventListener("selectionchange", where);
    el.focus();
    return () => {
      document.removeEventListener("selectionchange", where);
      for (const t of ["beforeinput", "paste", "cut", "drop"]) el.removeEventListener(t, stop);
    };
  }, [segment.source]);
  const [first, second] = parts;
  const hasText = (runs: Run[]) => /[\p{L}\p{N}]/u.test(plain(runs));
  const ok = hasText(first) && hasText(second);
  const trimmed = (runs: Run[], end: boolean): Run[] =>
    runs.map((r, i) =>
      typeof r !== "string" ? r : end && i === runs.length - 1 ? r.trimEnd() : !end && i === 0 ? r.trimStart() : r,
    );
  const split = async () => {
    if (!ok || busy) return;
    setBusy(true);
    setError("");
    const no = await onSplit(first);
    setBusy(false);
    if (no) setError(no);
  };
  return (
    <Modal title="Split the segment" onClose={onClose}>
      <form
        className="dialog-form"
        onSubmit={(e) => {
          e.preventDefault();
          void split();
        }}
      >
        <p className="help" id="split-help">
          Click where the second part begins, or go there with the arrow keys. The translation stays with the first part.
          {segment.joined ? " Split where two joined segments met, they stand apart again." : ""}
        </p>
        <div
          ref={box}
          className="split-source"
          role="textbox"
          aria-multiline="true"
          aria-readonly="true"
          aria-label="Source of the segment"
          aria-describedby="split-help"
          contentEditable
          suppressContentEditableWarning
          spellCheck={false}
          lang={sourceLang}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void split();
            }
          }}
        />
        <dl className="split-parts" aria-live="polite">
          <dt>First part</dt>
          <dd lang={sourceLang}>{ok ? <RunsView runs={trimmed(first, true)} /> : <span className="muted">Place the cursor inside the text.</span>}</dd>
          <dt>Second part</dt>
          <dd lang={sourceLang}>{ok ? <RunsView runs={trimmed(second, false)} /> : <span className="muted">Both parts need text.</span>}</dd>
        </dl>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary btn-small" disabled={!ok || busy}>
            Split
          </button>
        </div>
      </form>
    </Modal>
  );
}

function ReplaceDialog({
  segments,
  lang: targetLang,
  initial,
  onClose,
  onReplace,
}: {
  segments: Segment[];
  lang: LangCode;
  initial: string;
  onClose: () => void;
  onReplace: (changes: { segmentId: string; target: Run[] }[], places: number) => void;
}) {
  const [find, setFind] = useState(initial);
  const [to, setTo] = useState("");
  const [matchCase, setMatchCase] = useState(false);
  const [wholeWords, setWholeWords] = useState(false);
  const re = searchPattern(find, { matchCase, wholeWords });
  const changes: { segmentId: string; target: Run[] }[] = [];
  let places = 0;
  let locked = 0;
  if (re)
    for (const s of segments) {
      if (s.locked) {
        if (countIn(s.target, re)) locked++;
        continue;
      }
      const r = replaceInRuns(s.target, re, to);
      if (r.count) {
        places += r.count;
        changes.push({ segmentId: s.id, target: r.runs });
      }
    }
  return (
    <Modal title="Find and replace" onClose={onClose}>
      <form
        className="dialog-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (changes.length) onReplace(changes, places);
        }}
      >
        <label className="field">
          <span className="field-label">Find in the translations</span>
          <input value={find} lang={targetLang} autoFocus onChange={(e) => setFind(e.target.value)} />
        </label>
        <label className="field">
          <span className="field-label">Replace with</span>
          <input value={to} lang={targetLang} onChange={(e) => setTo(e.target.value)} />
        </label>
        <label className="check">
          <input type="checkbox" checked={matchCase} onChange={(e) => setMatchCase(e.target.checked)} /> Match case
        </label>
        <label className="check">
          <input type="checkbox" checked={wholeWords} onChange={(e) => setWholeWords(e.target.checked)} /> Whole words only
        </label>
        <p role="status">
          {!find
            ? "Nothing to find yet."
            : places === 0
              ? "Not found in the translations of this file."
              : `${plural(places, "place", "places")} in ${plural(changes.length, "segment", "segments")}.`}
          {locked > 0 && ` ${plural(locked, "locked segment has", "locked segments have")} it too and ${locked === 1 ? "stays" : "stay"} as ${locked === 1 ? "it is" : "they are"}.`}
        </p>
        <p className="help">Formatting tags stay where they are. A confirmed segment that changes is a draft again, to be confirmed anew.</p>
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary btn-small" disabled={!changes.length}>
            Replace all
          </button>
        </div>
      </form>
    </Modal>
  );
}
