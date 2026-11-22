import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { AlertTriangle, ArrowLeft, Check, CircleDashed, Copy, GitMerge, Keyboard, PencilLine, Plus, Repeat2, Search, Tag, X } from "lucide-react";
import { lang, USER } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges } from "../merge";
import { isTag, missingTags, plain, sourceKey, wordCount, TM_UNITS, type Run, type Segment, type TmUnit } from "../editor/model";
import { concordance, lookup, qa, recognize, type QaIssue, type TermHit, type TmMatch } from "../editor/match";
import { TargetEditor, chipText, tagDescription, type TargetEditorHandle } from "../editor/TargetEditor";
import { InLang, LangChip, LocationBadge, StatusBadge } from "../components/ui";
import { fmtDate, plural } from "../format";

const STATUS_TEXT: Record<Segment["status"], string> = { new: "Not translated", draft: "Draft", confirmed: "Confirmed" };

type Filter = "all" | "todo" | "warnings";
type Result = { kind: "tm"; m: TmMatch } | { kind: "term"; h: TermHit };

export function Editor({ projectId }: { projectId: string }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const project = state.projects.find((p) => p.id === projectId);
  const segments = useMemo(() => project?.segments ?? [], [project?.segments]);
  const [activeId, setActiveId] = useState(() => segments.find((s) => s.status !== "confirmed")?.id ?? segments[0]?.id ?? "");
  const [filter, setFilter] = useState<Filter>("all");
  const [capture, setCapture] = useState<{ source: string; target: string } | null>(null);
  const [lookupText, setLookupText] = useState<string | null>(null);
  const [selected, setSelected] = useState(0);
  const [announcement, setAnnouncement] = useState("");
  const editor = useRef<TargetEditorHandle>(null);
  const sourceSelection = useRef("");

  const source = project?.sourceLang ?? "de-DE";
  const target = project?.targetLangs[0] ?? "en-GB";
  const termbases = useMemo(() => {
    if (!project) return [];
    const ids = [...project.setup.masters, ...(project.setup.projectTermbaseId ? [project.setup.projectTermbaseId] : [])];
    return ids.map((id) => state.termbases.find((t) => t.id === id)!).filter(Boolean);
  }, [project, state.termbases]);
  const writeTermbase = project ? state.termbases.find((t) => t.id === (project.setup.mode === "direct" ? project.setup.masters[0] : project.setup.projectTermbaseId)) : undefined;

  // The memory: the client's memory (sample project only) plus every segment
  // confirmed in this document, which is what confirming stores in Trados,
  // memoQ and Phrase.
  const memory = useMemo<TmUnit[]>(() => {
    const doc = segments
      .map((s, i) => ({ s, i }))
      .filter(({ s }) => s.status === "confirmed" && plain(s.target).trim())
      .map(({ s, i }) => ({ source: s.source, target: s.target, project: project?.name ?? "", by: USER, at: project?.createdAt ?? "", docSegment: i + 1 }));
    return [...(project?.id === "p-hofmann" ? TM_UNITS : []), ...doc];
  }, [segments, project?.id, project?.name, project?.createdAt]);

  const active = segments.find((s) => s.id === activeId);
  const index = segments.findIndex((s) => s.id === activeId);
  const hits = useMemo(() => (active ? recognize(plain(active.source), source, target, termbases, state.entries) : []), [active, source, target, termbases, state.entries]);
  const matches = useMemo(() => {
    if (!active) return [];
    const others = memory.filter((u) => u.docSegment !== index + 1);
    // Identical units (a repetition confirmed twice) show once.
    const seen = new Set<string>();
    return lookup(active.source, others, 70, 6)
      .filter((m) => {
        const k = sourceKey(m.unit.source) + "|" + sourceKey(m.unit.target);
        if (seen.has(k)) return false;
        seen.add(k);
        return true;
      })
      .slice(0, 3);
  }, [active, memory, index]);
  const issues = useMemo(() => (active ? qa(active.source, active.target, hits, source, target) : []), [active, hits, source, target]);
  const allIssues = useMemo<QaIssue[]>(() => {
    if (!active) return [];
    const extra: QaIssue[] = [];
    // Unedited fuzzy match: a common check in memoQ, Phrase and Trados Verify.
    if (active.origin?.kind === "tm" && active.origin.score < 100 && plain(active.target) === active.origin.inserted) {
      extra.push({ kind: "unedited", text: `The ${active.origin.score}% match is unchanged. Check what differs from the memory source.` });
    }
    const ignored = new Set(active.ignored ?? []);
    return [...issues, ...extra].filter((q) => !ignored.has(q.text));
  }, [active, issues]);

  // The numbered results list: memory matches first, then terms (as in memoQ).
  const results: Result[] = [
    ...matches.map((m) => ({ kind: "tm" as const, m })),
    ...hits.filter((h) => h.targets.some((t) => t.status !== "forbidden")).map((h) => ({ kind: "term" as const, h })),
  ];
  const concordanceHits = useMemo(() => (lookupText ? concordance(lookupText, memory) : []), [lookupText, memory]);

  // Track a selection made in the active source text, for "Add term" and concordance.
  useEffect(() => {
    const on = () => {
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed) return;
      const node = sel.anchorNode instanceof Element ? sel.anchorNode : sel.anchorNode?.parentElement;
      if (node?.closest(".seg.is-active .seg-source")) sourceSelection.current = sel.toString().trim();
    };
    document.addEventListener("selectionchange", on);
    return () => document.removeEventListener("selectionchange", on);
  }, []);

  // When a segment opens: insert the best match into an empty target (Trados'
  // default), then tell screen readers where they are.
  useEffect(() => {
    if (!active) return;
    setSelected(0);
    sourceSelection.current = "";
    const parts = [`Segment ${index + 1} of ${segments.length}`, STATUS_TEXT[active.status]];
    const best = matches[0];
    if (state.autoInsert && best && active.status === "new" && !plain(active.target).trim()) {
      insertMatch(best);
      parts.push(`${best.score}% match inserted`);
    } else if (best) {
      parts.push(`${plural(matches.length, "memory match", "memory matches")}, best ${best.score}%`);
    }
    if (hits.length) parts.push(plural(hits.length, "term", "terms"));
    if (active.warnings?.length) parts.push(plural(active.warnings.length, "warning", "warnings"));
    setAnnouncement(parts.join(". ") + ".");
  }, [activeId]); // eslint-disable-line react-hooks/exhaustive-deps

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

  const visible =
    filter === "all"
      ? segments
      : segments.filter((s) => s.id === activeId || (filter === "todo" ? s.status !== "confirmed" : (s.warnings?.length ?? 0) > 0));
  const withWarnings = segments.filter((s) => s.warnings?.length).length;
  const totalWords = segments.reduce((n, s) => n + wordCount(s.source), 0);
  const doneWords = segments.filter((s) => s.status === "confirmed").reduce((n, s) => n + wordCount(s.source), 0);
  const pendingChanges =
    project.setup.mode === "layered" && project.setup.projectTermbaseId
      ? project.setup.masters.reduce((n, mid) => {
          const m = state.termbases.find((t) => t.id === mid)!;
          return n + computeChanges(state.entries.filter((e) => e.termbaseId === project.setup.projectTermbaseId), m, state.entries.filter((e) => e.termbaseId === mid), state.baseSnapshots, state.branchedAt[project.id] ?? project.createdAt).length;
        }, 0)
      : 0;

  function setTarget(runs: Run[], status: Segment["status"] = "draft", origin?: Segment["origin"]) {
    if (!active) return;
    const empty = !plain(runs).trim() && !runs.some(isTag);
    dispatch({ type: "setTarget", projectId, segmentId: active.id, target: runs, status: empty ? "new" : status, origin });
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
    if (!plain(active.target).trim()) {
      setAnnouncement("The target is empty. Type a translation or insert a match first.");
      return;
    }
    const key = sourceKey(active.source);
    const repetitions = state.autoPropagate ? segments.filter((s) => s.id !== active.id && s.status !== "confirmed" && sourceKey(s.source) === key) : [];
    const warnings = allIssues.map((q) => ({ kind: q.kind, text: q.text }));
    dispatch({ type: "confirm", projectId, segmentId: active.id, warnings });

    const said = [warnings.length ? `Confirmed with ${plural(warnings.length, "warning", "warnings")}` : "Confirmed"];
    if (repetitions.length) {
      said.push(`${plural(repetitions.length, "repetition", "repetitions")} filled: segment ${repetitions.map((r) => segments.indexOf(r) + 1).join(", ")}`);
      notify(`Also filled ${plural(repetitions.length, "repetition", "repetitions")} (segment ${repetitions.map((r) => segments.indexOf(r) + 1).join(", ")})`);
    }
    const skip = new Set([active.id, ...repetitions.map((r) => r.id)]);
    const next = segments.slice(index + 1).find((s) => s.status !== "confirmed" && !skip.has(s.id)) ?? segments.find((s) => s.status !== "confirmed" && !skip.has(s.id));
    if (next) activate(next.id);
    else {
      notify("All segments confirmed");
      said.push("All segments are confirmed");
    }
    // The next segment's own announcement follows this one.
    setTimeout(() => setAnnouncement((a) => `${said.join(". ")}. ${a}`), 0);
  }

  function insertMatch(m: TmMatch) {
    editor.current?.setRuns(m.unit.target);
    setTarget(m.unit.target, "draft", { kind: "tm", score: m.score, inserted: plain(m.unit.target) });
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
    setCapture({ source: sourceSelection.current, target: editor.current?.selectedText() ?? "" });
  }

  function openLookup() {
    setCapture(null);
    setLookupText(editor.current?.selectedText() || sourceSelection.current || "");
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
            {project.file}
            <span className="lang-pair">
              <LangChip code={source} /> → <LangChip code={target} />
            </span>
          </span>
        </div>
        <div className="editor-progress" aria-label={`${Math.round((doneWords / (totalWords || 1)) * 100)} percent confirmed`}>
          <span className="progress-track" aria-hidden="true">
            <span className="progress-fill" style={{ width: `${(doneWords / (totalWords || 1)) * 100}%` }} />
          </span>
          <span className="progress-text">
            {plural(doneWords, "word", "words")} of {totalWords.toLocaleString("en-GB")} confirmed
          </span>
        </div>
        <div className="segmented segmented-small" role="radiogroup" aria-label="Show segments">
          <label className="segment">
            <input type="radio" name="segfilter" checked={filter === "all"} onChange={() => setFilter("all")} />
            <span>All</span>
          </label>
          <label className="segment">
            <input type="radio" name="segfilter" checked={filter === "todo"} onChange={() => setFilter("todo")} />
            <span>To do</span>
          </label>
          <label className="segment">
            <input type="radio" name="segfilter" checked={filter === "warnings"} onChange={() => setFilter("warnings")} />
            <span>Warnings{withWarnings > 0 && ` (${withWarnings})`}</span>
          </label>
        </div>
        {pendingChanges > 0 && (
          <a className="btn btn-small" href={`#/merge/${project.id}`}>
            <GitMerge size={14} aria-hidden="true" />
            {w("reviewMerge")} ({pendingChanges})
          </a>
        )}
      </header>

      <p className="sr-only" role="status" aria-live="polite">
        {announcement}
      </p>

      {segments.length === 0 ? (
        <div className="empty">
          <p className="empty-title">No text to translate in this prototype</p>
          <p>The document has no readable text here (for example a scan, or a format only the full product imports). Text recognition for scans comes with the file filters.</p>
        </div>
      ) : (
        <div className="editor-body">
          <ol className="segments" aria-label="Segments">
            {visible.map((s) => {
              const n = segments.indexOf(s) + 1;
              const isActive = s.id === activeId;
              return (
                <li key={s.id} className={`seg seg-${s.status}${isActive ? " is-active" : ""}`} aria-current={isActive ? "true" : undefined}>
                  <span className="seg-num">{n}</span>
                  <div className="seg-source" lang={source} onClick={() => !isActive && activate(s.id)}>
                    <SourceText runs={s.source} hits={isActive ? hits : []} />
                  </div>
                  <div className="seg-target">
                    {isActive ? (
                      <TargetEditor
                        key={s.id}
                        ref={editor}
                        initial={s.target}
                        lang={target}
                        label={`Translation of segment ${n}, ${lang(target).name}`}
                        describedBy="seg-help"
                        onChange={(runs) => setTarget(runs)}
                        onKeyDown={onKeyDown}
                      />
                    ) : (
                      <button type="button" className="seg-target-view" lang={target} onClick={() => activate(s.id)} aria-label={`Segment ${n}: ${plain(s.target) || "not translated"}. ${STATUS_TEXT[s.status]}${s.warnings?.length ? `, ${plural(s.warnings.length, "warning", "warnings")}` : ""}. Edit`}>
                        <span className="seg-target-text">{s.target.length ? <RunsView runs={s.target} /> : <span className="missing">Not translated</span>}</span>
                      </button>
                    )}
                  </div>
                  <SegmentStatusCell s={s} />
                </li>
              );
            })}
          </ol>

          <aside className="results" aria-label="Translation results">
            <p id="seg-help" className="sr-only">
              Control Enter confirms and moves on. Control and a number inserts a numbered result. Control E adds a term. Control K searches the memory. F9 inserts the next tag.
            </p>
            {active && (
              <>
                <Section title="Translation results" count={results.length}>
                  {results.length === 0 ? (
                    <p className="muted">No memory matches or terms for this segment.</p>
                  ) : (
                    <ol className="result-list">
                      {results.map((r, i) => (
                        <li key={r.kind === "tm" ? `tm-${i}` : `t-${r.h.entry.id}`}>
                          <button type="button" className={`result${r === highlighted ? " is-selected" : ""}`} onClick={() => insertResult(r)} aria-keyshortcuts={i < 9 ? `Control+${i + 1}` : undefined}>
                            <span className="result-key" aria-hidden="true">
                              {i + 1}
                            </span>
                            {r.kind === "tm" ? <MatchBody m={r.m} source={source} target={target} /> : <TermBody h={r.h} source={source} target={target} />}
                          </button>
                        </li>
                      ))}
                    </ol>
                  )}
                </Section>

                <Section title="Checks" count={allIssues.length}>
                  {allIssues.length === 0 ? (
                    <p className="muted">{plain(active.target).trim() ? "No problems found." : "Checks run as you type and again when you confirm."}</p>
                  ) : (
                    <ul className="issues">
                      {allIssues.map((q) => (
                        <li key={q.text} className={`issue issue-${q.kind}`}>
                          <AlertTriangle size={14} aria-hidden="true" />
                          <span className="issue-text">{q.text}</span>
                          <button type="button" className="btn btn-quiet btn-tiny" onClick={() => dispatch({ type: "ignoreIssue", projectId, segmentId: active.id, text: q.text })} aria-label={`Ignore: ${q.text}`}>
                            Ignore
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  {(active.ignored?.length ?? 0) > 0 && <p className="help">{plural(active.ignored!.length, "warning", "warnings")} ignored for this segment.</p>}
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
                      <input id="lookup-q" autoFocus value={lookupText} onChange={(e) => setLookupText(e.target.value)} placeholder="Words in either language" />
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
                          <p className="muted">Nothing in the memory contains “{lookupText.trim()}”.</p>
                        ) : (
                          <ul className="lookup-list">
                            {concordanceHits.map((c, i) => (
                              <li key={i}>
                                <span lang={source}>{c.side === "source" ? <Highlight text={plain(c.unit.source)} start={c.start} end={c.end} /> : plain(c.unit.source)}</span>
                                <span lang={target} className="lookup-target">
                                  {c.side === "target" ? <Highlight text={plain(c.unit.target)} start={c.start} end={c.end} /> : plain(c.unit.target)}
                                </span>
                                <span className="result-meta">{unitMeta(c.unit)}</span>
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
                        dispatch({ type: "addTerm", id: `t-${Date.now().toString(36)}`, now: new Date().toISOString(), input: { projectId, source: src, target: tgt, note, segment: plain(active.source) } });
                        notify(`Added “${src}” to ${writeTermbase?.name ?? "the termbase"}`);
                        setCapture(null);
                        editor.current?.focus();
                      }}
                    />
                  ) : (
                    <div className="capture-intro">
                      <p className="muted">Select the term in the source and its translation in the target, then add it. The sentence is saved with it.</p>
                      <div className="tool-row">
                        <button type="button" className="btn btn-small" onClick={openCapture}>
                          <Plus size={14} aria-hidden="true" />
                          Add term
                          <kbd>Ctrl+E</kbd>
                        </button>
                        <button type="button" className="btn btn-small" onClick={openLookup}>
                          <Search size={14} aria-hidden="true" />
                          Search memory
                          <kbd>Ctrl+K</kbd>
                        </button>
                      </div>
                    </div>
                  )}
                </Section>

                <div className="tool-row">
                  <button type="button" className="btn btn-small" onClick={() => active && editor.current?.setRuns(active.source)}>
                    <Copy size={14} aria-hidden="true" />
                    Copy source
                    <kbd>Ctrl+Ins</kbd>
                  </button>
                  <button type="button" className="btn btn-small" onClick={insertNextTag} disabled={missingTags(active.source, active.target).length === 0}>
                    <Tag size={14} aria-hidden="true" />
                    Next tag
                    <kbd>F9</kbd>
                  </button>
                  <button type="button" className="btn btn-primary btn-small" onClick={confirmAndNext}>
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
                    <dd>Confirm, fill repetitions and go to the next open segment</dd>
                    <dt>Alt+↓ / Alt+↑</dt>
                    <dd>Next / previous segment</dd>
                    <dt>Ctrl+1 to 9</dt>
                    <dd>Insert result 1 to 9 (a memory match replaces the target, a term goes in at the caret)</dd>
                    <dt>Ctrl+↓ / Ctrl+↑, Ctrl+Space</dt>
                    <dd>Move through the results, insert the highlighted one</dd>
                    <dt>Ctrl+Insert</dt>
                    <dd>Copy source to target</dd>
                    <dt>F9</dt>
                    <dd>Insert the next missing tag</dd>
                    <dt>Ctrl+E</dt>
                    <dd>Add a term from the selections</dd>
                    <dt>Ctrl+K or F3</dt>
                    <dd>Search the memory for the selection</dd>
                  </dl>
                  <p className="help">These follow Trados, memoQ and Phrase where they agree. Editable in the full product.</p>
                </details>
              </>
            )}
          </aside>
        </div>
      )}
    </div>
  );
}

function unitMeta(u: TmUnit): string {
  return u.docSegment ? `This document, segment ${u.docSegment}` : `Memory, ${u.project}, ${u.by}, ${fmtDate(u.at)}`;
}

function SegmentStatusCell({ s }: { s: Segment }) {
  const warnings = s.warnings ?? [];
  return (
    <span className="seg-status" title={[STATUS_TEXT[s.status], ...warnings.map((x) => x.text)].join("\n")}>
      {s.status === "confirmed" ? <Check size={16} aria-label="Confirmed" /> : s.status === "draft" ? <PencilLine size={15} aria-label="Draft" /> : <CircleDashed size={15} aria-label="Not translated" />}
      {warnings.length > 0 && (
        <span className="seg-warn">
          <AlertTriangle size={13} aria-hidden="true" />
          <span className="sr-only">{plural(warnings.length, "warning", "warnings")}</span>
          <span aria-hidden="true">{warnings.length}</span>
        </span>
      )}
      {s.origin?.kind === "tm" && <span className="seg-origin">{s.origin.score}%</span>}
      {s.origin?.kind === "repetition" && (
        <span className="seg-origin">
          <Repeat2 size={13} aria-label="Repetition" />
        </span>
      )}
    </span>
  );
}

function MatchBody({ m, source, target }: { m: TmMatch; source: string; target: string }) {
  return (
    <>
      <span className={`result-score${m.score === 100 ? " is-exact" : ""}`}>{m.score}%</span>
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

function TermBody({ h, source, target }: { h: TermHit; source: string; target: string }) {
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
              <span className={t.status === "forbidden" ? "term-forbidden" : undefined}>
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

function Highlight({ text, start, end }: { text: string; start: number; end: number }) {
  return (
    <>
      {text.slice(0, start)}
      <mark>{text.slice(start, end)}</mark>
      {text.slice(end)}
    </>
  );
}

function Section({ title, count, children }: { title: string; count?: number; children: ReactNode }) {
  return (
    <section className="results-section" aria-label={title}>
      <h2 className="results-title">
        {title}
        {count !== undefined && count > 0 && <span className="results-count">{count}</span>}
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
          <span key={i} className={`tag-chip tag-${r.kind}`} role="img" aria-label={tagDescription(r)}>
            {chipText(r)}
          </span>
        ),
      )}
    </>
  );
}

/** Source text with tags as chips and recognised terms underlined. */
function SourceText({ runs, hits }: { runs: Run[]; hits: TermHit[] }) {
  const out: ReactNode[] = [];
  let offset = 0;
  runs.forEach((r, i) => {
    if (typeof r !== "string") {
      out.push(
        <span key={`t${i}`} className={`tag-chip tag-${r.kind}`} role="img" aria-label={tagDescription(r)}>
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
      if (a > pos) out.push(<span key={`${i}-${pos}`}>{r.slice(pos - start, a - start)}</span>);
      const tgt = h.targets.filter((t) => t.status !== "forbidden").map((t) => t.text).join(", ");
      out.push(
        <span key={`${i}-h${a}`} className="term-hit" title={`${h.termbase.name}: ${tgt || "no translation"}`}>
          {r.slice(a - start, b - start)}
        </span>,
      );
      pos = b;
    }
    if (pos < end) out.push(<span key={`${i}-${pos}`}>{r.slice(pos - start)}</span>);
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
      <input id="cap-src" ref={first} lang={sourceLang} value={src} onChange={(e) => setSrc(e.target.value)} placeholder="Select it in the source, or type" />
      <label htmlFor="cap-tgt">{lang(targetLang).name}</label>
      <input id="cap-tgt" ref={second} lang={targetLang} value={tgt} onChange={(e) => setTgt(e.target.value)} placeholder="Select it in your translation, or type" />
      <label htmlFor="cap-note">
        Note <span className="optional">optional</span>
      </label>
      <input id="cap-note" value={note} onChange={(e) => setNote(e.target.value)} />
      <p className="help">
        Saved as proposed to {termbaseName} <LocationBadge location={location} />, with this sentence.
      </p>
      <div className="row">
        <button type="submit" className="btn btn-primary btn-small" disabled={!valid}>
          Add term
        </button>
        <button type="button" className="btn btn-quiet btn-small" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
