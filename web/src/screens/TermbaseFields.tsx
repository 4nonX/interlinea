import { useEffect, useId, useRef, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import {
  FIELD_KINDS,
  FIELD_LEVELS,
  type FieldKind,
  type InfoType,
} from "../data";
import { tl } from "../i18n";

/**
 * The fields of a termbase as they are being defined: what a value is
 * called, where it belongs (the entry, a language or a term) and what
 * kind of value it takes. As in a termbase definition of other tools,
 * a field can hold free text, one value of a list, a number, a date or
 * yes or no.
 */
export interface FieldDraft {
  /** Tells drafts apart while their names are typed. */
  key: string;
  name: string;
  level: InfoType["level"];
  kind: FieldKind;
  /** The values of a list. */
  values: string[];
}

let drafted = 0;
export function newDraft(f?: Partial<Omit<FieldDraft, "key">>): FieldDraft {
  return {
    key: `f${++drafted}`,
    name: "",
    level: "concept",
    kind: "text",
    values: [],
    ...f,
  };
}

type Shape = Omit<FieldDraft, "key">;
const text = (name: string, level: InfoType["level"]): Shape => ({
  name,
  level,
  kind: "text",
  values: [],
});
const list = (
  name: string,
  level: InfoType["level"],
  values: string[],
): Shape => ({ name, level, kind: "picklist", values });

/** Structures to start a termbase from, from a plain glossary to full terminology. */
export const STRUCTURES: {
  id: string;
  name: string;
  text: string;
  fields: Shape[];
}[] = [
  {
    id: "terms",
    name: tl("Terms only"),
    text: tl("A glossary: the terms of each language, each with its status. Fields can be added later."),
    fields: [],
  },
  {
    id: "notes",
    name: tl("Terms with definitions and notes"),
    text: tl("A definition and its source for each language, and a note on the entry."),
    fields: [
      text(tl("Definition"), "language"),
      text(tl("Source"), "language"),
      text(tl("Note"), "concept"),
    ],
  },
  {
    id: "terminology",
    name: tl("Terminology with grammar"),
    text: tl("Subject field, notes and the date it was last checked; definitions per language; part of speech, gender, number, context and usage per term."),
    fields: [
      text(tl("Subject field"), "concept"),
      text(tl("Note"), "concept"),
      { name: tl("Last checked"), level: "concept", kind: "date", values: [] },
      text(tl("Definition"), "language"),
      text(tl("Source of the definition"), "language"),
      list(tl("Part of speech"), "term", [
        "noun",
        "verb",
        "adjective",
        "adverb",
        tl("proper noun"),
        "other",
      ]),
      list(tl("Gender"), "term", ["masculine", "feminine", "neuter", "common"]),
      list(tl("Number"), "term", ["singular", "plural"]),
      text(tl("Context"), "term"),
      text(tl("Usage note"), "term"),
    ],
  },
];

/** What keeps a field from being saved, or nothing. */
export function fieldProblem(
  f: Pick<FieldDraft, "name" | "kind" | "values">,
  others: string[],
): string {
  const name = f.name.trim();
  if (!name) return tl("Give the field a name.");
  if (
    others.some(
      (o) => o.trim().toLocaleLowerCase() === name.toLocaleLowerCase(),
    )
  )
    return tl("Another field has this name.");
  if (f.kind === "picklist" && f.values.length === 0)
    return tl("Give the list its values, one a line.");
  return "";
}

/** The values of a list, one a line, as they are typed. */
export function ValuesEditor({
  values,
  onChange,
  label,
  id,
}: {
  values: string[];
  onChange: (v: string[]) => void;
  label: string;
  id?: string;
}) {
  const [draft, setDraft] = useState(values.join("\n"));
  const shown = useRef(values);
  useEffect(() => {
    if (shown.current.join("\n") !== values.join("\n")) {
      shown.current = values;
      setDraft(values.join("\n"));
    }
  }, [values]);
  const read = (t: string) => [
    ...new Set(
      t
        .split("\n")
        .map((v) => v.trim())
        .filter(Boolean),
    ),
  ];
  return (
    <textarea
      id={id}
      className="values-editor"
      aria-label={label}
      rows={Math.min(6, Math.max(3, values.length + 1))}
      value={draft}
      placeholder={tl("One value a line\nsuch as\nnoun\nverb")}
      onChange={(e) => {
        setDraft(e.target.value);
        const now = read(e.target.value);
        shown.current = now;
        onChange(now);
      }}
      onBlur={() => setDraft(read(draft).join("\n"))}
    />
  );
}

/** The kind of value a field takes. */
export function KindSelect({
  value,
  onChange,
  label,
  id,
}: {
  value: FieldKind;
  onChange: (k: FieldKind) => void;
  label: string;
  id?: string;
}) {
  return (
    <select
      id={id}
      aria-label={label}
      value={value}
      onChange={(e) => onChange(e.target.value as FieldKind)}
    >
      {FIELD_KINDS.map((k) => (
        <option key={k.kind} value={k.kind}>
          {k.name}
        </option>
      ))}
    </select>
  );
}

/** The fields of a termbase being made: each can be changed or taken out, and more added. */
export function FieldTable({
  fields,
  onChange,
}: {
  fields: FieldDraft[];
  onChange: (f: FieldDraft[]) => void;
}) {
  const lastName = useRef<HTMLInputElement>(null);
  const [added, setAdded] = useState(0);
  useEffect(() => {
    if (added) lastName.current?.focus();
  }, [added]);
  const set = (key: string, patch: Partial<FieldDraft>) =>
    onChange(fields.map((f) => (f.key === key ? { ...f, ...patch } : f)));
  return (
    <div className="field-table">
      {fields.length === 0 ? (
        <p className="help">
          {tl("No fields: entries hold their terms and each term's status.")}
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{tl("Name")}</th>
              <th scope="col">{tl("Belongs to")}</th>
              <th scope="col">{tl("Values")}</th>
              <th scope="col">
                <span className="sr-only">{tl("Remove")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {fields.map((f, i) => {
              const problem = fieldProblem(
                f,
                fields.filter((o) => o.key !== f.key).map((o) => o.name),
              );
              const label = f.name.trim() || `field ${i + 1}`;
              return (
                <tr key={f.key}>
                  <td>
                    <input
                      ref={i === fields.length - 1 ? lastName : undefined}
                      value={f.name}
                      aria-label={tl("Name of field {x}", { x: i + 1 })}
                      aria-invalid={!!problem}
                      placeholder={tl("Name of the field")}
                      onChange={(e) => set(f.key, { name: e.target.value })}
                    />
                    {problem && <p className="field-problem">{problem}</p>}
                  </td>
                  <td>
                    <select
                      aria-label={tl("Where {label} belongs", { label })}
                      value={f.level}
                      onChange={(e) =>
                        set(f.key, {
                          level: e.target.value as InfoType["level"],
                        })
                      }
                    >
                      {FIELD_LEVELS.map((l) => (
                        <option key={l.level} value={l.level}>
                          {l.name}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <div className="field-kind">
                      <KindSelect
                        label={tl("Values of {label}", { label })}
                        value={f.kind}
                        onChange={(kind) => set(f.key, { kind })}
                      />
                      {f.kind === "picklist" && (
                        <ValuesEditor
                          label={tl("The values of {label}, one a line", { label })}
                          values={f.values}
                          onChange={(values) => set(f.key, { values })}
                        />
                      )}
                    </div>
                  </td>
                  <td className="cell-action">
                    <button
                      type="button"
                      className="btn btn-quiet btn-small btn-icon"
                      aria-label={tl("Remove {label}", { label })}
                      title={tl("Remove {label}", { label })}
                      onClick={() =>
                        onChange(fields.filter((x) => x.key !== f.key))
                      }
                    >
                      <Trash2 size={14} aria-hidden="true" />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      <div className="row">
        <button
          type="button"
          className="btn btn-small"
          onClick={() => {
            onChange([...fields, newDraft()]);
            setAdded((n) => n + 1);
          }}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("Add a field")}
        </button>
      </div>
    </div>
  );
}

/**
 * Adds one field to a termbase that has some already: its name, where it
 * belongs and what it takes. Closed, it is a button.
 */
export function AddFieldForm({
  taken,
  termbase,
  onAdd,
}: {
  /** The names of the fields there are. */
  taken: string[];
  termbase: string;
  onAdd: (f: Omit<FieldDraft, "key">) => void;
}) {
  const [open, setOpen] = useState(false);
  const [f, setF] = useState<FieldDraft>(() => newDraft());
  const ref = useRef<HTMLInputElement>(null);
  const id = useId();
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  if (!open)
    return (
      <div className="row">
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => setOpen(true)}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("Add a field")}
        </button>
      </div>
    );
  const same = taken.find(
    (t) => t.trim().toLocaleLowerCase() === f.name.trim().toLocaleLowerCase(),
  );
  const similar =
    !same && f.name.trim().length >= 3
      ? taken.find((t) =>
          t
            .toLocaleLowerCase()
            .startsWith(f.name.trim().slice(0, 3).toLocaleLowerCase()),
        )
      : undefined;
  const problem = fieldProblem(f, taken);
  return (
    <form
      className="add-field"
      aria-label={tl("Add a field")}
      onSubmit={(e) => {
        e.preventDefault();
        if (problem) return;
        const { key: _k, ...rest } = f;
        onAdd({
          ...rest,
          name: f.name.trim(),
          values: f.kind === "picklist" ? f.values : [],
        });
        setF(newDraft());
        setOpen(false);
      }}
    >
      <label htmlFor={`${id}-n`}>{tl("Field name")}</label>
      <input
        id={`${id}-n`}
        ref={ref}
        value={f.name}
        onChange={(e) => setF({ ...f, name: e.target.value })}
        placeholder={tl("For example Source, Region, Register")}
        aria-describedby={`${id}-h`}
      />
      <p id={`${id}-h`} className="help" aria-live="polite">
        {same
          ? tl("{termbase} already has “{same}”.", { termbase, same })
          : similar
            ? tl("Did you mean the existing field “{similar}”?", { similar })
            : tl("Every entry in this termbase gets the field. Nothing else changes.")}
      </p>
      <fieldset className="scope">
        <legend>{tl("Where does the value belong?")}</legend>
        {FIELD_LEVELS.map((l) => (
          <label key={l.level} className="check">
            <input
              type="radio"
              name={`${id}-level`}
              checked={f.level === l.level}
              onChange={() => setF({ ...f, level: l.level })}
            />
            <span>
              {l.name}: {l.hint}
            </span>
          </label>
        ))}
      </fieldset>
      <label htmlFor={`${id}-k`}>{tl("Values")}</label>
      <KindSelect
        id={`${id}-k`}
        label={tl("Values")}
        value={f.kind}
        onChange={(kind) => setF({ ...f, kind })}
      />
      {f.kind === "picklist" && (
        <ValuesEditor
          label={tl("The values of the list, one a line")}
          values={f.values}
          onChange={(values) => setF({ ...f, values })}
        />
      )}
      <div className="row">
        <button
          type="submit"
          className="btn btn-primary btn-small"
          disabled={!!problem}
        >
          {tl("Add field")}
        </button>
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => {
            setF(newDraft());
            setOpen(false);
          }}
        >
          {tl("Cancel")}
        </button>
      </div>
    </form>
  );
}

const DATE = /^\d{4}-\d{2}-\d{2}/;
const NUMBER = /^[-+]?(\d+([.,]\d*)?|[.,]\d+)$/;

/**
 * The value of a field of an entry, language or term, with the control its
 * kind asks for: a list, yes or no, a date, a number or text. A value that
 * does not fit its kind (from an import, or set before the kind changed)
 * is shown as it is and can be corrected as text.
 */
export function FieldValue({
  it,
  value,
  onSave,
  id,
  long,
}: {
  it: InfoType;
  value: string;
  onSave: (v: string) => void;
  id: string;
  /** Text of the whole entry gets two lines. */
  long?: boolean;
}) {
  const [draft, setDraft] = useState(value);
  const [problem, setProblem] = useState("");
  useEffect(() => {
    setDraft(value);
    setProblem("");
  }, [value]);
  if (it.kind === "picklist" || it.kind === "boolean") {
    const options =
      it.kind === "boolean"
        ? [
            { v: "true", t: tl("Yes") },
            { v: "false", t: tl("No") },
          ]
        : (it.values ?? []).map((v) => ({ v, t: v }));
    return (
      <select
        id={id}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          onSave(e.target.value);
        }}
      >
        <option value="">{tl("Not set")}</option>
        {options.map((o) => (
          <option key={o.v} value={o.v}>
            {o.t}
          </option>
        ))}
        {draft && !options.some((o) => o.v === draft) && (
          <option value={draft}>{draft}</option>
        )}
      </select>
    );
  }
  if (it.kind === "date" && (draft === "" || DATE.test(draft))) {
    return (
      <input
        id={id}
        type="date"
        value={draft.slice(0, 10)}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => draft !== value && onSave(draft)}
      />
    );
  }
  if (it.kind === "number") {
    return (
      <span className="field-number">
        <input
          id={id}
          inputMode="decimal"
          value={draft}
          placeholder={tl("Not set")}
          aria-invalid={!!problem}
          aria-describedby={problem ? `${id}-p` : undefined}
          onChange={(e) => {
            setDraft(e.target.value);
            setProblem("");
          }}
          onBlur={() => {
            const v = draft.trim();
            if (v && !NUMBER.test(v)) {
              setProblem(tl("Type a number, such as 12 or 0.5."));
              return;
            }
            if (v !== value) onSave(v);
          }}
        />
        {problem && (
          <span id={`${id}-p`} className="field-problem">
            {problem}
          </span>
        )}
      </span>
    );
  }
  return long ? (
    <textarea
      id={id}
      value={draft}
      rows={2}
      placeholder={tl("Not set")}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => draft !== value && onSave(draft)}
    />
  ) : (
    <input
      id={id}
      value={draft}
      placeholder={tl("Not set")}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => draft !== value && onSave(draft)}
    />
  );
}
