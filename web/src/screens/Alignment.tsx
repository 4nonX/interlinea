import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { lang, type LangCode } from "../data";
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

/**
 * A document and its translation, done before, into units of a memory:
 * the core pairs their sentences, and the pairs are checked here, as lines
 * between the two texts, before they are added.
 */
export function AlignDocuments({
  name,
  sourceLang,
  targetLang,
  add,
  onClose,
  onAdded,
}: {
  name: string;
  sourceLang: LangCode;
  targetLang: LangCode;
  /** Adds the units to the memory, all or none. */
  add: (units: { source: CoreRun[]; target: CoreRun[] }[], overwrite: boolean) => Promise<void>;
  onClose: () => void;
  onAdded: (units: number) => void | Promise<void>;
}) {
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
      setResult(a);
      setLinks(linksOf(a));
      setPicked(null);
    } catch (err) {
      setProblem(text(err));
    } finally {
      setBusy(false);
    }
  }

  if (!result)
    return (
      <Modal title={`Align documents into ${name}`} onClose={onClose} wide>
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
          <label className="field">
            <span className="field-label">Document in {lang(sourceLang).name}</span>
            <input type="file" accept={ACCEPT} onChange={(e) => setSource(e.target.files?.[0] ?? null)} />
          </label>
          <label className="field">
            <span className="field-label">Its translation in {lang(targetLang).name}</span>
            <input type="file" accept={ACCEPT} onChange={(e) => setTarget(e.target.files?.[0] ?? null)} />
          </label>
          {problem && (
            <p className="error" role="alert">
              {problem}
            </p>
          )}
          <div className="row">
            <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary btn-small" disabled={!source || !target || busy}>
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
      setLinks(toggle(links, picked.side === "source" ? { source: picked.i, target: p.i } : { source: p.i, target: picked.i }));
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

  return (
    <Modal title={`Align documents into ${name}`} onClose={onClose} wide>
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
            <button
              type="button"
              className="btn btn-quiet btn-small"
              disabled={!picked || !unchecked(links, picked.side, picked.i)}
              onClick={() => {
                if (!picked) return;
                setLinks(confirm(links, picked.side, picked.i));
                setPicked(null);
              }}
            >
              Confirm
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
              setLinks(draw(links, l));
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
                      <path d={d} className="align-hit" onClick={() => setLinks(toggle(links, l))}>
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
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" onClick={() => setResult(null)}>
            Other documents
          </button>
          <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={!units.length || busy}
            onClick={() => {
              setBusy(true);
              setProblem("");
              add(
                units.map((u) => ({ source: joined(result.source, u.source), target: joined(result.target, u.target) })),
                overwrite,
              ).then(
                () => onAdded(units.length),
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
