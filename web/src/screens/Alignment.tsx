import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { LANGUAGES, lang, type LangCode } from "../data";
import { alignDocuments, loadCore, type Alignment } from "../core";
import { coreToRuns, plain, type CoreRun } from "../editor/model";
import { confirm, draw, fixedOf, joined, linkKey, linksOf, needsLook, scoresOf, toggle, unchecked, unitsOf, type Link, type Unit } from "../align";
import { fmtNum, plural } from "../format";
import { LangChip } from "../components/ui";
import { Modal } from "./TermbaseDialogs";

const ACCEPT = ".docx,.docm,.pptx,.pptm,.ppsx,.xlsx,.xlsm,.html,.htm,.xhtml,.idml,.txt,.md,text/plain";

const text = (e: unknown) => (e instanceof Error ? e.message : String(e));

type Side = "source" | "target";
interface Pick {
  side: Side;
  i: number;
}

/** Where the segments are on the board, to draw the lines between them. */
interface Places {
  source: number[];
  target: number[];
  x1: number;
  x2: number;
  height: number;
}

type AddUnits = (units: { source: CoreRun[]; target: CoreRun[] }[], overwrite: boolean) => Promise<void>;

/** A memory the aligned units go into. */
export interface AlignInto {
  name: string;
  sourceLang: LangCode;
  targetLang: LangCode;
  /** Adds the units to the memory, all or none. */
  add: AddUnits;
}

/** The memories of one place to choose from, and how a new one is made there. */
export interface AlignChoice {
  memories: AlignInto[];
  /** Whether a memory of this name is there already. */
  taken: (name: string) => boolean;
  make: (name: string, sourceLang: LangCode, targetLang: LangCode) => Promise<AlignInto>;
}

/** The lines and what they were paired from, to go back to. */
interface Snap {
  result: Alignment;
  links: Link[];
}

const NEW = "\u0000new";

/**
 * A document and its translation, done before, into units of a memory:
 * the core pairs their sentences, and the pairs are checked here, as lines
 * between the two texts, before they are added. Opened from a memory the
 * units go into that one; opened from the page, the memory is chosen or
 * made here.
 */
export function AlignDocuments({
  into,
  choice,
  onClose,
  onAdded,
}: {
  into?: AlignInto;
  choice?: AlignChoice;
  onClose: () => void;
  onAdded: (units: number, memory: string) => void | Promise<void>;
}) {
  const [chosen, setChosen] = useState(() => choice?.memories[0]?.name ?? NEW);
  const [newName, setNewName] = useState("");
  const [newSource, setNewSource] = useState<LangCode>("de-DE");
  const [newTarget, setNewTarget] = useState<LangCode>("en-GB");
  // A memory made here is made once, even if adding to it has to be tried again.
  const made = useRef<AlignInto | null>(null);
  const existing = into ?? choice?.memories.find((m) => m.name === chosen);
  const name = existing?.name ?? newName.trim();
  const sourceLang = existing?.sourceLang ?? newSource;
  const targetLang = existing?.targetLang ?? newTarget;
  const destination = existing
    ? ""
    : !name
      ? "Give the memory a name."
      : choice?.taken(name) && made.current?.name !== name
        ? "A memory with this name exists already."
        : newSource === newTarget
          ? "Source and target language must differ."
          : "";
  const [past, setPast] = useState<Snap[]>([]);
  const [future, setFuture] = useState<Snap[]>([]);
  // Closing or choosing other documents with work on the lines asks first.
  const [leaving, setLeaving] = useState<"close" | "other" | null>(null);
  const [source, setSource] = useState<File | null>(null);
  const [target, setTarget] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [result, setResult] = useState<Alignment | null>(null);
  const [links, setLinks] = useState<Link[]>([]);
  const [picked, setPicked] = useState<Pick | null>(null);
  const [overwrite, setOverwrite] = useState(false);
  const [places, setPlaces] = useState<Places | null>(null);
  const board = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // Where a line is being pulled from.
  const pulling = useRef<Pick | null>(null);
  const scores = useMemo(() => (result ? scoresOf(result) : new Map<string, number>()), [result]);

  const measure = useCallback(() => {
    const b = board.current;
    if (!b) return;
    const top = b.getBoundingClientRect().top;
    const mid = (side: Side) => [...b.querySelectorAll<HTMLElement>(`[data-side=${side}]`)].map((el) => {
      const r = el.getBoundingClientRect();
      return r.top - top + r.height / 2;
    });
    const edge = (side: Side) => b.querySelector<HTMLElement>(`.align-column[data-column=${side}]`)?.getBoundingClientRect();
    const left = b.getBoundingClientRect().left;
    setPlaces({ source: mid("source"), target: mid("target"), x1: (edge("source")?.right ?? 0) - left, x2: (edge("target")?.left ?? 0) - left, height: b.scrollHeight });
  }, []);
  useLayoutEffect(() => {
    if (!result || !board.current) return;
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(board.current);
    return () => ro.disconnect();
  }, [result, measure]);

  async function pair(fixed: Unit[] = []) {
    if (!source || !target) return;
    setBusy(true);
    setProblem("");
    try {
      const c = await loadCore();
      const a = alignDocuments(
        c,
        { name: source.name, bytes: new Uint8Array(await source.arrayBuffer()), lang: sourceLang },
        { name: target.name, bytes: new Uint8Array(await target.arrayBuffer()), lang: targetLang },
        fixed,
      );
      // Paired again, the lines before can be gone back to; paired anew, there is nothing before.
      setPast(fixed.length && result ? [...past, { result, links }] : []);
      setFuture([]);
      setResult(a);
      setLinks(linksOf(a));
      setPicked(null);
    } catch (err) {
      setProblem(text(err));
    } finally {
      setBusy(false);
    }
  }

  // A change of the lines, which can be undone.
  const change = (next: Link[]) => {
    if (!result) return;
    setPast([...past, { result, links }]);
    setFuture([]);
    setLinks(next);
  };
  const step = (from: Snap[], setFrom: (s: Snap[]) => void, to: Snap[], setTo: (s: Snap[]) => void) => {
    const snap = from[from.length - 1];
    if (!snap || !result) return;
    setFrom(from.slice(0, -1));
    setTo([...to, { result, links }]);
    setResult(snap.result);
    setLinks(snap.links);
    setPicked(null);
  };
  const undo = () => step(past, setPast, future, setFuture);
  const redo = () => step(future, setFuture, past, setPast);
  const keys = useRef({ undo, redo });
  keys.current = { undo, redo };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
      const k = e.key.toLowerCase();
      if (k === "z" && !e.shiftKey) keys.current.undo();
      else if (k === "y" || (k === "z" && e.shiftKey)) keys.current.redo();
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  if (!result)
    return (
      <Modal title={into ? `Align documents into ${into.name}` : "Align documents"} onClose={onClose} wide>
        <form
          className="dialog-form"
          onSubmit={(e) => {
            e.preventDefault();
            void pair();
          }}
        >
          <p className="help">
            Choose a document and its translation. Their sentences are paired, and you check the pairs before they go into the memory. The two files need not be
            of the same format: DOCX, PPTX, XLSX, HTML, IDML or text.
          </p>
          {choice && (
            <label className="field">
              <span className="field-label">Memory the units go into</span>
              <select value={chosen} onChange={(e) => setChosen(e.target.value)}>
                {choice.memories.map((m) => (
                  <option key={m.name} value={m.name}>
                    {m.name} ({m.sourceLang} to {m.targetLang})
                  </option>
                ))}
                <option value={NEW}>A new memory</option>
              </select>
            </label>
          )}
          {choice && !existing && (
            <div className="field-grid">
              <label className="field">
                <span className="field-label">Name of the new memory</span>
                <input value={newName} onChange={(e) => setNewName(e.target.value)} aria-invalid={!!destination && !!newName} />
              </label>
              <span />
              <label className="field">
                <span className="field-label">Source language</span>
                <select value={newSource} onChange={(e) => setNewSource(e.target.value)}>
                  {LANGUAGES.map((l) => (
                    <option key={l.code} value={l.code}>
                      {l.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="field-label">Target language</span>
                <select value={newTarget} onChange={(e) => setNewTarget(e.target.value)}>
                  {LANGUAGES.map((l) => (
                    <option key={l.code} value={l.code}>
                      {l.name}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          )}
          <label className="field">
            <span className="field-label">Document in {lang(sourceLang).name}</span>
            <input type="file" accept={ACCEPT} onChange={(e) => setSource(e.target.files?.[0] ?? null)} />
          </label>
          <label className="field">
            <span className="field-label">Its translation in {lang(targetLang).name}</span>
            <input type="file" accept={ACCEPT} onChange={(e) => setTarget(e.target.files?.[0] ?? null)} />
          </label>
          {(problem || (destination && newName)) && (
            <p className="error" role="alert">
              {problem || destination}
            </p>
          )}
          <div className="row">
            <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary btn-small" disabled={!source || !target || busy || !!destination}>
              {busy ? "Pairing…" : "Pair the sentences"}
            </button>
          </div>
        </form>
      </Modal>
    );

  const units = unitsOf(links);
  const fixed = fixedOf(links);
  // A pair that was checked needs no look any more.
  const look = units.filter((u) => needsLook(u, scores) && !fixed.some((f) => f.source[0] === u.source[0]));
  const paired = { source: new Set(links.map((l) => l.source)), target: new Set(links.map((l) => l.target)) };
  const alone = result.source.length - paired.source.size + result.target.length - paired.target.size;
  // The lines of a pair that needs a look, to draw them otherwise.
  const lookLines = new Set(look.flatMap((u) => links.filter((l) => u.source.includes(l.source)).map(linkKey)));

  // A segment chosen, then one of the other text: the line between them is
  // drawn, or taken away if it is there.
  function choose(p: Pick) {
    if (picked && picked.side !== p.side) {
      change(toggle(links, picked.side === "source" ? { source: picked.i, target: p.i } : { source: p.i, target: picked.i }));
      setPicked(null);
    } else setPicked(picked && picked.i === p.i ? null : p);
  }

  // The next segment without a line, or pair that needs a look, below what is in view.
  function next() {
    const s = scroller.current;
    if (!s || !places) return;
    const ys = [
      ...result!.source.map((_, i) => (paired.source.has(i) ? -1 : (places.source[i] ?? -1))),
      ...result!.target.map((_, i) => (paired.target.has(i) ? -1 : (places.target[i] ?? -1))),
      ...look.map((u) => places.source[u.source[0] ?? 0] ?? -1),
    ]
      .filter((y) => y >= 0)
      .sort((a, b) => a - b);
    const from = s.scrollTop + s.clientHeight / 2;
    const y = ys.find((v) => v > from + 1) ?? ys[0];
    if (y !== undefined) s.scrollTop = Math.max(0, y - s.clientHeight / 2);
  }

  const column = (side: Side) => {
    const segments = side === "source" ? result.source : result.target;
    const what = side === "source" ? "Sentence" : "Translation";
    return (
      <div className="align-column" data-column={side} lang={side === "source" ? sourceLang : targetLang}>
        {segments.map((runs, i) => {
          const on = picked?.side === side && picked.i === i;
          const lone = !paired[side].has(i);
          return (
            <button
              key={i}
              type="button"
              className={`align-seg${on ? " is-picked" : ""}${lone ? " is-alone" : ""}`}
              data-side={side}
              data-i={i}
              aria-pressed={on}
              aria-label={`${what} ${i + 1}${lone ? ", no pair" : ""}: ${plain(coreToRuns(runs))}`}
              onClick={() => choose({ side, i })}
              onPointerDown={() => {
                pulling.current = { side, i };
              }}
            >
              <span className="align-n" aria-hidden="true">
                {i + 1}
              </span>
              <span>{plain(coreToRuns(runs))}</span>
              <span className="align-port" aria-hidden="true" />
            </button>
          );
        })}
      </div>
    );
  };

  const worked = past.length > 0 || future.length > 0;
  const leave = (how: "close" | "other") => {
    if (worked && leaving !== how) {
      setLeaving(how);
      return;
    }
    setLeaving(null);
    if (how === "close") onClose();
    else {
      setResult(null);
      setPast([]);
      setFuture([]);
    }
  };

  return (
    <Modal title={`Align documents into ${name}`} onClose={() => leave("close")} wide>
      <div className="dialog-form align-review">
        <p role="status">
          {plural(units.length, "unit", "units")} from {fmtNum(result.source.length)} and {plural(result.target.length, "segment", "segments")}.{" "}
          {fixed.length > 0 && `${plural(fixed.length, "unit is", "units are")} checked. `}
          {alone > 0 && `${plural(alone, "segment has", "segments have")} no pair and ${alone === 1 ? "is" : "are"} left out. `}
          {look.length > 0 && `${plural(look.length, "pair needs", "pairs need")} a look: the lengths or numbers do not fit.`}
          {alone === 0 && look.length === 0 && "All of them fit."}
        </p>
        <p className="help">
          A line joins a sentence and its translation, from the point on the one to the point on the other; a ring instead of a point means nothing is joined
          there. To draw a line, choose a sentence and then its translation, or pull from one point to the other; the same takes a line away, as does choosing
          the line. Sentences joined by lines become one unit. A dotted line was paired for you; one you draw or confirm is solid, and stays when the rest is
          paired again around it.
        </p>
        <div className="align-head">
          <LangChip code={sourceLang} />
          <span className="row">
            <button type="button" className="btn btn-quiet btn-small" disabled={!past.length} onClick={undo} title="Ctrl+Z">
              Undo
            </button>
            <button type="button" className="btn btn-quiet btn-small" disabled={!future.length} onClick={redo} title="Ctrl+Y">
              Redo
            </button>
            <button
              type="button"
              className="btn btn-quiet btn-small"
              disabled={!picked || !unchecked(links, picked.side, picked.i)}
              onClick={() => {
                if (!picked) return;
                change(confirm(links, picked.side, picked.i));
                setPicked(null);
              }}
            >
              Confirm
            </button>
            <button type="button" className="btn btn-quiet btn-small" disabled={links.every((l) => l.mine)} onClick={() => change(links.map((l) => ({ ...l, mine: true })))}>
              Confirm all
            </button>
            <button type="button" className="btn btn-quiet btn-small" disabled={busy || fixed.length === 0 || (fixed.length === units.length && alone === 0)} onClick={() => void pair(fixed)}>
              {busy ? "Pairing…" : "Pair the rest again"}
            </button>
            <button type="button" className="btn btn-quiet btn-small" disabled={alone === 0 && look.length === 0} onClick={next}>
              Next to look at
            </button>
          </span>
          <LangChip code={targetLang} />
        </div>
        <div className="align-scroll" ref={scroller}>
          <div
            className="align-board"
            ref={board}
            onPointerUp={(e) => {
              const from = pulling.current;
              pulling.current = null;
              const el = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>("[data-side]");
              if (!from || !el) return;
              const to = { side: el.dataset.side as Side, i: Number(el.dataset.i) };
              if (to.side === from.side) return;
              const l = from.side === "source" ? { source: from.i, target: to.i } : { source: to.i, target: from.i };
              if (!links.some((x) => linkKey(x) === linkKey(l) && x.mine)) change(draw(links, l));
              setPicked(null);
            }}
          >
            {column("source")}
            <div className="align-gutter" />
            {column("target")}
            {places && (
              <svg className="align-lines" width="100%" height={places.height} aria-label="Lines" role="group">
                {links.map((l) => {
                  const y1 = places.source[l.source];
                  const y2 = places.target[l.target];
                  if (y1 === undefined || y2 === undefined) return null;
                  const k = linkKey(l);
                  // A curve that leaves the sentence and reaches the translation level:
                  // lines to one segment stay apart, and each end shows whose it is.
                  const mid = (places.x1 + places.x2) / 2;
                  const d = `M ${places.x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${places.x2} ${y2}`;
                  return (
                    <g key={k} data-line={k}>
                      <path d={d} className={`align-line${l.mine ? " is-mine" : ""}${lookLines.has(k) ? " is-look" : ""}`} />
                      <path d={d} className="align-hit" onClick={() => change(toggle(links, l))}>
                        <title>Take this line away</title>
                      </path>
                    </g>
                  );
                })}
              </svg>
            )}
          </div>
        </div>
        <label className="check">
          <input type="checkbox" checked={overwrite} onChange={(e) => setOverwrite(e.target.checked)} /> Replace other translations of these sentences
        </label>
        {problem && (
          <p className="error" role="alert">
            {problem}
          </p>
        )}
        {leaving && (
          <div className="row align-leave" role="alertdialog" aria-label="Leave the alignment">
            <span>{leaving === "close" ? "Close without adding? What you did to the lines is lost." : "Choose other documents? What you did to the lines is lost."}</span>
            <button type="button" className="btn btn-small" autoFocus onClick={() => setLeaving(null)}>
              Stay
            </button>
            <button type="button" className="btn btn-danger btn-small" onClick={() => leave(leaving)}>
              {leaving === "close" ? "Close" : "Other documents"}
            </button>
          </div>
        )}
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" disabled={!!leaving} onClick={() => leave("other")}>
            Other documents
          </button>
          <button type="button" className="btn btn-quiet btn-small" disabled={!!leaving} onClick={() => leave("close")}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={!units.length || busy || !!leaving}
            onClick={() => {
              setBusy(true);
              setProblem("");
              const send = units.map((u) => ({ source: joined(result.source, u.source), target: joined(result.target, u.target) }));
              (async () => {
                const to = existing ?? made.current ?? (made.current = await choice!.make(name, sourceLang, targetLang));
                await to.add(send, overwrite);
              })().then(
                () => onAdded(units.length, name),
                (err) => {
                  setProblem(text(err));
                  setBusy(false);
                },
              );
            }}
          >
            Add {plural(units.length, "unit", "units")} to {name}
          </button>
        </div>
      </div>
    </Modal>
  );
}
