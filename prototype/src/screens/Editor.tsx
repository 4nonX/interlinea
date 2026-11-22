import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { AlertTriangle, ArrowLeft, Check, CircleDashed, Copy, GitMerge, Keyboard, PencilLine, Plus, Tag } from "lucide-react";
import { lang } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges } from "../merge";
import { isTag, missingTags, plain, wordCount, TM_UNITS, type Run, type Segment } from "../editor/model";
import { lookup, qa, recognize, type TermHit, type TmMatch } from "../editor/match";
import { TargetEditor, chipText, tagDescription, type TargetEditorHandle } from "../editor/TargetEditor";
import { InLang, LangChip, LocationBadge, StatusBadge } from "../components/ui";
import { fmtDate, plural } from "../format";

const STATUS_TEXT: Record<Segment["status"], string> = { new: "Not translated", draft: "Draft", confirmed: "Confirmed" };

export function Editor({ projectId }: { projectId: string }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const project = state.projects.find((p) => p.id === projectId);
  const segments = project?.segments ?? [];
  const [activeId, setActiveId] = useState(() => segments.find((s) => s.status !== "confirmed")?.id ?? segments[0]?.id ?? "");
  const [filter, setFilter] = useState<"all" | "todo">("all");
  const [capture, setCapture] = useState<{ source: string; target: string } | null>(null);
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

  const active = segments.find((s) => s.id === activeId);
  const index = segments.findIndex((s) => s.id === activeId);
  const hits = useMemo(() => (active ? recognize(plain(active.source), source, target, termbases, state.entries) : []), [active, source, target, termbases, state.entries]);
  const matches = useMemo(() => (active && project?.id === "p-hofmann" ? lookup(active.source, TM_UNITS) : []), [active, project?.id]);
  const issues = useMemo(() => (active ? qa(active.source, active.target, hits, source, target) : []), [active, hits, source, target]);

  // Track a selection made in the active source text, for "Add term".
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

  // Announce the segment to screen readers when it becomes active.
  useEffect(() => {
    if (!active) return;
    const parts = [`Segment ${index + 1} of ${segments.length}`, STATUS_TEXT[active.status]];
    if (matches.length) parts.push(`${plural(matches.length, "memory match", "memory matches")}, best ${matches[0]!.score}%`);
    if (hits.length) parts.push(plural(hits.length, "term", "terms"));
    setAnnouncement(parts.join(". ") + ".");
    sourceSelection.current = "";
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

  const visible = filter === "all" ? segments : segments.filter((s) => s.status !== "confirmed" || s.id === activeId);
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
    const i = segments.findIndex((s) => s.id === activeId);
    const next = segments[i + delta];
    if (next) activate(next.id);
  }

  function confirmAndNext() {
    if (!active) return;
    if (!plain(active.target).trim()) {
      setAnnouncement("The target is empty. Type a translation or insert a match first.");
      return;
    }
    dispatch({ type: "setTarget", projectId, segmentId: active.id, target: active.target, status: "confirmed" });
    const i = segments.findIndex((s) => s.id === activeId);
    const next = segments.slice(i + 1).find((s) => s.status !== "confirmed") ?? segments.find((s) => s.status !== "confirmed" && s.id !== activeId);
    if (next) activate(next.id);
    else {
      notify("All segments confirmed");
      setAnnouncement("All segments are confirmed.");
    }
  }

  function insertMatch(m: TmMatch) {
    editor.current?.setRuns(m.unit.target);
    setTarget(m.unit.target, "draft", { kind: "tm", score: m.score });
  }

  function insertTerm(h: TermHit) {
    const t = h.targets.find((x) => x.status !== "forbidden");
    if (t) editor.current?.insertText(t.text);
  }

  function insertNextTag() {
    if (!active) return;
    const next = missingTags(active.source, active.target)[0];
    if (next) editor.current?.insertTag(next);
    else setAnnouncement("All tags are already in the target.");
  }

  function openCapture() {
    setCapture({ source: sourceSelection.current, target: editor.current?.selectedText() ?? "" });
  }

  // The numbered results list: memory matches first, then terms (as in memoQ).
  const results: ({ kind: "tm"; m: TmMatch } | { kind: "term"; h: TermHit })[] = [
    ...matches.map((m) => ({ kind: "tm" as const, m })),
    ...hits.filter((h) => h.targets.some((t) => t.status !== "forbidden")).map((h) => ({ kind: "term" as const, h })),
  ];

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key === "Enter") {
      e.preventDefault();
      confirmAndNext();
    } else if (e.altKey && e.key === "ArrowDown") {
      e.preventDefault();
      move(1);
    } else if (e.altKey && e.key === "ArrowUp") {
      e.preventDefault();
      move(-1);
    } else if (mod && e.key === "Insert") {
      e.preventDefault();
      if (active) editor.current?.setRuns(active.source);
    } else if (mod && e.shiftKey && /^Digit[1-9]$/.test(e.code)) {
      const r = results[Number(e.code.slice(5)) - 1];
      if (r) {
        e.preventDefault();
        if (r.kind === "tm") insertMatch(r.m);
        else insertTerm(r.h);
      }
    } else if (mod && e.shiftKey && e.code === "KeyE") {
      e.preventDefault();
      openCapture();
    } else if (e.key === "F9") {
      e.preventDefault();
      insertNextTag();
    }
  }

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
                      <button type="button" className="seg-target-view" lang={target} onClick={() => activate(s.id)} aria-label={`Segment ${n}: ${plain(s.target) || "not translated"}. Edit`}>
                        <span className="seg-target-text">{s.target.length ? <RunsView runs={s.target} /> : <span className="missing">Not translated</span>}</span>
                      </button>
                    )}
                  </div>
                  <span className="seg-status" title={STATUS_TEXT[s.status]}>
                    {s.status === "confirmed" ? <Check size={16} aria-label="Confirmed" /> : s.status === "draft" ? <PencilLine size={15} aria-label="Draft" /> : <CircleDashed size={15} aria-label="Not translated" />}
                    {s.origin?.kind === "tm" && <span className="seg-origin">{s.origin.score}%</span>}
                  </span>
                </li>
              );
            })}
          </ol>

          <aside className="results" aria-label="Translation results">
            <p id="seg-help" className="sr-only">
              Control Enter confirms and moves on. Control Shift and a number inserts a numbered result. Control Shift E adds a term. F9 inserts the next tag.
            </p>
            {active && (
              <>
                <Section title="Translation results" count={results.length}>
                  {results.length === 0 ? (
                    <p className="muted">No memory matches or terms for this segment.</p>
                  ) : (
                    <ol className="result-list">
                      {results.map((r, i) =>
                        r.kind === "tm" ? (
                          <li key={`tm-${i}`}>
                            <button type="button" className="result" onClick={() => insertMatch(r.m)}>
                              <span className="result-key" aria-hidden="true">
                                {i + 1}
                              </span>
                              <span className={`result-score${r.m.score === 100 ? " is-exact" : ""}`}>{r.m.score}%</span>
                              <span className="result-body">
                                <span className="result-source" lang={source}>
                                  {r.m.diff.map((d, k) => (
                                    <span key={k}>
                                      {d.space && " "}
                                      {d.changed ? <mark>{d.text}</mark> : d.text}
                                    </span>
                                  ))}
                                </span>
                                <span className="result-target" lang={target}>
                                  <RunsView runs={r.m.unit.target} />
                                </span>
                                <span className="result-meta">
                                  Memory, {r.m.unit.project}, {r.m.unit.by}, {fmtDate(r.m.unit.at)}
                                </span>
                              </span>
                            </button>
                          </li>
                        ) : (
                          <li key={`t-${r.h.entry.id}`}>
                            <button type="button" className="result" onClick={() => insertTerm(r.h)}>
                              <span className="result-key" aria-hidden="true">
                                {i + 1}
                              </span>
                              <span className="result-score result-term">
                                <Tag size={13} aria-hidden="true" />
                                <span className="sr-only">Term</span>
                              </span>
                              <span className="result-body">
                                <span className="result-source">
                                  <InLang code={source}>{r.h.term.text}</InLang>
                                </span>
                                <span className="result-target">
                                  {r.h.targets.map((t, k) => (
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
                                <span className="result-meta">{r.h.termbase.name}</span>
                              </span>
                            </button>
                          </li>
                        ),
                      )}
                    </ol>
                  )}
                </Section>

                <Section title="Checks" count={issues.length}>
                  {issues.length === 0 ? (
                    <p className="muted">{plain(active.target).trim() ? "No problems found." : "Checks run as you type."}</p>
                  ) : (
                    <ul className="issues">
                      {issues.map((q) => (
                        <li key={q.text} className={`issue issue-${q.kind}`}>
                          <AlertTriangle size={14} aria-hidden="true" />
                          {q.text}
                        </li>
                      ))}
                    </ul>
                  )}
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
                        dispatch({ type: "addTerm", id: `t-${Date.now().toString(36)}`, now: new Date().toISOString(), input: { projectId, source: src, target: tgt, note, segment: plain(active.source) } });
                        notify(`Added “${src}” to ${writeTermbase?.name ?? "the termbase"}`);
                        setCapture(null);
                        editor.current?.focus();
                      }}
                    />
                  ) : (
                    <div className="capture-intro">
                      <p className="muted">Select the term in the source and its translation in the target, then add it. The sentence is saved with it.</p>
                      <button type="button" className="btn btn-small" onClick={openCapture}>
                        <Plus size={14} aria-hidden="true" />
                        Add term
                        <kbd>Ctrl+Shift+E</kbd>
                      </button>
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
                    <dd>Confirm and go to the next open segment</dd>
                    <dt>Alt+↓ / Alt+↑</dt>
                    <dd>Next / previous segment</dd>
                    <dt>Ctrl+Shift+1 to 9</dt>
                    <dd>Insert result 1 to 9 (memory match replaces, term inserts at the caret)</dd>
                    <dt>Ctrl+Insert</dt>
                    <dd>Copy source to target</dd>
                    <dt>F9</dt>
                    <dd>Insert the next missing tag</dd>
                    <dt>Ctrl+Shift+E</dt>
                    <dd>Add a term from the selections</dd>
                  </dl>
                  <p className="help">The desktop app uses Ctrl+1 to 9 like memoQ; browsers reserve those for tabs.</p>
                </details>
              </>
            )}
          </aside>
        </div>
      )}
    </div>
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
