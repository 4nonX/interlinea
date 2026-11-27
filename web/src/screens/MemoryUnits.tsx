import { useRef, useState, type FormEvent } from "react";
import { Columns2, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { lang, type LangCode } from "../data";
import { coreToRuns, plain, runsToCore, type CoreRun, type Run } from "../editor/model";
import { chipText, TargetEditor, tagDescription } from "../editor/TargetEditor";
import { fmtDate, fmtNum, plural } from "../format";
import { LangChip } from "../components/ui";
import { ConfirmButton, Modal } from "./TermbaseDialogs";
import { AlignDocuments } from "./Alignment";
import { tl, tlx } from "../i18n";

/** A unit as a search finds it. */
export interface FoundUnit {
  id: number;
  source: CoreRun[];
  target: CoreRun[];
  changedBy?: string;
  changedAt?: string;
  usageCount?: number;
  /** What a save is checked against, where the memory counts versions. */
  version?: number;
  attributes?: Record<string, string>;
  notes?: string[];
}

/** Where a memory's units are kept: the team server, or the core in the page. */
export interface UnitsBackend {
  search(query: string, inTarget: boolean): Promise<FoundUnit[]>;
  /** With overwrite, the unit replaces the other translations of its sentence. */
  add(source: string, target: string, overwrite: boolean): Promise<void>;
  /** Adds units with their formatting, all or none. */
  addMany(units: { source: CoreRun[]; target: CoreRun[] }[], overwrite: boolean): Promise<void>;
  /** Throws SavedElsewhere when someone else changed the unit in the meantime. */
  change(unit: FoundUnit, target: CoreRun[]): Promise<void>;
  remove(unit: FoundUnit): Promise<void>;
}

/** Someone else changed the unit since it was found. */
export class SavedElsewhere extends Error {}

const text = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** Content to read, its tags as the editor shows them. */
function Content({ runs }: { runs: Run[] }) {
  return (
    <>
      {runs.map((r, i) =>
        typeof r === "string" ? (
          r
        ) : (
          <span key={i} className={`tag-chip tag-${r.kind}`} role="img" aria-label={tagDescription(r)}>
            {chipText(r)}
          </span>
        ),
      )}
    </>
  );
}

/**
 * The units of a memory: searched in the sentences or the translations,
 * and, for those who may edit it, a unit added, its translation changed,
 * or the unit deleted.
 */
export function MemoryUnits({
  name,
  sourceLang,
  targetLang,
  edit,
  backend,
  notify,
  onChanged,
}: {
  name: string;
  sourceLang: LangCode;
  targetLang: LangCode;
  edit: boolean;
  backend: UnitsBackend;
  notify: (text: string) => void;
  /** The number of units changed. */
  onChanged: () => void | Promise<void>;
}) {
  const [query, setQuery] = useState("");
  const [inTarget, setInTarget] = useState(false);
  const [hits, setHits] = useState<FoundUnit[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [aligning, setAligning] = useState(false);
  const [changing, setChanging] = useState<FoundUnit | null>(null);
  // What was searched last, to search again after a unit changed.
  const last = useRef<{ query: string; inTarget: boolean } | null>(null);

  async function run(q: string, t: boolean) {
    try {
      setHits(await backend.search(q, t));
      last.current = { query: q, inTarget: t };
    } catch (err) {
      notify(text(err));
    }
  }
  const again = async () => {
    if (last.current) await run(last.current.query, last.current.inTarget);
  };

  return (
    <>
      <form
        className="role-add unit-search"
        aria-label={tl("Search {name}", { name })}
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          if (query.trim()) void run(query, inTarget);
        }}
      >
        <label className="field">
          <span className="field-label">{tl("Search the units")}</span>
          <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <label className="check">
          <input type="checkbox" checked={inTarget} onChange={(e) => setInTarget(e.target.checked)} />{" "}{tl("In the translations")}
        </label>
        <button type="submit" className="btn" disabled={!query.trim()}>
          <Search size={15} aria-hidden="true" />{" "}{tl("Search")}
        </button>
        {edit && (
          <button type="button" className="btn unit-add" onClick={() => setAdding(true)}>
            <Plus size={15} aria-hidden="true" />{" "}{tl("Add a unit")}
          </button>
        )}
        {edit && (
          <button type="button" className="btn" onClick={() => setAligning(true)}>
            <Columns2 size={15} aria-hidden="true" />{" "}{tl("Align documents")}
          </button>
        )}
      </form>
      {hits &&
        (hits.length === 0 ? (
          <p className="muted">{tl("Nothing found.")}</p>
        ) : (
          <>
            <table className="table" aria-label={tl("Units found")}>
              <thead>
                <tr>
                  <th scope="col">
                    <LangChip code={sourceLang} />
                  </th>
                  <th scope="col">
                    <LangChip code={targetLang} />
                  </th>
                  <th scope="col">{tl("Changed")}</th>
                  {edit && (
                    <th scope="col">
                      <span className="sr-only">{tl("Actions")}</span>
                    </th>
                  )}
                </tr>
              </thead>
              <tbody>
                {hits.map((u) => {
                  const said = plain(coreToRuns(u.source));
                  return (
                    <tr key={u.id}>
                      <td>{said}</td>
                      <td>{plain(coreToRuns(u.target))}</td>
                      <td className="cell-sub unit-changed">{[u.changedBy, u.changedAt && fmtDate(u.changedAt)].filter(Boolean).join(", ")}</td>
                      {edit && (
                        <td className="cell-action">
                          <span className="row">
                            <button
                              type="button"
                              className="btn btn-quiet btn-small"
                              aria-label={tl("Change the translation of “{said}”", { said })}
                              onClick={() => setChanging(u)}
                            >
                              <Pencil size={14} aria-hidden="true" />{" "}{tl("Change")}
                            </button>
                            <ConfirmButton
                              label={
                                <>
                                  <Trash2 size={14} aria-hidden="true" />
                                  <span className="sr-only">{tlx("Delete the unit “{said}”", { said: said })}</span>
                                </>
                              }
                              confirm={tl("Delete this unit?")}
                              action={tl("Delete")}
                              onConfirm={() =>
                                void backend.remove(u).then(
                                  async () => {
                                    notify(tl("Deleted the unit"));
                                    await again();
                                    await onChanged();
                                  },
                                  (err) => notify(text(err)),
                                )
                              }
                            />
                          </span>
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {hits.length >= 50 && <p className="cell-sub">{tl("The first 50 units found. Search for more of the sentence to narrow it down.")}</p>}
          </>
        ))}
      {adding && (
        <AddUnit
          name={name}
          sourceLang={sourceLang}
          targetLang={targetLang}
          backend={backend}
          onClose={() => setAdding(false)}
          onAdded={async () => {
            setAdding(false);
            notify(tl("Added the unit to {name}", { name }));
            await again();
            await onChanged();
          }}
        />
      )}
      {aligning && (
        <AlignDocuments
          into={{ name, sourceLang, targetLang, add: backend.addMany }}
          onClose={() => setAligning(false)}
          onAdded={async (n) => {
            setAligning(false);
            notify(tl("Added {n} to {name}", { n: plural(n, "unit", "units"), name }));
            await again();
            await onChanged();
          }}
        />
      )}
      {changing && (
        <ChangeUnit
          unit={changing}
          sourceLang={sourceLang}
          targetLang={targetLang}
          backend={backend}
          onClose={() => setChanging(null)}
          onSaved={async () => {
            setChanging(null);
            notify(tl("Saved the unit"));
            await again();
          }}
        />
      )}
    </>
  );
}

/** A new unit: the sentence and its translation, as plain text. */
function AddUnit({
  name,
  sourceLang,
  targetLang,
  backend,
  onClose,
  onAdded,
}: {
  name: string;
  sourceLang: LangCode;
  targetLang: LangCode;
  backend: UnitsBackend;
  onClose: () => void;
  onAdded: () => Promise<void>;
}) {
  const [source, setSource] = useState("");
  const [target, setTarget] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const [problem, setProblem] = useState("");
  return (
    <Modal title={tl("Add a unit to {name}", { name })} onClose={onClose} wide>
      <form
        className="dialog-form"
        onSubmit={(e) => {
          e.preventDefault();
          backend.add(source.trim(), target.trim(), overwrite).then(onAdded, (err) => setProblem(text(err)));
        }}
      >
        <label className="field">
          <span className="field-label">{lang(sourceLang).name}</span>
          <textarea rows={3} lang={sourceLang} value={source} autoFocus onChange={(e) => setSource(e.target.value)} />
        </label>
        <label className="field">
          <span className="field-label">{lang(targetLang).name}</span>
          <textarea rows={3} lang={targetLang} value={target} onChange={(e) => setTarget(e.target.value)} />
        </label>
        <label className="check">
          <input type="checkbox" checked={overwrite} onChange={(e) => setOverwrite(e.target.checked)} />{" "}{tl("Replace other translations of this sentence")}
        </label>
        <p className="help">{tl("Without it, a sentence that is there already keeps its translations and gets this one as well.")}</p>
        {problem && (
          <p className="error" role="alert">
            {problem}
          </p>
        )}
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
            {tl("Cancel")}
          </button>
          <button type="submit" className="btn btn-primary btn-small" disabled={!source.trim() || !target.trim()}>
            {tl("Add the unit")}
          </button>
        </div>
      </form>
    </Modal>
  );
}

/** A unit found, to change its translation; the formatting tags stay. */
function ChangeUnit({
  unit,
  sourceLang,
  targetLang,
  backend,
  onClose,
  onSaved,
}: {
  unit: FoundUnit;
  sourceLang: LangCode;
  targetLang: LangCode;
  backend: UnitsBackend;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [runs, setRuns] = useState<Run[]>(() => coreToRuns(unit.target));
  const [problem, setProblem] = useState("");
  const who = [unit.changedBy, unit.changedAt && fmtDate(unit.changedAt)].filter(Boolean).join(", ");
  const used = unit.usageCount ?? 0;

  function save() {
    if (!plain(runs).trim()) {
      setProblem(tl("A unit needs a translation. Delete the unit instead."));
      return;
    }
    backend.change(unit, runsToCore(runs)).then(onSaved, (err) =>
      setProblem(err instanceof SavedElsewhere ? tl("Someone else changed this unit in the meantime. Search again to see it as it is now.") : text(err)),
    );
  }

  return (
    <Modal title={tl("Change the unit")} onClose={onClose} wide>
      <div className="dialog-form">
        <div className="field">
          <span className="field-label">{lang(sourceLang).name}</span>
          <p className="unit-text unit-source" lang={sourceLang}>
            <Content runs={coreToRuns(unit.source)} />
          </p>
        </div>
        <div className="field">
          <span className="field-label" id="unit-target-label">
            {lang(targetLang).name}
          </span>
          <div className="unit-text unit-target">
            <TargetEditor
              initial={coreToRuns(unit.target)}
              lang={targetLang}
              label={lang(targetLang).name}
              onChange={setRuns}
              onKeyDown={(e) => {
                // A unit is one segment: Enter saves it.
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  save();
                }
              }}
            />
          </div>
        </div>
        {who && (
          <p className="help">
            {used === 1 ? tl("Last changed by {who}. Used once.", { who }) : used > 1 ? tl("Last changed by {who}. Used {n} times.", { who, n: fmtNum(used) }) : tl("Last changed by {who}.", { who })}</p>
        )}
        {problem && (
          <p className="error" role="alert">
            {problem}
          </p>
        )}
        <div className="row">
          <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
            {tl("Cancel")}
          </button>
          <button type="button" className="btn btn-primary btn-small" onClick={save}>
            {tl("Save")}
          </button>
        </div>
      </div>
    </Modal>
  );
}
