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
  Settings2,
  Tag,
  X,
} from "lucide-react";
import { fileTargets, lang, type LangCode } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges, incomingFromMaster } from "../merge";
import {
  isTag,
  missingTags,
  plain,
  runsToCore,
  sourceKey,
  wordCount,
  type Run,
  type Segment,
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
  availableMatches,
  type Available,
  checkSegment,
  memoryRefs,
  projectMemoryOf,
  type MemoryRef,
} from "../project";
import { go } from "../App";
import { Preview } from "../editor/Preview";
import { AnalysisTable } from "../components/AnalysisTable";
import {
  core,
  DOCX_TYPE,
  download,
  loadCore,
  mergeDocx,
  mergeXliff,
  returnSdlPackage,
  SDL_RETURN_TYPE,
  tm,
  XLIFF_TYPE,
  type CoreApi,
} from "../core";
import {
  TargetEditor,
  chipText,
  tagDescription,
  type TargetEditorHandle,
} from "../editor/TargetEditor";
import { InLang, LangChip, LocationBadge, StatusBadge } from "../components/ui";
import { fmtDate, fmtNum, plural } from "../format";
import { usePresence, useServerConcordance } from "../remote";

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
  const here = useMemo(() => {
    const m = new Map<string, typeof present>();
    for (const x of present) m.set(x.segment, [...(m.get(x.segment) ?? []), x]);
    return m;
  }, [present]);
  // Term recognition runs in the core; the index is rebuilt when an entry changes.
  const termIndex = useMemo(() => {
    if (!c || !project) return null;
    const name = `editor:${project.id}`;
    buildIndex(c, name, termbases, state.entries);
    // A new object each rebuild, so hits and checks recompute.
    return { name };
  }, [c, project, termbases, state.entries]);
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
  // The match column: the best match every segment of the file has in the
  // memories, worked out once the file is open and again when the
  // memories gain units, a moment after the last change.
  const [available, setAvailable] = useState<Map<string, Available>>(
    () => new Map(),
  );
  const sourceLang = project?.sourceLang;
  useEffect(() => {
    if (!c || !file || !sourceLang) return;
    const t = setTimeout(() => {
      try {
        setAvailable(
          availableMatches(c, file, sourceLang, target, memories, context),
        );
      } catch {
        // A memory that cannot be read shows in the results panel.
        setAvailable(new Map());
      }
    }, 300);
    return () => clearTimeout(t);
    // The file's sources do not change while it is open: its ID stands for them.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [c, file?.id, sourceLang, target, memories, context, memVersion]);
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

  const visible =
    filter === "all"
      ? segments
      : segments.filter(
          (s) =>
            s.id === activeId ||
            (filter === "todo"
              ? s.status !== "confirmed"
              : (s.warnings?.length ?? 0) > 0),
        );
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
          const rpx = await returnSdlPackage(pkg.bytes, translated);
          const name = pkg.name.replace(/\.sdlppx$/i, "") + ".sdlrpx";
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
      );
      const base = (file.name.split("/").pop() ?? file.name).replace(
        /\.docx$/i,
        "",
      );
      const name = `${base}_${target}.docx`;
      download(out, name, DOCX_TYPE);
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
      insertNextTag();
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
            counted as in Trados: context matches first, then repetitions within
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
                  <SegmentStatusCell s={s} available={available.get(s.id)} />
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
                Layout from the Word file. Untranslated text appears in the
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
                    <dt>Ctrl+K or F3</dt>
                    <dd>Search the memory for the selection</dd>
                  </dl>
                  <p className="help">
                    These follow Trados, memoQ and Phrase where they agree.
                    Editable in the full product.
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

function SegmentStatusCell({
  s,
  available,
}: {
  s: Segment;
  /** The best match the memories hold for the segment. */
  available?: Available;
}) {
  // Shown until the segment has a translation from a match or is confirmed.
  const offer =
    available && !s.origin && s.status !== "confirmed" && !s.locked
      ? available
      : undefined;
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
      {offer && <AvailableBadge offer={offer} />}
    </span>
  );
}

/**
 * The best match the memories hold for a segment that has no translation
 * from one yet: outlined with dashes, where an inserted match is filled.
 */
function AvailableBadge({ offer }: { offer: Available }) {
  if (offer.band === "repetition" || offer.band === "cross-file") {
    const text =
      offer.band === "repetition"
        ? "Repeats an earlier segment of this file"
        : "Repeats a segment of another file";
    return (
      <span className="seg-origin seg-available" title={text}>
        <Repeat2 size={13} aria-label={text} />
      </span>
    );
  }
  const context = offer.band === "context";
  const score = offer.score ?? 100;
  const desc = `${scoreDescription(score, context)}${offer.memory ? ` in ${offer.memory}` : ""}, not inserted`;
  return (
    <span
      className={`score is-available band-${bandOf(score, context)}`}
      title={desc}
    >
      <span aria-hidden="true">{scoreLabel(score, context)}</span>
      <span className="sr-only">{desc}</span>
    </span>
  );
}

/** The match score next to a segment, as in Trados' status column. */
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
