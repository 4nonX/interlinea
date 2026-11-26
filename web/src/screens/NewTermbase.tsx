import { useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, Search } from "lucide-react";
import {
  FIELD_KINDS,
  FIELD_LEVELS,
  lang,
  LANGUAGES,
  type InfoType,
  type LangCode,
  type Location,
  type Termbase,
} from "../data";
import { useStore } from "../store";
import { useRemote, errorText } from "../remote";
import { infoTypesFromCore, LEVEL_TO_CORE } from "../termio";
import { go } from "../App";
import { Modal } from "./TermbaseDialogs";
import {
  FieldTable,
  fieldProblem,
  newDraft,
  STRUCTURES,
  type FieldDraft,
} from "./TermbaseFields";
import { tl, tlx } from "../i18n";
import { listOf } from "../format";

type Kind = "master" | "project";
type Step = "start" | "languages" | "fields" | "review";

const STEP_NAME: Record<Step, string> = {
  start: tl("Start"),
  languages: tl("Languages"),
  fields: tl("Fields"),
  review: tl("Review"),
};

/** A field as the core keeps it. */
function toCore(f: Pick<FieldDraft, "name" | "level" | "kind" | "values">) {
  return {
    name: f.name.trim(),
    level: LEVEL_TO_CORE[f.level],
    kind: f.kind,
    ...(f.kind === "picklist" ? { values: f.values } : {}),
  };
}

/** A field as it is described in a list: “Gender (term, a list of values)”. */
function fieldLine(f: Pick<FieldDraft, "name" | "level" | "kind" | "values">) {
  const level =
    FIELD_LEVELS.find((l) => l.level === f.level)?.name.toLocaleLowerCase() ??
    f.level;
  const kind =
    FIELD_KINDS.find((k) => k.kind === f.kind)?.name.toLocaleLowerCase() ??
    f.kind;
  return `${f.name.trim()} (${level}, ${kind}${f.kind === "picklist" ? `: ${f.values.join(", ")}` : ""})`;
}

/**
 * Languages chosen from the list of all languages, as checkboxes, with a
 * search: nobody has to know which languages there are or how they are
 * written.
 */
export function LanguageChecklist({
  chosen,
  onChange,
  label,
}: {
  chosen: LangCode[];
  onChange: (l: LangCode[]) => void;
  label: string;
}) {
  const [find, setFind] = useState("");
  const f = find.trim().toLocaleLowerCase();
  const shown = LANGUAGES.filter(
    (l) =>
      !f ||
      l.name.toLocaleLowerCase().includes(f) ||
      l.code.toLocaleLowerCase().includes(f),
  );
  return (
    <div className="lang-pick">
      {chosen.length > 0 && (
        <ul className="lang-list" aria-label={tl("Chosen languages")}>
          {chosen.map((c) => (
            <li key={c}>
              <button
                type="button"
                className="chip-button"
                aria-label={tl("Remove {name}", { name: lang(c).name })}
                onClick={() => onChange(chosen.filter((x) => x !== c))}
              >
                {lang(c).name} ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <label className="search-field">
        <Search size={14} aria-hidden="true" />
        <span className="sr-only">{tl("Find a language")}</span>
        <input
          type="search"
          value={find}
          placeholder={tl("Find a language, such as German or de-AT")}
          onChange={(e) => setFind(e.target.value)}
        />
      </label>
      <div className="lang-pick-list" role="group" aria-label={label}>
        {shown.length === 0 ? (
          <p className="muted">{tl("No language is called that.")}</p>
        ) : (
          shown.map((l) => (
            <label key={l.code} className="check">
              <input
                type="checkbox"
                checked={chosen.includes(l.code)}
                onChange={(e) =>
                  onChange(
                    e.target.checked
                      ? [...chosen, l.code]
                      : chosen.filter((x) => x !== l.code),
                  )
                }
              />
              <span>
                {l.name} <span className="muted">{l.code}</span>
              </span>
            </label>
          ))
        )}
      </div>
    </div>
  );
}

/**
 * Makes a termbase, in steps. A master is a termbase of its own: its
 * languages, and its fields, started from a ready-made structure or from
 * another termbase's. A project termbase is based on masters, as a branch
 * is on what it branches from: it has their languages and fields, may add
 * fields of its own, and sends its new and changed entries to them for
 * review, with the fields they use.
 */
export function NewTermbaseDialog({
  onClose,
  kind: firstKind = "master",
}: {
  onClose: () => void;
  kind?: Kind;
}) {
  const { state, dispatch, notify } = useStore();
  const r = useRemote();
  const [step, setStep] = useState<Step>("start");
  const [name, setName] = useState("");
  const [loc, setLoc] = useState<Location>(r.api ? "server" : "local");
  const [kind, setKind] = useState<Kind>(firstKind);
  const [langs, setLangs] = useState<LangCode[]>([]);
  const [start, setStart] = useState("terms");
  const [fields, setFields] = useState<FieldDraft[]>([]);
  const [masters, setMasters] = useState<
    { name: string; languages: string[] }[] | null
  >(null);
  const [based, setBased] = useState<string[]>([]);
  const [inherited, setInherited] = useState<{
    languages: string[];
    fields: Omit<FieldDraft, "key">[];
  }>({ languages: [], fields: [] });
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const heading = useRef<HTMLHeadingElement>(null);

  // A project termbase is based on masters of the same place: on the team
  // server. On this computer, a project's termbase is made with the project.
  const canBranch = loc === "server" && !!r.api;
  useEffect(() => {
    if (!canBranch && kind === "project") setKind("master");
  }, [canBranch, kind]);

  // The masters on the server, to base a project termbase on, or to take a structure from.
  useEffect(() => {
    if (loc !== "server" || !r.api) {
      setMasters(null);
      return;
    }
    let live = true;
    r.api.termbases().then(
      (list) =>
        live &&
        setMasters(
          list
            .filter((t) => t.kind === "master")
            .map((t) => ({ name: t.name, languages: t.languages ?? [] })),
        ),
      () => live && setMasters([]),
    );
    return () => {
      live = false;
    };
  }, [loc, r.api]);

  // What the chosen masters give a project termbase.
  useEffect(() => {
    if (kind !== "project" || !r.api || based.length === 0) {
      setInherited({ languages: [], fields: [] });
      return;
    }
    let live = true;
    Promise.all(based.map((m) => r.api!.termbase(m))).then(
      (ds) => {
        if (!live) return;
        const languages: string[] = [];
        const fs: Omit<FieldDraft, "key">[] = [];
        for (const d of ds) {
          for (const l of d.definition.languages)
            if (!languages.includes(l)) languages.push(l);
          for (const it of infoTypesFromCore(d.definition.fields as never)) {
            if (
              !fs.some(
                (f) =>
                  f.name.toLocaleLowerCase() === it.name.toLocaleLowerCase(),
              )
            )
              fs.push({
                name: it.name,
                level: it.level,
                kind: it.kind,
                values: it.values ?? [],
              });
          }
        }
        setInherited({ languages, fields: fs });
      },
      (e) => live && setProblem(errorText(e)),
    );
    return () => {
      live = false;
    };
  }, [kind, based, r.api]);

  // Structures of the termbases there are, to start from.
  const localSources = state.termbases.filter(
    (t) => !t.remote && t.infoTypes.length > 0,
  );
  const serverSources = (masters ?? []).map((m) => m.name);
  const chooseStart = async (id: string) => {
    setStart(id);
    setProblem("");
    const ready = STRUCTURES.find((s) => s.id === id);
    if (ready) {
      setFields(
        ready.fields.map((f) => newDraft({ ...f, values: [...f.values] })),
      );
      return;
    }
    if (id.startsWith("local:")) {
      const tb = state.termbases.find((t) => t.id === id.slice(6));
      setFields(
        (tb?.infoTypes ?? []).map((it) =>
          newDraft({
            name: it.name,
            level: it.level,
            kind: it.kind,
            values: [...(it.values ?? [])],
          }),
        ),
      );
      return;
    }
    if (id.startsWith("server:") && r.api) {
      try {
        const d = await r.api.termbase(id.slice(7));
        setFields(
          infoTypesFromCore(d.definition.fields as never).map((it) =>
            newDraft({
              name: it.name,
              level: it.level,
              kind: it.kind,
              values: [...(it.values ?? [])],
            }),
          ),
        );
      } catch (e) {
        setProblem(errorText(e));
      }
    }
  };

  const taken = useMemo(() => {
    const names = new Set<string>();
    for (const t of state.termbases)
      if ((t.remote ? "server" : "local") === loc)
        names.add(t.name.toLocaleLowerCase());
    for (const m of masters ?? []) names.add(m.name.toLocaleLowerCase());
    return names;
  }, [state.termbases, masters, loc]);

  const steps: Step[] =
    kind === "master"
      ? ["start", "languages", "fields", "review"]
      : ["start", "fields", "review"];
  const at = steps.indexOf(step);
  const ownFieldProblems = fields
    .map((f) =>
      fieldProblem(f, [
        ...fields.filter((o) => o.key !== f.key).map((o) => o.name),
        ...(kind === "project" ? inherited.fields.map((i) => i.name) : []),
      ]),
    )
    .filter(Boolean);
  const stepProblem = (s: Step): string => {
    switch (s) {
      case "start":
        if (!name.trim()) return tl("Give the termbase a name.");
        if (taken.has(name.trim().toLocaleLowerCase()))
          return tl("A termbase with this name is there already.");
        if (kind === "project" && based.length === 0)
          return tl("Choose the masters it is based on.");
        return "";
      case "languages":
        return langs.length === 0 ? tl("Choose its languages, at least one.") : "";
      case "fields":
        return ownFieldProblems[0] ?? "";
      default:
        return "";
    }
  };
  const blocking = stepProblem(step);
  const move = (to: Step) => {
    setStep(to);
    setProblem("");
    requestAnimationFrame(() => heading.current?.focus());
  };

  const create = async () => {
    const own = fields.map(toCore);
    const n = name.trim();
    setBusy(true);
    setProblem("");
    try {
      if (loc === "server" && r.api) {
        if (kind === "project")
          await r.api.createTermbase({ name: n, fields: own }, based);
        else
          await r.api.createTermbase({
            name: n,
            languages: langs,
            fields: own,
          });
        notify(
          kind === "project"
            ? tl("Created project termbase “{n}”", { n })
            : tl("Created master “{n}”", { n }),
        );
        const id = await r.openTermbase(n);
        onClose();
        window.location.hash = `#/terminology/${id}`;
        return;
      }
      const id = `tb-${Date.now().toString(36)}`;
      const infoTypes: InfoType[] = fields.map((f, i) => ({
        id: `it-${Date.now().toString(36)}-${i}`,
        name: f.name.trim(),
        level: f.level,
        kind: f.kind,
        ...(f.kind === "picklist" ? { values: f.values } : {}),
      }));
      const tb: Termbase = {
        id,
        name: n,
        kind: "master",
        location: "local",
        languages: langs,
        infoTypes,
      };
      dispatch({ type: "createMaster", termbase: tb });
      notify(tl("Created master “{n}”", { n }));
      onClose();
      go({ page: "terminology", termbaseId: id });
    } catch (e) {
      setProblem(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={tl("New termbase")} onClose={onClose} wide>
      <ol className="stepper stepper-small" aria-label={tl("Steps")}>
        {steps.map((s, i) => (
          <li
            key={s}
            aria-current={s === step ? "step" : undefined}
            className={at > i ? "is-done" : undefined}
          >
            <span className="stepper-num" aria-hidden="true">
              {at > i ? <CheckCircle2 size={14} /> : i + 1}
            </span>
            <span>{STEP_NAME[s]}</span>
          </li>
        ))}
      </ol>
      <h3 className="sr-only" tabIndex={-1} ref={heading}>
        {STEP_NAME[step]}
      </h3>

      {step === "start" && (
        <>
          <div className="field">
            <label htmlFor="ntb-name">{tl("Name")}</label>
            <input
              id="ntb-name"
              autoFocus
              value={name}
              placeholder={
                kind === "project"
                  ? tl("For example a case or an order")
                  : tl("For example a client or a subject")
              }
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          {r.api && (
            <fieldset className="field">
              <legend>{tl("Where it is kept")}</legend>
              <label className="check">
                <input
                  type="radio"
                  name="ntb-loc"
                  checked={loc === "server"}
                  onChange={() => setLoc("server")}
                />
                <span>{tl("On the team server, for everyone working with it")}</span>
              </label>
              <label className="check">
                <input
                  type="radio"
                  name="ntb-loc"
                  checked={loc === "local"}
                  onChange={() => setLoc("local")}
                />
                <span>{tl("In this browser")}</span>
              </label>
            </fieldset>
          )}
          <fieldset className="field">
            <legend>{tl("What kind of termbase")}</legend>
            <label className="check">
              <input
                type="radio"
                name="ntb-kind"
                checked={kind === "master"}
                onChange={() => setKind("master")}
              />
              <span>
                {tlx("{strong} A termbase of its own, kept across projects, for a client, a subject or a company. You choose its languages and fields.", { strong: <strong>{tl("A master.")}</strong> })}
              </span>
            </label>
            <label className="check">
              <input
                type="radio"
                name="ntb-kind"
                checked={kind === "project"}
                disabled={!canBranch}
                onChange={() => setKind("project")}
              />
              <span>
                {tlx("{strong} Based on masters: it shows their entries, gets their languages and fields, and sends its new and changed entries to them for review.{x}", { strong: <strong>{tl("A project termbase.")}</strong>, x: !canBranch && (
                  <span className="muted">
                    {" "}{tl("Made on the team server; in this browser a project gets its termbase with the project.")}
                  </span>
                ) })}</span>
            </label>
          </fieldset>
          {kind === "project" && (
            <fieldset className="field">
              <legend>{tl("Based on")}</legend>
              {masters === null ? (
                <p className="muted">{tl("Loading…")}</p>
              ) : masters.length === 0 ? (
                <p className="help">
                  {tl("There is no master on the server yet. Make one first.")}
                </p>
              ) : (
                masters.map((m) => (
                  <label key={m.name} className="check">
                    <input
                      type="checkbox"
                      checked={based.includes(m.name)}
                      onChange={(e) =>
                        setBased((b) =>
                          e.target.checked
                            ? [...b, m.name]
                            : b.filter((x) => x !== m.name),
                        )
                      }
                    />
                    <span>
                      {m.name}{" "}
                      {m.languages.length > 0 && (
                        <span className="muted">{m.languages.join(", ")}</span>
                      )}
                    </span>
                  </label>
                ))
              )}
              <p className="help">
                {tl("The first master chosen comes first where masters have the same entry.")}
              </p>
            </fieldset>
          )}
        </>
      )}

      {step === "languages" && (
        <div className="field">
          <span className="field-label">{tl("The languages of")}{" "}{name.trim()}</span>
          <LanguageChecklist
            chosen={langs}
            onChange={setLangs}
            label={tl("Languages of the termbase")}
          />
          <p className="help">
            {tl("A termbase has no source and target: it has languages, and a project looks up terms in its own. More languages can be added later.")}
          </p>
        </div>
      )}

      {step === "fields" && (
        <>
          {kind === "master" ? (
            <fieldset className="field">
              <legend>{tl("Start from")}</legend>
              {STRUCTURES.map((s) => (
                <label key={s.id} className="check">
                  <input
                    type="radio"
                    name="ntb-start"
                    checked={start === s.id}
                    onChange={() => void chooseStart(s.id)}
                  />
                  <span>
                    <strong>{s.name}.</strong> {s.text}
                  </span>
                </label>
              ))}
              {(loc === "local"
                ? localSources.length > 0
                : serverSources.length > 0) && (
                <label className="check check-select">
                  <input
                    type="radio"
                    name="ntb-start"
                    checked={
                      start.startsWith("local:") || start.startsWith("server:")
                    }
                    onChange={() =>
                      void chooseStart(
                        loc === "local"
                          ? `local:${localSources[0]!.id}`
                          : `server:${serverSources[0]}`,
                      )
                    }
                  />
                  <span>{tl("The fields of")}</span>
                  <select
                    aria-label={tl("The termbase to take the fields from")}
                    value={
                      start.startsWith("local:") || start.startsWith("server:")
                        ? start
                        : ""
                    }
                    onChange={(e) =>
                      e.target.value && void chooseStart(e.target.value)
                    }
                  >
                    <option value="">{tl("a termbase…")}</option>
                    {loc === "local"
                      ? localSources.map((t) => (
                          <option key={t.id} value={`local:${t.id}`}>
                            {t.name}
                          </option>
                        ))
                      : serverSources.map((n) => (
                          <option key={n} value={`server:${n}`}>
                            {n}
                          </option>
                        ))}
                  </select>
                </label>
              )}
              <p className="help">
                {tl("Choosing a start sets the fields below; change them as you need. Every term has a status (approved, proposed or forbidden) without a field for it.")}
              </p>
            </fieldset>
          ) : (
            <div className="field">
              <span className="field-label">{tl("From")}{" "}{listOf(based)}</span>
              <p className="help">
                {inherited.languages.length > 0
                  ? `Languages: ${inherited.languages.map((l) => lang(l).name).join(", ")}. `
                  : ""}
                {inherited.fields.length > 0
                  ? `Fields: ${inherited.fields.map(fieldLine).join("; ")}.`
                  : tl("The masters have no fields.")}
              </p>
              <span className="field-label">{tl("Fields of its own")}</span>
              <p className="help">
                {tl("Only if the project needs more than the masters have. A field of its own reaches a master with the first entry that uses it, once a reviewer approves the entry.")}
              </p>
            </div>
          )}
          <FieldTable fields={fields} onChange={setFields} />
        </>
      )}

      {step === "review" && (
        <dl className="review-list">
          <dt>{tl("Name")}</dt>
          <dd>{name.trim()}</dd>
          {r.api && (
            <>
              <dt>{tl("Kept")}</dt>
              <dd>
                {loc === "server" ? tl("On the team server") : tl("In this browser")}
              </dd>
            </>
          )}
          <dt>{tl("Kind")}</dt>
          <dd>
            {kind === "master"
              ? tl("A master")
              : tl("A project termbase based on {join}", { join: listOf(based) })}
          </dd>
          <dt>{tl("Languages")}</dt>
          <dd>
            {(kind === "master" ? langs : inherited.languages)
              .map((l) => lang(l).name)
              .join(", ") || tl("None yet")}
          </dd>
          <dt>{tl("Fields")}</dt>
          <dd>
            {[...(kind === "project" ? inherited.fields : []), ...fields]
              .length === 0 ? (
              tl("None: terms and their status")
            ) : (
              <ul className="plain-list">
                {(kind === "project" ? inherited.fields : []).map((f) => (
                  <li key={`i-${f.name}`}>
                    {fieldLine(f)}{" "}
                    <span className="muted">{tl("from the masters")}</span>
                  </li>
                ))}
                {fields.map((f) => (
                  <li key={f.key}>{fieldLine(f)}</li>
                ))}
              </ul>
            )}
          </dd>
        </dl>
      )}

      {(problem || (blocking && step !== "start" && step !== "languages")) && (
        <p className="error" role="alert">
          {problem || blocking}
        </p>
      )}
      <div className="row row-end">
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={onClose}
        >
          {tl("Cancel")}
        </button>
        {at > 0 && (
          <button
            type="button"
            className="btn btn-small"
            onClick={() => move(steps[at - 1]!)}
          >
            {tl("Back")}
          </button>
        )}
        {step !== "review" ? (
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={!!blocking}
            title={blocking || undefined}
            onClick={() => move(steps[at + 1]!)}
          >
            {tl("Continue")}
          </button>
        ) : (
          <button
            type="button"
            className="btn btn-primary btn-small"
            disabled={busy}
            onClick={() => void create()}
          >
            {kind === "project" ? tl("Create project termbase") : tl("Create master")}
          </button>
        )}
      </div>
      {blocking &&
        (step === "start" || step === "languages") &&
        (step === "languages" || name.trim() !== "") && (
          <p className="help" aria-live="polite">
            {blocking}
          </p>
        )}
    </Modal>
  );
}
