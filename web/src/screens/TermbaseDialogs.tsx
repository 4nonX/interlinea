import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { AlertTriangle, FileUp, Trash2 } from "lucide-react";
import { core, loadCore, type CoreApi } from "../core";
import {
  lang,
  LANGUAGES,
  type Entry,
  type InfoType,
  type LangCode,
  type Termbase,
  type TermStatus,
  type ReviewRules,
} from "../data";
import { useLocalStore, type ImportInput } from "../store";
import { AddFieldForm, KindSelect, ValuesEditor } from "./TermbaseFields";
import { listOf, plural } from "../format";
import { LangChip } from "../components/ui";
import { go } from "../App";
import {
  FORMAT_LABEL,
  IMPORT_ACCEPT,
  fileLanguages,
  fromCoreEntry,
  matchFields,
  remapLanguages,
  suggestLanguage,
  term,
  toCoreEntry,
  type ColumnRole,
  type CoreEntry,
  type CoreLevel,
  type CoreTermbase,
  type Detected,
  type Mapping,
  type PlanAction,
  type PlanReport,
  type Policy,
  type ReadReport,
} from "../termio";
import { tl, tlx } from "../i18n";

/** The loaded core, loading it on first use. */
export function useCore(): CoreApi | null {
  const { notify } = useLocalStore();
  const [c, setC] = useState<CoreApi | null>(() => core());
  useEffect(() => {
    if (c) return;
    let live = true;
    loadCore().then(
      (x) => live && setC(x),
      (e: unknown) =>
        live &&
        notify(
          tl("The core could not be loaded: {error}", { error: e instanceof Error ? e.message : String(e) }),
        ),
    );
    return () => {
      live = false;
    };
  }, [c, notify]);
  return c;
}

/** A modal dialog that is open while it is mounted. */
export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    const d = ref.current!;
    if (!d.open) d.showModal();
    return () => d.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className={`dialog${wide ? " dialog-wide" : ""}`}
      aria-labelledby={id}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      <div className="dialog-body">
        <h2 id={id} className="dialog-title">
          {title}
        </h2>
        {children}
      </div>
    </dialog>
  );
}

/** A button that asks once more before it does something that cannot be undone. */
export function ConfirmButton({
  label,
  confirm,
  onConfirm,
  disabled,
  small = true,
  action = tl("Remove"),
  plain = false,
}: {
  label: ReactNode;
  confirm: string;
  onConfirm: () => void;
  disabled?: boolean;
  small?: boolean;
  /** The label of the confirming button. */
  action?: string;
  /** The step takes nothing away: asked in plain colours, not in red. */
  plain?: boolean;
}) {
  const [asking, setAsking] = useState(false);
  const yes = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (asking) yes.current?.focus();
  }, [asking]);
  if (!asking)
    return (
      <button
        type="button"
        className={`btn btn-quiet${small ? " btn-small" : ""}`}
        onClick={() => setAsking(true)}
        disabled={disabled}
      >
        {label}
      </button>
    );
  return (
    <span className={plain ? "confirm confirm-plain" : "confirm"} role="group" aria-label={confirm}>
      <span className="confirm-text">{confirm}</span>
      <button
        ref={yes}
        type="button"
        className={plain ? "btn btn-primary btn-small" : "btn btn-danger btn-small"}
        onClick={() => {
          setAsking(false);
          onConfirm();
        }}
      >
        {action}
      </button>
      <button
        type="button"
        className="btn btn-quiet btn-small"
        onClick={() => setAsking(false)}
      >
        {tl("Cancel")}
      </button>
    </span>
  );
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

function nextId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
}

/* Export */

/* Settings */

/** What merges into a master kept in this browser without review: the rules a master on the team server has. */
function ReviewRulesSection({ tb }: { tb: Termbase }) {
  const { state, dispatch, notify } = useLocalStore();
  const [who, setWho] = useState("");
  const [forLang, setForLang] = useState("");
  const [problem, setProblem] = useState("");
  const rules = tb.rules ?? {};
  const minor = rules.minorFields ?? [];
  const trusted = rules.trusted ?? [];
  const put = (next: ReviewRules, done: string) => {
    const set = !!next.keepIdentical || !!next.minorFields?.length || !!next.trusted?.length;
    dispatch({ type: "updateTermbase", termbaseId: tb.id, patch: { rules: set ? next : undefined } });
    setProblem("");
    notify(done);
  };
  // Whoever wrote in the projects based on this master, and oneself.
  const branches = new Set(state.termbases.filter((t) => !t.remote && t.kind === "project" && (t.basedOn ?? []).includes(tb.id)).map((t) => t.id));
  const authors = [
    ...new Set([
      state.userName,
      ...state.entries.filter((e) => branches.has(e.termbaseId)).flatMap((e) => e.history.map((c) => c.by)),
      ...trusted.map((t) => t.author),
    ]),
  ]
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b));
  return (
    <section className="settings-section" aria-labelledby="tbs-review">
      <h3 id="tbs-review" className="settings-title">
        {tl("Review")}
      </h3>
      <p className="help">{tl("Projects merge their new and changed entries into this master, and the merge is reviewed first. These rules say what needs no review.")}</p>
      <label className="check">
        <input
          type="checkbox"
          checked={!rules.keepIdentical}
          onChange={(e) =>
            put(
              { ...rules, keepIdentical: e.target.checked ? undefined : true },
              e.target.checked ? tl("Entries the master already has are merged without review") : tl("Entries the master already has wait for review, too"),
            )
          }
        />
        {tl("Entries the master already has as they are need no review")}
      </label>
      <span className="field-label">{tl("Fields that need no approval")}</span>
      {tb.infoTypes.length === 0 ? (
        <p className="help">{tl("The termbase has no fields yet. A change to notes or sources only could be merged at once.")}</p>
      ) : (
        <>
          <div className="row-start" role="group" aria-label={tl("Fields that need no approval")}>
            {tb.infoTypes.map((f) => (
              <label key={f.id} className="check">
                <input
                  type="checkbox"
                  checked={minor.includes(f.id)}
                  onChange={(e) => {
                    const next = e.target.checked ? [...minor, f.id] : minor.filter((m) => m !== f.id);
                    put(
                      { ...rules, minorFields: next.length ? next : undefined },
                      e.target.checked ? tl("Changes to {name} only are merged without review", { name: f.name }) : tl("Changes to {name} wait for review", { name: f.name }),
                    );
                  }}
                />
                {f.name}
              </label>
            ))}
          </div>
          <p className="help">{tl("An entry changed in these fields only needs no review.")}</p>
        </>
      )}
      <span className="field-label">{tl("Trusted authors")}</span>
      {trusted.length === 0 ? (
        <p className="help">{tl("Nobody yet: every entry waits for review.")}</p>
      ) : (
        <ul className="mr-reviewers">
          {trusted.map((t, i) => (
            <li key={t.author}>
              <span className="cell-title">{t.author}</span>
              <span className="cell-sub">{t.lang ? tl("entries with terms in {name}", { name: lang(t.lang).name }) : tl("all entries")}</span>
              <button
                type="button"
                className="btn btn-quiet btn-small"
                aria-label={tl("No longer trust {author}", { author: t.author })}
                onClick={() => {
                  const next = trusted.filter((_, j) => j !== i);
                  put({ ...rules, trusted: next.length ? next : undefined }, tl("The entries of {author} wait for review again", { author: t.author }));
                }}
              >
                {tl("Remove")}
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="role-add"
        aria-label={tl("Trust an author")}
        onSubmit={(e) => {
          e.preventDefault();
          if (!who) {
            setProblem(tl("Say whose entries to trust."));
            return;
          }
          // One rule an author: a new one replaces the old.
          const next = [...trusted.filter((t) => t.author.toLowerCase() !== who.toLowerCase()), forLang ? { author: who, lang: forLang } : { author: who }];
          setWho("");
          setForLang("");
          put({ ...rules, trusted: next }, tl("The entries of {who} need no review", { who }));
        }}
      >
        <label>
          <span className="field-label">{tl("Author")}</span>
          <select value={who} onChange={(e) => setWho(e.target.value)}>
            <option value="">{tl("Choose a person…")}</option>
            {authors.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span className="field-label">{tl("For")}</span>
          <select value={forLang} onChange={(e) => setForLang(e.target.value)}>
            <option value="">{tl("All entries")}</option>
            {tb.languages.map((l) => (
              <option key={l} value={l}>
                {tlx("Terms in {name}", { name: lang(l).name })}</option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn btn-small">
          {tl("Trust")}
        </button>
      </form>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      <p className="help">{tl("Their new and changed entries need no review. An entry that conflicts with the master always does.")}</p>
    </section>
  );
}

export function SettingsDialog({
  tb,
  onClose,
}: {
  tb: Termbase;
  onClose: () => void;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const [name, setName] = useState(tb.name);
  const [addLang, setAddLang] = useState("");
  const entries = state.entries.filter((e) => e.termbaseId === tb.id);
  const usedBy = state.projects.filter(
    (p) =>
      p.setup.masters.includes(tb.id) || p.setup.projectTermbaseId === tb.id,
  );
  const now = () => new Date().toISOString();
  const termsIn = (code: LangCode) =>
    entries.reduce((n, e) => n + (e.terms[code]?.length ?? 0), 0);
  const valuesOf = (id: string) =>
    entries.reduce(
      (n, e) =>
        n +
        (e.concept[id] ? 1 : 0) +
        Object.values(e.langFields ?? {}).filter((f) => f[id]).length +
        Object.values(e.terms)
          .flat()
          .filter((t) => t.fields?.[id]).length,
      0,
    );
  const saveName = () => {
    const v = name.trim();
    if (!v || v === tb.name) {
      setName(tb.name);
      return;
    }
    dispatch({ type: "updateTermbase", termbaseId: tb.id, patch: { name: v } });
    notify(tl("Renamed to “{v}”", { v }));
  };
  const setInfo = (id: string, patch: Partial<InfoType>) =>
    dispatch({
      type: "updateTermbase",
      termbaseId: tb.id,
      patch: {
        infoTypes: tb.infoTypes.map((i) =>
          i.id === id ? { ...i, ...patch } : i,
        ),
      },
    });

  return (
    <Modal title={tl("Termbase settings")} onClose={onClose} wide>
      <div className="field">
        <label htmlFor="tb-name">{tl("Name")}</label>
        <input
          id="tb-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={saveName}
          onKeyDown={(e) => e.key === "Enter" && saveName()}
        />
      </div>

      <section className="settings-section" aria-labelledby="tbs-langs">
        <h3 id="tbs-langs" className="settings-title">
          {tl("Languages")}
        </h3>
        {tb.languages.length === 0 ? (
          <p className="help">
            {tl("No languages yet. They are added when you add terms or import a file.")}
          </p>
        ) : (
          <ul className="settings-list">
            {tb.languages.map((l) => {
              const n = termsIn(l);
              return (
                <li key={l}>
                  <span className="settings-name">
                    {lang(l).name} <LangChip code={l} />
                  </span>
                  <span className="muted">{plural(n, "term", "terms")}</span>
                  <ConfirmButton
                    label={
                      <>
                        <Trash2 size={14} aria-hidden="true" />
                        <span className="sr-only">{tl("Remove {name}", { name: lang(l).name })}</span>
                      </>
                    }
                    confirm={
                      n
                        ? tl("Remove {name} and its {n}?", { name: lang(l).name, n: plural(n, "term", "terms") })
                        : `Remove ${lang(l).name}?`
                    }
                    onConfirm={() => {
                      dispatch({
                        type: "removeLanguage",
                        termbaseId: tb.id,
                        langCode: l,
                        now: now(),
                      });
                      notify(
                        (n ? tl("Removed {name} and {terms}", { name: lang(l).name, terms: plural(n, "term", "terms") }) : tl("Removed {name}", { name: lang(l).name })),
                      );
                    }}
                  />
                </li>
              );
            })}
          </ul>
        )}
        <form
          className="row row-start"
          onSubmit={(e) => {
            e.preventDefault();
            if (!addLang) return;
            dispatch({
              type: "updateTermbase",
              termbaseId: tb.id,
              patch: { languages: [...tb.languages, addLang] },
            });
            notify(tl("Added {name}", { name: lang(addLang).name }));
            setAddLang("");
          }}
        >
          <label htmlFor="tbs-add-lang" className="sr-only">
            {tl("Language to add")}
          </label>
          <select
            id="tbs-add-lang"
            className="select-auto"
            value={addLang}
            onChange={(e) => setAddLang(e.target.value)}
          >
            <option value="">{tl("Add a language…")}</option>
            {LANGUAGES.filter((l) => !tb.languages.includes(l.code)).map(
              (l) => (
                <option key={l.code} value={l.code}>
                  {l.name}
                </option>
              ),
            )}
          </select>
          <button type="submit" className="btn btn-small" disabled={!addLang}>
            {tl("Add")}
          </button>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="tbs-fields">
        <h3 id="tbs-fields" className="settings-title">
          {tl("Fields")}
        </h3>
        {tb.infoTypes.length === 0 ? (
          <p className="help">
            {tl("No fields yet: entries hold their terms and each term's status.")}
          </p>
        ) : (
          <table className="table settings-fields">
            <thead>
              <tr>
                <th scope="col">{tl("Name")}</th>
                <th scope="col">{tl("Applies to")}</th>
                <th scope="col">{tl("Values")}</th>
                <th scope="col" className="num">
                  {tl("Used")}
                </th>
                <th scope="col">
                  <span className="sr-only">{tl("Remove")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {tb.infoTypes.map((it) => (
                <FieldSettingsRow
                  key={it.id}
                  it={it}
                  used={valuesOf(it.id)}
                  onChange={(p) => setInfo(it.id, p)}
                  onRemove={() => {
                    dispatch({
                      type: "removeInfoType",
                      termbaseId: tb.id,
                      infoTypeId: it.id,
                      now: now(),
                    });
                    notify(tl("Removed the field “{name}”", { name: it.name }));
                  }}
                />
              ))}
            </tbody>
          </table>
        )}
        <AddFieldForm
          termbase={tb.name}
          taken={tb.infoTypes.map((i) => i.name)}
          onAdd={(f) => {
            dispatch({
              type: "addInfoType",
              termbaseId: tb.id,
              infoType: {
                id: `it-${Date.now().toString(36)}`,
                name: f.name,
                level: f.level,
                kind: f.kind,
                ...(f.kind === "picklist" ? { values: f.values } : {}),
              },
            });
            notify(tl("Added field “{name}” to {name2}", { name: f.name, name2: tb.name }));
          }}
        />
      </section>

      {tb.kind === "master" && <ReviewRulesSection tb={tb} />}

      {tb.kind === "master" && (
        <section
          className="settings-section settings-danger"
          aria-labelledby="tbs-delete"
        >
          <h3 id="tbs-delete" className="settings-title">
            {tl("Delete termbase")}
          </h3>
          {usedBy.length ? (
            <p className="help">
              {usedBy.length === 1 ? tl("{name} uses this termbase. Remove it from the project first.", { name: usedBy[0]!.name }) : tl("{names} use this termbase. Remove it from the projects first.", { names: listOf(usedBy.map((p) => p.name)) })}
            </p>
          ) : (
            <>
              <p className="help">
                {tlx("Deletes {length} and their history. Export the termbase first if you may need it again.", { length: plural(entries.length, "entry", "entries") })}
              </p>
              <ConfirmButton
                label={tl("Delete termbase")}
                action={tl("Delete")}
                confirm={tl("Delete {name} for good?", { name: tb.name })}
                onConfirm={() => {
                  dispatch({ type: "deleteTermbase", termbaseId: tb.id });
                  notify(tl("Deleted {name}", { name: tb.name }));
                  onClose();
                  go({ page: "terminology" });
                }}
              />
            </>
          )}
        </section>
      )}

      <div className="row">
        <button
          type="button"
          className="btn btn-primary btn-small"
          onClick={() => {
            saveName();
            onClose();
          }}
        >
          {tl("Done")}
        </button>
      </div>
    </Modal>
  );
}

const LEVEL_LABEL: Record<InfoType["level"], string> = {
  concept: tl("Entry"),
  language: tl("Language"),
  term: tl("Term"),
};

function FieldSettingsRow({
  it,
  used,
  onChange,
  onRemove,
}: {
  it: InfoType;
  used: number;
  onChange: (p: Partial<InfoType>) => void;
  onRemove: () => void;
}) {
  const [name, setName] = useState(it.name);
  const id = useId();
  return (
    <tr>
      <th scope="row">
        <label htmlFor={`${id}-n`} className="sr-only">
          {tl("Field name")}
        </label>
        <input
          id={`${id}-n`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={() =>
            name.trim() && name.trim() !== it.name
              ? onChange({ name: name.trim() })
              : setName(it.name)
          }
        />
      </th>
      <td>{LEVEL_LABEL[it.level]}</td>
      <td>
        <div className="settings-kind">
          <KindSelect
            id={`${id}-k`}
            label={tl("Values of {name}", { name: it.name })}
            value={it.kind}
            onChange={(kind) =>
              onChange({
                kind,
                values: kind === "picklist" ? (it.values ?? []) : undefined,
              })
            }
          />
          {it.kind === "picklist" && (
            <ValuesEditor
              label={tl("The values of {name}, one a line", { name: it.name })}
              values={it.values ?? []}
              onChange={(values) => onChange({ values })}
            />
          )}
        </div>
      </td>
      <td className="num">{used}</td>
      <td>
        <ConfirmButton
          label={
            <>
              <Trash2 size={14} aria-hidden="true" />
              <span className="sr-only">{tl("Remove {name}", { name: it.name })}</span>
            </>
          }
          confirm={
            used
              ? tl("Remove “{name}” and {used}?", { name: it.name, used: plural(used, "value", "values") })
              : tl("Remove “{name}”?", { name: it.name })
          }
          onConfirm={onRemove}
        />
      </td>
    </tr>
  );
}

/* Import */

type Step = "file" | "columns" | "languages" | "review";

interface Loaded {
  name: string;
  bytes: Uint8Array;
  detected: Detected;
}

const ROLE_LABEL: Record<ColumnRole, string> = {
  term: tl("Terms"),
  status: tl("Status"),
  field: tl("Field"),
  id: tl("Entry ID"),
  ignore: tl("Leave out"),
};

/**
 * Import wizard, modelled on MultiTerm Convert and memoQ's import: pick a
 * file, map the columns of a spreadsheet, map its languages, choose what
 * happens to entries that already exist, then import.
 */
export function ImportDialog({
  tb,
  onClose,
}: {
  tb?: Termbase;
  onClose: () => void;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const c = useCore();
  const [step, setStep] = useState<Step>("file");
  const [target, setTarget] = useState<"this" | "new">(tb ? "this" : "new");
  const [newName, setNewName] = useState("");
  const [fallback, setFallback] = useState<TermStatus>("approved");
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [mapping, setMapping] = useState<Mapping | null>(null);
  const [read, setRead] = useState<{
    termbase: CoreTermbase;
    report: ReadReport;
  } | null>(null);
  const [langMap, setLangMap] = useState<Record<string, LangCode | null>>({});
  const [policy, setPolicy] = useState<Policy>({
    match: "term",
    onMatch: "merge",
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const dest: Termbase | undefined = target === "this" ? tb : undefined;
  const destLangs = dest?.languages ?? [];
  const existing = useMemo(
    () => (dest ? state.entries.filter((e) => e.termbaseId === dest.id) : []),
    [dest, state.entries],
  );

  const choose = async (f: File) => {
    setError("");
    if (!c) return;
    setBusy(true);
    try {
      const bytes = new Uint8Array(await f.arrayBuffer());
      const detected = term.detect(c, bytes, f.name);
      setLoaded({ name: f.name, bytes, detected });
      if (!newName)
        setNewName(f.name.replace(/\.[^.]+$/, "").replace(/[_-]+/g, " "));
      if (detected.tabular) {
        setMapping(detected.mapping!);
        setStep("columns");
      } else {
        readFile({ name: f.name, bytes, detected }, undefined);
      }
    } catch (e) {
      setError(tl("{name} could not be read: {e}", { name: f.name, e: errText(e) }));
    } finally {
      setBusy(false);
    }
  };

  const readFile = (l: Loaded, m: Mapping | undefined) => {
    if (!c) return;
    try {
      const r = term.read(c, l.bytes, l.detected.format, m);
      if (!r.termbase.entries?.length && l.detected.format !== "xdt")
        throw new Error(tl("the file has no entries with terms"));
      const map: Record<string, LangCode | null> = {};
      for (const fl of fileLanguages(r.termbase))
        map[fl] = suggestLanguage(fl, destLangs);
      setRead(r);
      setLangMap(map);
      setError("");
      setStep("languages");
    } catch (e) {
      setError(tl("{name} could not be read: {e}", { name: l.name, e: errText(e) }));
    }
  };

  // Entries of the file in the termbase's languages, and what importing them does.
  const incoming = useMemo(
    () =>
      read
        ? remapLanguages(read.termbase.entries ?? [], langMap)
        : { entries: [], dropped: 0 },
    [read, langMap],
  );
  const coreExisting = useMemo(
    () => (dest ? existing.map((e) => toCoreEntry(e, dest)) : []),
    [dest, existing],
  );
  const effective: Policy = existing.length
    ? policy
    : { match: "none", onMatch: "add" };
  const planned = useMemo(():
    { actions: PlanAction[]; report: PlanReport } | string | null => {
    if (!c || step !== "review") return null;
    try {
      const p = term.plan(c, coreExisting, incoming.entries, effective);
      return { actions: p.actions ?? [], report: p.report };
    } catch (e) {
      return tl("The import could not be planned: {error}", { error: errText(e) });
    }
  }, [
    c,
    step,
    coreExisting,
    incoming.entries,
    effective.match,
    effective.lang,
    effective.onMatch,
  ]); // eslint-disable-line react-hooks/exhaustive-deps
  const plan = typeof planned === "string" ? null : planned;
  const planError = typeof planned === "string" ? planned : "";

  const usedLangs = [
    ...new Set(Object.values(langMap).filter((v): v is LangCode => !!v)),
  ];
  const fields = useMemo(
    () =>
      read
        ? matchFields(
            dest ?? { infoTypes: [] },
            read.termbase,
            incoming.entries,
          )
        : null,
    [read, dest, incoming.entries],
  );

  const doImport = () => {
    if (!plan || !read || !loaded || !fields) return;
    const now = new Date().toISOString();
    const create: Termbase | undefined =
      target === "new"
        ? {
            id: nextId("tb"),
            name: newName.trim() || read.termbase.name || tl("Imported termbase"),
            kind: "master",
            location: "local",
            languages: [],
            infoTypes: [],
          }
        : undefined;
    const termbaseId = create?.id ?? dest!.id;
    const byCoreId = new Map(existing.map((e) => [e.externalId ?? e.id, e.id]));
    const adds: Entry[] = [];
    const updates = new Map<string, ImportInput["updates"][number]>();
    for (const a of plan.actions) {
      if (!a.entry) continue;
      const content = fromCoreEntry(a.entry, fields.ids, fallback);
      if (a.kind === "add") {
        const history = [];
        if (a.entry.createdAt && !a.entry.createdAt.startsWith("0001"))
          history.push({
            id: nextId("c"),
            at: a.entry.createdAt,
            by: a.entry.createdBy || "unknown",
            summary: tl("Created (date from the imported file)"),
          });
        history.push({
          id: nextId("c"),
          at: now,
          by: state.userName,
          summary: tl("Imported from {name}", { name: loaded.name }),
        });
        adds.push({
          id: nextId("e"),
          termbaseId,
          externalId: a.entry.id || undefined,
          version: 1,
          ...content,
          history,
        });
      } else {
        const id = byCoreId.get(a.existing ?? "");
        if (id)
          updates.set(id, {
            entryId: id,
            content,
            summary: (a.kind === "merge" ? tl("Merged with an entry from {name}", { name: loaded.name }) : tl("Replaced by an entry from {name}", { name: loaded.name })),
          });
      }
    }
    dispatch({
      type: "importEntries",
      input: {
        termbaseId,
        create,
        languages: usedLangs,
        infoTypes: fields.created,
        adds,
        updates: [...updates.values()],
        now,
      },
    });
    const r = plan.report;
    const parts = [
      r.added && tl("{n} added", { n: r.added }),
      r.merged && tl("{n} merged", { n: r.merged }),
      r.overwritten && tl("{n} replaced", { n: r.overwritten }),
      r.skipped + r.unchanged && tl("{n} left as they were", { n: r.skipped + r.unchanged }),
    ].filter(Boolean);
    notify(
      tl("Imported {name} into {termbase}: {changes}", { name: loaded.name, termbase: create?.name ?? dest!.name, changes: parts.join(", ") || tl("nothing changed") }),
    );
    onClose();
    go({ page: "terminology", termbaseId });
  };

  const steps: Step[] = loaded?.detected.tabular
    ? ["file", "columns", "languages", "review"]
    : ["file", "languages", "review"];
  const stepNames: Record<Step, string> = {
    file: tl("File"),
    columns: tl("Columns"),
    languages: tl("Languages"),
    review: tl("Review"),
  };

  return (
    <Modal
      title={
        tb && target === "this" ? tl("Import into {name}", { name: tb.name }) : tl("Import a termbase")
      }
      onClose={onClose}
      wide
    >
      <ol className="stepper stepper-small" aria-label={tl("Steps")}>
        {steps.map((s, i) => (
          <li
            key={s}
            aria-current={s === step ? "step" : undefined}
            className={steps.indexOf(step) > i ? "is-done" : undefined}
          >
            <span className="stepper-num" aria-hidden="true">
              {i + 1}
            </span>
            <span>{stepNames[s]}</span>
          </li>
        ))}
      </ol>

      {step === "file" && (
        <>
          {tb && (
            <fieldset className="field">
              <legend>{tl("Import into")}</legend>
              <label className="check">
                <input
                  type="radio"
                  name="imp-target"
                  checked={target === "this"}
                  onChange={() => setTarget("this")}
                />
                <span>{tb.name}</span>
              </label>
              <label className="check">
                <input
                  type="radio"
                  name="imp-target"
                  checked={target === "new"}
                  onChange={() => setTarget("new")}
                />
                <span>{tl("A new master termbase")}</span>
              </label>
            </fieldset>
          )}
          {target === "new" && (
            <div className="field-grid">
              <div className="field">
                <label htmlFor="imp-name">
                  {tlx("Name {span}", { span: <span className="optional">{tl("from the file if empty")}</span> })}</label>
                <input
                  id="imp-name"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                />
              </div>
            </div>
          )}
          <button
            type="button"
            className="dropzone dropzone-small"
            onClick={() => fileInput.current?.click()}
            disabled={!c || busy}
          >
            <FileUp size={22} aria-hidden="true" className="dropzone-icon" />
            <span className="cell-title">
              {c
                ? busy
                  ? tl("Reading…")
                  : tl("Choose a termbase file")
                : tl("Loading the core…")}
            </span>
            <span className="help">
              {tl("TBX (2 or 3), an .sdltb termbase or its XML export, an XLSX workbook, CSV or tab-separated text. A termbase definition (.xdt) brings only the languages.")}
            </span>
          </button>
          <input
            ref={fileInput}
            type="file"
            accept={IMPORT_ACCEPT}
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              e.target.value = "";
              if (f) void choose(f);
            }}
          />
        </>
      )}

      {step === "columns" && loaded && mapping && (
        <ColumnStep loaded={loaded} mapping={mapping} setMapping={setMapping} />
      )}

      {step === "languages" && read && loaded && (
        <>
          <p className="help">
            {read.report.skipped
              ? tl("{format}: {entries}, {terms}, {skipped} without terms skipped.", { format: FORMAT_LABEL[loaded.detected.format], entries: plural(read.report.entries, "entry", "entries"), terms: plural(read.report.terms, "term", "terms"), skipped: plural(read.report.skipped, "row", "rows") })
              : tl("{format}: {entries}, {terms}.", { format: FORMAT_LABEL[loaded.detected.format], entries: plural(read.report.entries, "entry", "entries"), terms: plural(read.report.terms, "term", "terms") })}
          </p>
          <table className="table">
            <caption className="sr-only">{tl("Languages of the file")}</caption>
            <thead>
              <tr>
                <th scope="col">{tl("In the file")}</th>
                <th scope="col">{tl("In the termbase")}</th>
              </tr>
            </thead>
            <tbody>
              {Object.keys(langMap).map((fl) => (
                <tr key={fl}>
                  <th scope="row">
                    <code>{fl}</code>
                  </th>
                  <td>
                    <label htmlFor={`lm-${fl}`} className="sr-only">
                      {tlx("Termbase language for {fl}", { fl: fl })}</label>
                    <select
                      id={`lm-${fl}`}
                      value={langMap[fl] ?? ""}
                      onChange={(e) =>
                        setLangMap((m) => ({
                          ...m,
                          [fl]: e.target.value || null,
                        }))
                      }
                    >
                      <option value="">{tl("Leave out")}</option>
                      {[
                        ...new Set([
                          ...destLangs,
                          ...LANGUAGES.map((l) => l.code),
                          langMap[fl] ?? fl,
                        ]),
                      ].map((code) => (
                        <option key={code} value={code}>
                          {lang(code).name}
                          {dest && !destLangs.includes(code) ? " (new)" : ""}
                        </option>
                      ))}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {fields && fields.created.length > 0 && (
            <p className="help">
              {tlx("New fields: {join}.", { join: fields.created
                .map(
                  (f) =>
                    `${f.name} (${LEVEL_LABEL[f.level].toLocaleLowerCase()})`,
                )
                .join(", ") })}
            </p>
          )}
          {incoming.dropped > 0 && (
            <p className="detect detect-warn">
              <AlertTriangle size={16} aria-hidden="true" />
              <span>
                {tlx("{dropped} no terms in the languages you import and will be left out.", { dropped: plural(incoming.dropped, "entry has", "entries have") })}
              </span>
            </p>
          )}
          <Warnings items={read.report.warnings} />
          <fieldset className="field">
            <legend>{tl("Terms without a status")}</legend>
            <label className="check">
              <input
                type="radio"
                name="imp-fallback"
                checked={fallback === "approved"}
                onChange={() => setFallback("approved")}
              />
              <span>{tl("Approved")}</span>
            </label>
            <label className="check">
              <input
                type="radio"
                name="imp-fallback"
                checked={fallback === "proposed"}
                onChange={() => setFallback("proposed")}
              />
              <span>{tl("Proposed, for lists nobody has checked")}</span>
            </label>
          </fieldset>
        </>
      )}

      {step === "review" && (
        <>
          {existing.length > 0 && (
            <div className="field-grid">
              <fieldset className="field">
                <legend>{tl("Find existing entries by")}</legend>
                <label className="check">
                  <input
                    type="radio"
                    name="imp-match"
                    checked={policy.match === "term"}
                    onChange={() => setPolicy((p) => ({ ...p, match: "term" }))}
                  />
                  <span>{tl("A shared term")}</span>
                </label>
                {policy.match === "term" && (
                  <div className="nested">
                    <label htmlFor="imp-match-lang" className="sr-only">
                      {tl("Language of the shared term")}
                    </label>
                    <select
                      id="imp-match-lang"
                      value={policy.lang ?? ""}
                      onChange={(e) =>
                        setPolicy((p) => ({
                          ...p,
                          lang: e.target.value || undefined,
                        }))
                      }
                    >
                      <option value="">{tl("In any language")}</option>
                      {usedLangs.map((l) => (
                        <option key={l} value={l}>
                          {tlx("In {name}", { name: lang(l).name })}</option>
                      ))}
                    </select>
                  </div>
                )}
                <label className="check">
                  <input
                    type="radio"
                    name="imp-match"
                    checked={policy.match === "id"}
                    onChange={() => setPolicy((p) => ({ ...p, match: "id" }))}
                  />
                  <span>
                    {tl("The entry ID (for a file exported from this termbase)")}
                  </span>
                </label>
                <label className="check">
                  <input
                    type="radio"
                    name="imp-match"
                    checked={policy.match === "none"}
                    onChange={() => setPolicy((p) => ({ ...p, match: "none" }))}
                  />
                  <span>{tl("Nothing: add every entry")}</span>
                </label>
              </fieldset>
              {policy.match !== "none" && (
                <fieldset className="field">
                  <legend>{tl("When an entry exists")}</legend>
                  {(
                    [
                      ["merge", tl("Merge: add the new terms and field values")],
                      ["overwrite", tl("Replace it with the entry from the file")],
                      ["skip", tl("Keep it and leave the file's entry out")],
                      ["add", tl("Add the file's entry as a separate entry")],
                    ] as const
                  ).map(([v, label]) => (
                    <label key={v} className="check">
                      <input
                        type="radio"
                        name="imp-on"
                        checked={policy.onMatch === v}
                        onChange={() =>
                          setPolicy((p) => ({ ...p, onMatch: v }))
                        }
                      />
                      <span>{label}</span>
                    </label>
                  ))}
                </fieldset>
              )}
            </div>
          )}
          {plan && (
            <PlanSummary
              plan={plan}
              incoming={incoming.entries}
              existing={existing}
              dest={dest}
            />
          )}
        </>
      )}

      {(error || planError) && (
        <p className="error" role="alert">
          {error || planError}
        </p>
      )}

      <div className="row">
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={onClose}
        >
          {tl("Cancel")}
        </button>
        {step !== "file" && (
          <button
            type="button"
            className="btn btn-small"
            onClick={() => {
              setError("");
              setStep(steps[steps.indexOf(step) - 1]!);
            }}
          >
            {tl("Back")}
          </button>
        )}
        {step === "columns" && (
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={
              !mapping?.columns.some((col) => col.role === "term" && col.lang)
            }
            onClick={() => loaded && mapping && readFile(loaded, mapping)}
          >
            {tl("Continue")}
          </button>
        )}
        {step === "languages" && (
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={incoming.entries.length === 0}
            onClick={() => setStep("review")}
          >
            {tl("Continue")}
          </button>
        )}
        {step === "review" && (
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={
              !plan ||
              (target === "this" &&
                plan.actions.every((a) => a.kind === "skip"))
            }
            onClick={doImport}
          >
            {tl("Import")}
          </button>
        )}
      </div>
    </Modal>
  );
}

function Warnings({ items }: { items?: string[] }) {
  if (!items?.length) return null;
  return (
    <ul className="import-warnings">
      {items.slice(0, 6).map((w) => (
        <li key={w} className="detect detect-warn">
          <AlertTriangle size={16} aria-hidden="true" />
          <span>{w}</span>
        </li>
      ))}
      {items.length > 6 && (
        <li className="help">{tlx("and {n} more notes", { n: items.length - 6 })}</li>
      )}
    </ul>
  );
}

function ColumnStep({
  loaded,
  mapping,
  setMapping,
}: {
  loaded: Loaded;
  mapping: Mapping;
  setMapping: (m: Mapping) => void;
}) {
  const rows = loaded.detected.rows ?? [];
  const width = Math.max(mapping.columns.length, ...rows.map((r) => r.length));
  const body = mapping.header ? rows.slice(1) : rows;
  const langCodes = [
    ...new Set([
      ...LANGUAGES.map((l) => l.code),
      ...mapping.columns.map((col) => col.lang).filter((l): l is string => !!l),
    ]),
  ];
  const set = (i: number, patch: Partial<Mapping["columns"][number]>) =>
    setMapping({
      ...mapping,
      columns: mapping.columns.map((col) =>
        col.index === i ? { ...col, ...patch } : col,
      ),
    });
  const hasTerms = mapping.columns.some(
    (col) => col.role === "term" && col.lang,
  );
  return (
    <>
      <p className="help">
        {tlx("{name}: {plural} of entries. Each row is one entry. Check what each column holds; the guess comes from the column names.", { name: loaded.name, plural: plural(
          (loaded.detected.total ?? rows.length) - (mapping.header ? 1 : 0),
          "row",
          "rows",
        ) })}
      </p>
      <div className="row row-start">
        <label className="check">
          <input
            type="checkbox"
            checked={mapping.header}
            onChange={(e) =>
              setMapping({ ...mapping, header: e.target.checked })
            }
          />
          <span>{tl("The first row holds column names")}</span>
        </label>
        <label htmlFor="imp-sep" className="inline-label">
          {tl("Synonyms in one cell separated by")}
        </label>
        <input
          id="imp-sep"
          className="input-tiny"
          value={mapping.separator ?? ";"}
          maxLength={3}
          onChange={(e) =>
            setMapping({ ...mapping, separator: e.target.value })
          }
        />
      </div>
      <div className="table-wrap mapping-wrap">
        <table className="table mapping">
          <caption className="sr-only">{tl("Columns of {name}", { name: loaded.name })}</caption>
          <thead>
            <tr>
              {Array.from({ length: width }, (_, i) => {
                const col = mapping.columns.find((x) => x.index === i) ?? {
                  index: i,
                  role: "ignore" as const,
                };
                return (
                  <th key={i} scope="col">
                    <span className="mapping-head">
                      {col.header || tl("Column {n}", { n: i + 1 })}
                    </span>
                    <label htmlFor={`col-${i}-role`} className="sr-only">
                      {tlx("Column {n} holds", { n: i + 1 })}
                    </label>
                    <select
                      id={`col-${i}-role`}
                      value={col.role}
                      onChange={(e) =>
                        set(i, {
                          role: e.target.value as ColumnRole,
                          field:
                            e.target.value === "field"
                              ? col.field || col.header || `Column ${i + 1}`
                              : undefined,
                          level:
                            e.target.value === "field"
                              ? (col.level ?? (col.lang ? "language" : "entry"))
                              : undefined,
                        })
                      }
                    >
                      {(Object.keys(ROLE_LABEL) as ColumnRole[]).map((r) => (
                        <option key={r} value={r}>
                          {ROLE_LABEL[r]}
                        </option>
                      ))}
                    </select>
                    {(col.role === "term" ||
                      col.role === "status" ||
                      (col.role === "field" && col.level !== "entry")) && (
                      <>
                        <label htmlFor={`col-${i}-lang`} className="sr-only">
                          {tlx("Language of column {n}", { n: i + 1 })}</label>
                        <select
                          id={`col-${i}-lang`}
                          value={col.lang ?? ""}
                          onChange={(e) =>
                            set(i, { lang: e.target.value || undefined })
                          }
                        >
                          <option value="">{tl("Language…")}</option>
                          {langCodes.map((code) => (
                            <option key={code} value={code}>
                              {lang(code).name}
                            </option>
                          ))}
                        </select>
                      </>
                    )}
                    {col.role === "field" && (
                      <>
                        <label htmlFor={`col-${i}-field`} className="sr-only">
                          {tlx("Field name of column {n}", { n: i + 1 })}</label>
                        <input
                          id={`col-${i}-field`}
                          value={col.field ?? ""}
                          placeholder={tl("Field name")}
                          onChange={(e) => set(i, { field: e.target.value })}
                        />
                        <label htmlFor={`col-${i}-level`} className="sr-only">
                          {tlx("Column {n} applies to", { n: i + 1 })}
                        </label>
                        <select
                          id={`col-${i}-level`}
                          value={col.level ?? "entry"}
                          onChange={(e) =>
                            set(i, { level: e.target.value as CoreLevel })
                          }
                        >
                          <option value="entry">{tl("Whole entry")}</option>
                          <option value="language">{tl("One language")}</option>
                          <option value="term">{tl("The terms")}</option>
                        </select>
                      </>
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {body.slice(0, 8).map((r, ri) => (
              <tr key={ri}>
                {Array.from({ length: width }, (_, i) => (
                  <td
                    key={i}
                    className={
                      mapping.columns.find((x) => x.index === i)?.role ===
                      "ignore"
                        ? "muted"
                        : undefined
                    }
                  >
                    {r[i] ?? ""}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!hasTerms && (
        <p className="error" role="alert">
          {tl("Mark at least one column as terms and give it a language.")}
        </p>
      )}
    </>
  );
}

function termsOf(e: CoreEntry): string {
  return e.languages
    .flatMap((l) => l.terms.slice(0, 2).map((t) => t.text))
    .slice(0, 4)
    .join(", ");
}

const KIND_LABEL: Record<PlanAction["kind"], string> = {
  add: tl("New"),
  merge: tl("Merged"),
  overwrite: tl("Replaced"),
  skip: tl("Unchanged"),
};

function PlanSummary({
  plan,
  incoming,
  existing,
  dest,
}: {
  plan: { actions: PlanAction[]; report: PlanReport };
  incoming: CoreEntry[];
  existing: Entry[];
  dest?: Termbase;
}) {
  const r = plan.report;
  const byCoreId = new Map(existing.map((e) => [e.externalId ?? e.id, e]));
  const src = dest?.languages[0];
  const label = (id?: string) => {
    const e = id ? byCoreId.get(id) : undefined;
    if (!e) return id ?? "";
    return (
      (src && e.terms[src]?.[0]?.text) ||
      Object.values(e.terms).flat()[0]?.text ||
      id
    );
  };
  const shown = plan.actions.slice(0, 40);
  return (
    <section aria-labelledby="plan-title" className="plan">
      <h3 id="plan-title" className="settings-title">
        {tl("What the import does")}
      </h3>
      <dl className="plan-counts">
        <div>
          <dt>{tl("New entries")}</dt>
          <dd>{r.added}</dd>
        </div>
        <div>
          <dt>{tl("Merged")}</dt>
          <dd>{r.merged}</dd>
        </div>
        <div>
          <dt>{tl("Replaced")}</dt>
          <dd>{r.overwritten}</dd>
        </div>
        <div>
          <dt>{tl("Unchanged")}</dt>
          <dd>{r.skipped + r.unchanged}</dd>
        </div>
      </dl>
      <Warnings items={r.warnings} />
      <div className="table-wrap plan-wrap">
        <table className="table">
          <caption className="sr-only">
            {tl("Entries of the file and what happens to them")}
          </caption>
          <thead>
            <tr>
              <th scope="col">{tl("Entry in the file")}</th>
              <th scope="col">{tl("Result")}</th>
              <th scope="col">{tl("Existing entry")}</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((a, i) => (
              <tr key={i}>
                <td>{termsOf(incoming[a.incoming]!)}</td>
                <td>
                  <span className={`tag tag-plan-${a.kind}`}>
                    {KIND_LABEL[a.kind]}
                  </span>
                </td>
                <td>
                  {a.kind === "add" ? (
                    <span className="muted">{tl("none")}</span>
                  ) : (
                    label(a.existing)
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {plan.actions.length > shown.length && (
        <p className="help">
          {tlx("and {plural}", { plural: plural(
            plan.actions.length - shown.length,
            "more entry",
            "more entries",
          ) })}</p>
      )}
    </section>
  );
}
