import { useEffect, useMemo, useState } from "react";
import { CheckCircle2 } from "lucide-react";
import {
  lang,
  type Entry,
  type ExportOptions,
  type ExportPreset,
  type Termbase,
} from "../data";
import { download, pdfFonts } from "../core";
import { useLocalStore } from "../store";
import { useRemote, errorText } from "../remote";
import { plural } from "../format";
import { FORMATS, fileBase, term, toCoreEntry, toCoreTermbase } from "../termio";
import { Modal, useCore } from "./TermbaseDialogs";
import { may } from "./ServerTermbase";

/** Glossaries to read and print, then the exchange formats. */
const READ_FORMATS = [
  { id: "pdf", label: ".pdf", note: "To print or send: fonts included, the same everywhere." },
  { id: "docx", label: ".docx", note: "To edit further in a word processor." },
  { id: "html", label: ".html", note: "One web page with a letter index and a search." },
  { id: "txt", label: ".txt", note: "Plain text." },
];

const MEDIA: Record<string, string> = {
  pdf: "application/pdf",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  html: "text/html",
  txt: "text/plain",
};

const STATUSES = [
  { id: "approved", name: "Approved" },
  { id: "proposed", name: "Proposed" },
  { id: "forbidden", name: "Forbidden" },
];

type Step = "format" | "content" | "layout";
const STEP_NAME: Record<Step, string> = { format: "Format", content: "Content", layout: "Layout" };

const glossaryFormat = (f: string) => f === "pdf" || f === "docx" || f === "html" || f === "txt";

/** Options as a fresh export of a format starts: everything, and a glossary as a dictionary with its letters. */
export function defaultOptions(format: string): ExportOptions {
  return glossaryFormat(format)
    ? { format, layout: "dictionary", letters: true, labels: true, status: true, allFields: true }
    : { format, allFields: true };
}

/**
 * Exports a termbase, in steps: the format (or settings kept before),
 * what goes in (languages, entries, statuses, fields, a filter), and for a
 * glossary how it is laid out. Settings can be kept with the termbase
 * under a name, for everyone who exports it.
 */
export function ExportTermbaseDialog({
  tb,
  entries,
  shown,
  onClose,
}: {
  tb: Termbase;
  entries: Entry[];
  shown: Entry[];
  onClose: () => void;
}) {
  const { dispatch, notify } = useLocalStore();
  const r = useRemote();
  const c = useCore();
  const [step, setStep] = useState<Step>("format");
  const [o, setO] = useState<ExportOptions>(() => defaultOptions("pdf"));
  const [scope, setScope] = useState<"all" | "shown">("all");
  const [preset, setPreset] = useState("");
  const [saveName, setSaveName] = useState("");
  const [kept, setKept] = useState<ExportPreset[]>(tb.exportPresets ?? []);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const ready = useMemo(() => (c ? term.presets(c) : []), [c]);
  const manage = may(tb, "manage");

  // Settings kept with a termbase on the server are in its definition.
  useEffect(() => {
    if (!tb.remote || !r.api) return;
    let live = true;
    r.api.termbase(tb.name).then(
      (d) => live && setKept(d.definition.exports ?? []),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [tb.remote, tb.name, r.api]);

  const glossary = glossaryFormat(o.format);
  const steps: Step[] = glossary ? ["format", "content", "layout"] : ["format", "content"];
  const at = steps.indexOf(step);
  const set = (patch: Partial<ExportOptions>) => setO((x) => ({ ...x, ...patch }));
  const langs = o.languages ?? tb.languages;
  const fields = tb.infoTypes;
  const chosenFields = o.allFields || o.fields === undefined ? fields.map((f) => f.name) : o.fields;
  const partial = shown.length < entries.length;
  const list = scope === "all" ? entries : shown;
  const filterField = fields.find((f) => f.name === o.filterField);

  const choosePreset = (name: string) => {
    setPreset(name);
    const p = [...kept, ...ready].find((x) => x.name === name);
    if (!p) return;
    const next = { ...p.options };
    // Languages or a sort the termbase lacks are left to it.
    if (next.languages) next.languages = next.languages.filter((l) => tb.languages.includes(l));
    if (next.languages && next.languages.length === 0) delete next.languages;
    if (next.sortBy && !tb.languages.includes(next.sortBy)) delete next.sortBy;
    setO(next);
  };

  const keep = async () => {
    const name = saveName.trim();
    if (!name) return;
    const options: ExportOptions = { ...o };
    delete options.entries;
    const next = [...kept.filter((p) => p.name.toLocaleLowerCase() !== name.toLocaleLowerCase()), { name, options }];
    setProblem("");
    try {
      if (tb.remote && r.api) {
        const d = await r.api.termbase(tb.name);
        await r.api.setDefinition(tb.name, { ...d.definition, exports: next });
      } else {
        dispatch({ type: "updateTermbase", termbaseId: tb.id, patch: { exportPresets: next } });
      }
      setKept(next);
      setPreset(name);
      setSaveName("");
      notify(`Kept the export settings “${name}” with ${tb.name}`);
    } catch (e) {
      setProblem(errorText(e));
    }
  };

  const removeKept = async (name: string) => {
    const next = kept.filter((p) => p.name !== name);
    try {
      if (tb.remote && r.api) {
        const d = await r.api.termbase(tb.name);
        await r.api.setDefinition(tb.name, { ...d.definition, exports: next });
      } else {
        dispatch({ type: "updateTermbase", termbaseId: tb.id, patch: { exportPresets: next.length ? next : undefined } });
      }
      setKept(next);
      if (preset === name) setPreset("");
      notify(`Removed the export settings “${name}”`);
    } catch (e) {
      setProblem(errorText(e));
    }
  };

  const run = async () => {
    setBusy(true);
    setProblem("");
    try {
      const options: ExportOptions = { ...o };
      const ext = glossary ? `.${o.format}` : (FORMATS.find((f) => f.id === o.format)?.ext ?? "");
      const name = `${fileBase(tb)}${o.format === "tbx2" ? "_tbx2" : ""}${ext}`;
      let data: Uint8Array;
      let type: string;
      let summary: string;
      let missing = "";
      if (tb.remote && r.api && o.format === "pdf") {
        // The team server sets a PDF in its fonts and the computer's, so
        // Chinese, Japanese and Korean have theirs.
        if (scope === "shown") options.entries = shown.map((e) => toCoreEntry(e, tb).id ?? "").filter(Boolean);
        const res = await r.api.exportWith(tb.name, options);
        ({ data, type, summary, missing } = res);
      } else {
        if (!c) throw new Error("The core is still loading.");
        const fonts = o.format === "pdf" ? await pdfFonts() : [];
        const res = term.exportWith(c, toCoreTermbase(tb, list), options, fonts);
        data = res.data;
        type = MEDIA[o.format] ?? FORMATS.find((f) => f.id === o.format)?.type ?? "application/octet-stream";
        summary = res.summary;
        missing = res.missing;
      }
      download(data, name, type);
      notify(
        missing
          ? `Exported ${summary} to ${name}. No font has ${[...missing].slice(0, 12).join(" ")}: they show as boxes.`
          : `Exported ${summary} to ${name}`,
      );
      onClose();
    } catch (e) {
      setProblem(`The export failed: ${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };

  const blocking =
    langs.length === 0
      ? "Choose a language at least."
      : glossary && o.sortBy && !langs.includes(o.sortBy)
        ? "Sort by one of the languages exported."
        : list.length === 0
          ? "There is no entry to export."
          : "";

  return (
    <Modal title={`Export ${tb.name}`} onClose={onClose} wide>
      <ol className="stepper stepper-small" aria-label="Steps">
        {steps.map((s, i) => (
          <li key={s} aria-current={s === step ? "step" : undefined} className={at > i ? "is-done" : undefined}>
            <button type="button" className="stepper-link" onClick={() => setStep(s)}>
              <span className="stepper-num" aria-hidden="true">
                {at > i ? <CheckCircle2 size={14} /> : i + 1}
              </span>
              <span>{STEP_NAME[s]}</span>
            </button>
          </li>
        ))}
      </ol>

      {step === "format" && (
        <>
          {(kept.length > 0 || ready.length > 0) && (
            <div className="field">
              <label htmlFor="exp-preset">Start from</label>
              <select id="exp-preset" value={preset} onChange={(e) => choosePreset(e.target.value)}>
                <option value="">Settings of my own</option>
                {kept.length > 0 && (
                  <optgroup label={`Kept with ${tb.name}`}>
                    {kept.map((p) => (
                      <option key={`k-${p.name}`} value={p.name}>
                        {p.name}
                      </option>
                    ))}
                  </optgroup>
                )}
                <optgroup label="Ready-made">
                  {ready.map((p) => (
                    <option key={`r-${p.name}`} value={p.name}>
                      {p.name}
                    </option>
                  ))}
                </optgroup>
              </select>
              {kept.some((p) => p.name === preset) && manage && (
                <div className="row row-start">
                  <button type="button" className="btn btn-quiet btn-small" onClick={() => void removeKept(preset)}>
                    Remove “{preset}”
                  </button>
                </div>
              )}
            </div>
          )}
          <fieldset className="field">
            <legend>To read and print</legend>
            <ul className="choice-list choice-grid">
              {READ_FORMATS.map((f) => (
                <li key={f.id}>
                  <label className="choice">
                    <input
                      type="radio"
                      name="export-format"
                      checked={o.format === f.id}
                      onChange={() => {
                        setPreset("");
                        setO((x) => (glossaryFormat(x.format) ? { ...x, format: f.id } : { ...defaultOptions(f.id), languages: x.languages, statuses: x.statuses }));
                      }}
                    />
                    <span className="choice-body">
                      <span className="choice-title">{f.label}</span>
                      <span className="choice-desc">{f.note}</span>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          </fieldset>
          <fieldset className="field">
            <legend>To exchange with other tools</legend>
            <ul className="choice-list choice-grid">
              {FORMATS.map((f) => (
                <li key={f.id}>
                  <label className="choice">
                    <input
                      type="radio"
                      name="export-format"
                      checked={o.format === f.id}
                      onChange={() => {
                        setPreset("");
                        setO((x) => ({ ...x, format: f.id }));
                      }}
                    />
                    <span className="choice-body">
                      <span className="choice-title">{f.label}</span>
                      <span className="choice-desc">{f.note}</span>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          </fieldset>
        </>
      )}

      {step === "content" && (
        <>
          <fieldset className="field">
            <legend>Languages</legend>
            <div className="check-list" role="group" aria-label="Languages exported">
              {tb.languages.map((l) => (
                <label key={l} className="check">
                  <input
                    type="checkbox"
                    checked={langs.includes(l)}
                    onChange={(e) => {
                      const next = e.target.checked ? tb.languages.filter((x) => x === l || langs.includes(x)) : langs.filter((x) => x !== l);
                      set({ languages: next.length === tb.languages.length ? undefined : next });
                    }}
                  />
                  <span>
                    {lang(l).name} <span className="muted">{l}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          {partial && (
            <fieldset className="field">
              <legend>Entries</legend>
              <div className="check-stack">
              <label className="check">
                <input type="radio" name="export-scope" checked={scope === "all"} onChange={() => setScope("all")} />
                <span>All {plural(entries.length, "entry", "entries")}</span>
              </label>
              <label className="check">
                <input type="radio" name="export-scope" checked={scope === "shown"} onChange={() => setScope("shown")} />
                <span>The {plural(shown.length, "entry", "entries")} the search shows</span>
              </label>
              </div>
            </fieldset>
          )}
          <fieldset className="field">
            <legend>Terms</legend>
            <div className="check-list" role="group" aria-label="Statuses exported">
              {STATUSES.map((s) => {
                const on = !o.statuses || o.statuses.includes(s.id);
                return (
                  <label key={s.id} className="check">
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={(e) => {
                        const cur = o.statuses ?? STATUSES.map((x) => x.id);
                        const next = e.target.checked ? STATUSES.map((x) => x.id).filter((x) => x === s.id || cur.includes(x)) : cur.filter((x) => x !== s.id);
                        set({ statuses: next.length === STATUSES.length ? undefined : next });
                      }}
                    />
                    <span>{s.name}</span>
                  </label>
                );
              })}
            </div>
          </fieldset>
          {fields.length > 0 && (
            <fieldset className="field">
              <legend>Fields</legend>
              <div className="check-list" role="group" aria-label="Fields exported">
                {fields.map((f) => (
                  <label key={f.id} className="check">
                    <input
                      type="checkbox"
                      checked={chosenFields.includes(f.name)}
                      onChange={(e) => {
                        const next = e.target.checked ? fields.map((x) => x.name).filter((x) => x === f.name || chosenFields.includes(x)) : chosenFields.filter((x) => x !== f.name);
                        set(next.length === fields.length ? { allFields: true, fields: undefined } : { allFields: false, fields: next });
                      }}
                    />
                    <span>{f.name}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}
          <div className="field-grid">
            <div className="field">
              <label htmlFor="exp-filter">Only entries with</label>
              <div className="row row-start">
                <select
                  id="exp-filter"
                  className="select-auto"
                  value={o.filterField ?? ""}
                  onChange={(e) => set({ filterField: e.target.value || undefined, filterValue: undefined })}
                >
                  <option value="">Any entry</option>
                  {fields.map((f) => (
                    <option key={f.id} value={f.name}>
                      {f.name}
                    </option>
                  ))}
                </select>
                {filterField &&
                  (filterField.kind === "picklist" ? (
                    <select aria-label={`Value of ${filterField.name}`} className="select-auto" value={o.filterValue ?? ""} onChange={(e) => set({ filterValue: e.target.value || undefined })}>
                      <option value="">Any value</option>
                      {(filterField.values ?? []).map((v) => (
                        <option key={v}>{v}</option>
                      ))}
                    </select>
                  ) : (
                    <input aria-label={`Value of ${filterField.name}`} placeholder="Any value" value={o.filterValue ?? ""} onChange={(e) => set({ filterValue: e.target.value || undefined })} />
                  ))}
              </div>
            </div>
            <div className="field">
              <label htmlFor="exp-since">Changed since</label>
              <input id="exp-since" type="date" value={o.changedSince ?? ""} onChange={(e) => set({ changedSince: e.target.value || undefined })} />
            </div>
          </div>
        </>
      )}

      {step === "layout" && glossary && (
        <>
          <fieldset className="field">
            <legend>Layout</legend>
            <ul className="choice-list choice-grid">
              {[
                { id: "dictionary", title: "Dictionary", text: "Headwords in two columns, each language on a line under them." },
                { id: "table", title: "Table", text: "A column for each language, a row for each entry." },
              ].map((x) => (
                <li key={x.id}>
                  <label className="choice">
                    <input type="radio" name="export-layout" checked={(o.layout ?? "dictionary") === x.id} onChange={() => set({ layout: x.id as ExportOptions["layout"] })} />
                    <span className="choice-body">
                      <span className="choice-title">{x.title}</span>
                      <span className="choice-desc">{x.text}</span>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          </fieldset>
          <div className="field-grid">
            <div className="field">
              <label htmlFor="exp-sort">Sorted by</label>
              <select id="exp-sort" value={o.sortBy ?? langs[0] ?? ""} onChange={(e) => set({ sortBy: e.target.value })}>
                {langs.map((l) => (
                  <option key={l} value={l}>
                    {lang(l).name}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="exp-title">Title</label>
              <input id="exp-title" value={o.title ?? ""} placeholder={tb.name} onChange={(e) => set({ title: e.target.value || undefined })} />
            </div>
          </div>
          <fieldset className="field">
            <legend>Show</legend>
            <div className="check-stack">
            {[
              { k: "letters", t: "Initial letters before their headwords" },
              { k: "perTerm", t: "Every synonym as a headword of its own" },
              { k: "groupHomonyms", t: "Entries with the same headword under it, numbered" },
              { k: "labels", t: "The names of fields before their values" },
              { k: "status", t: "Proposed and forbidden terms marked" },
            ].map((x) => (
              <label key={x.k} className="check">
                <input
                  type="checkbox"
                  checked={!!o[x.k as keyof ExportOptions]}
                  onChange={(e) => set({ [x.k]: e.target.checked } as Partial<ExportOptions>)}
                />
                <span>{x.t}</span>
              </label>
            ))}
            </div>
          </fieldset>
          {(o.format === "pdf" || o.format === "docx" || o.format === "html") && (
            <fieldset className="field">
              <legend>Page</legend>
              <div className="check-stack">
              <label className="check">
                <input type="radio" name="export-page" checked={(o.page ?? "a4") === "a4"} onChange={() => set({ page: "a4" })} />
                <span>A4</span>
              </label>
              <label className="check">
                <input type="radio" name="export-page" checked={o.page === "letter"} onChange={() => set({ page: "letter" })} />
                <span>Letter</span>
              </label>
              </div>
            </fieldset>
          )}
        </>
      )}

      {manage && (
        <div className="field export-keep">
          <label htmlFor="exp-keep">Keep these settings as</label>
          <div className="row row-start">
            <input id="exp-keep" value={saveName} placeholder="For example Client glossary" onChange={(e) => setSaveName(e.target.value)} />
            <button type="button" className="btn btn-small" disabled={!saveName.trim()} onClick={() => void keep()}>
              Keep
            </button>
          </div>
          <p className="help">Kept with {tb.name}, for everyone who exports it.</p>
        </div>
      )}

      {(problem || blocking) && (
        <p className={problem ? "error" : "help"} role={problem ? "alert" : undefined}>
          {problem || blocking}
        </p>
      )}
      <div className="row row-end">
        <button type="button" className="btn btn-quiet btn-small" onClick={onClose}>
          Cancel
        </button>
        {at > 0 && (
          <button type="button" className="btn btn-small" onClick={() => setStep(steps[at - 1]!)}>
            Back
          </button>
        )}
        {at < steps.length - 1 && (
          <button type="button" className="btn btn-small" onClick={() => setStep(steps[at + 1]!)}>
            Continue
          </button>
        )}
        <button type="button" className="btn btn-primary btn-small" disabled={busy || !!blocking || (!c && !(tb.remote && o.format === "pdf"))} onClick={() => void run()}>
          {busy ? "Exporting…" : `Export to ${glossary ? `.${o.format}` : (FORMATS.find((f) => f.id === o.format)?.label ?? o.format)}`}
        </button>
      </div>
    </Modal>
  );
}
