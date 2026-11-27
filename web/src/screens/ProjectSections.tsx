// The sections of a project's setup, shared by the New project flow and the
// project's settings page, so a choice made at setup is changed later in
// the same place and the same words.

import { createContext, useContext, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Info, Plus, X } from "lucide-react";
import {
  LANGUAGES,
  lang,
  primary,
  type LangCode,
  type MemorySetting,
  type PrepareSettings,
  type Project,
  type QaRule,
  type QaSettings,
  type SegmentationRule,
  type SpellDictionary,
  type Entry,
  type Termbase,
  type TranslationMemory,
} from "../data";
import { useLocalStore, useWords } from "../store";
import { MEMORY_FILES, MEMORY_FILES_NOTE, tm, type CoreApi } from "../core";
import { openMemory, openNewMemory } from "../editor/memory";
import {
  allDictionaries,
  useIncludedDictionaries,
} from "../editor/spelling";
import {
  checkOn,
  problemsAt,
  qaKinds,
  setCheck,
  type Draft,
  type Problem,
} from "../project";
import { LangChip, LocationBadge } from "../components/ui";
import { inSentence, listOf, plural } from "../format";
import { tl, tlx } from "../i18n";

export function Problems({
  messages,
  id,
}: {
  messages: string[];
  id?: string;
}) {
  if (!messages.length) return null;
  return (
    <div id={id} role="alert">
      {messages.map((m) => (
        <p key={m} className="error">
          {m}
        </p>
      ))}
    </div>
  );
}

/**
 * The memories and termbases the sections offer: by default this
 * computer's; a team-server project's settings offer the server's.
 */
export interface SectionsSource {
  memories: TranslationMemory[];
  termbases: Termbase[];
  entries: Entry[];
  /** Units per memory, when the memories are not in the browser. */
  units?: Record<string, number>;
  /** The grammar services a project there can name. */
  grammar?: string[];
}

const SourceCtx = createContext<SectionsSource | null>(null);
export const SectionsSourceProvider = SourceCtx.Provider;

function useSource(): SectionsSource & { projects: Project[]; local: boolean } {
  const { state } = useLocalStore();
  const o = useContext(SourceCtx);
  return o
    ? { ...o, projects: [], local: false }
    : { memories: state.memories, termbases: state.termbases, entries: state.entries, projects: state.projects, local: true };
}

/** Target languages: suggestions as checkboxes, any other from the list. */
export function TargetPicker({
  source,
  targets,
  setTargets,
  suggestions,
  fixed,
  pending,
}: {
  source: LangCode;
  targets: LangCode[];
  setTargets: (t: LangCode[]) => void;
  suggestions: LangCode[];
  /** Languages bilingual files need; they can't be unticked. */
  fixed: LangCode[];
  /**
   * The language picked and not added yet, kept by a new project, which
   * adds it when it moves on to the next step.
   */
  pending?: [LangCode | "", (c: LangCode | "") => void];
}) {
  const own = useState<LangCode | "">("");
  const [adding, setAdding] = pending ?? own;
  const quick = [...new Set([...suggestions, ...targets])].filter(
    (c) => c !== source,
  );
  const others = LANGUAGES.filter(
    (l) => l.code !== source && !quick.includes(l.code),
  );
  return (
    <div className="targets">
      <ul className="target-list">
        {quick.map((c) => (
          <li key={c}>
            <label className="check">
              <input
                type="checkbox"
                checked={targets.includes(c)}
                disabled={fixed.includes(c)}
                onChange={(e) =>
                  setTargets(
                    e.target.checked
                      ? [...targets, c]
                      : targets.filter((x) => x !== c),
                  )
                }
              />
              <span>
                {lang(c).name}
                {fixed.includes(c) && (
                  <span className="muted">{" "}{tl("(a bilingual file is for it)")}</span>
                )}
              </span>
            </label>
          </li>
        ))}
      </ul>
      <div className="add-lang">
        <label htmlFor="add-lang" className="sr-only">
          {tl("Add another target language")}
        </label>
        <select
          id="add-lang"
          value={adding}
          onChange={(e) => setAdding(e.target.value)}
        >
          <option value="">
            {pending && targets.length === 0
              ? tl("Choose a language…")
              : tl("Add another language…")}
          </option>
          {others.map((l) => (
            <option key={l.code} value={l.code}>
              {l.name}
            </option>
          ))}
        </select>
        <button
          type="button"
          className="btn btn-small"
          disabled={!adding}
          onClick={() => {
            setTargets([...targets, adding]);
            setAdding("");
          }}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("Add")}
        </button>
      </div>
      {adding && (
        <p className="help" role="status">
          {pending
            ? tl("{name} is added when you continue, or press Add now.", { name: lang(adding).name })
            : tl("Press Add to add {name} to the project.", { name: lang(adding).name })}
        </p>
      )}
    </div>
  );
}

/** A memory created during setup, stored when the project is created. */
export interface NewMemory {
  memory: TranslationMemory;
  /** Units imported from a TMX file. */
  units: number;
}

function fits(m: TranslationMemory, source: LangCode, target: LangCode) {
  return (
    primary(m.sourceLang) === primary(source) &&
    primary(m.targetLang) === primary(target)
  );
}

/** A memory setting as added: updating it only when there is no project memory to collect translations. */
export function newSetting(
  memory: string,
  projectMemory: boolean,
): MemorySetting {
  return {
    memory,
    lookup: true,
    concordance: true,
    update: !projectMemory,
    penalty: 0,
  };
}

/**
 * Memories per target language: which, in what order, used for what, and
 * with what penalty; new memories can be created here, empty or from TMX.
 */
export function MemoriesSection({
  c,
  draft,
  setDraft,
  problems,
  created,
  setCreated,
  projectId,
}: {
  c: CoreApi | null;
  draft: Draft;
  setDraft: (f: (d: Draft) => Draft) => void;
  problems: Problem[];
  /** Memories created in this setup, not stored yet. */
  created: NewMemory[];
  setCreated: (f: (m: NewMemory[]) => NewMemory[]) => void;
  /** The project being edited, if it exists already. */
  projectId?: string;
}) {
  const state = useSource();
  const all = useMemo(
    () => [
      ...state.memories.filter((m) => !m.projectId),
      ...created.map((n) => n.memory),
    ],
    [state.memories, created],
  );
  const setFor = (l: LangCode, f: (ms: MemorySetting[]) => MemorySetting[]) =>
    setDraft((d) => ({
      ...d,
      memories: { ...d.memories, [l]: f(d.memories[l] ?? []) },
    }));

  return (
    <>
      <div className="field">
        <label className="check">
          <input
            type="checkbox"
            checked={draft.projectMemory}
            onChange={(e) =>
              setDraft((d) => ({ ...d, projectMemory: e.target.checked }))
            }
          />
          <span>{tl("Give the project its own memory for each language")}</span>
        </label>
        <p className="help">
          {draft.projectMemory
            ? tl("Confirmed translations go into the project memory first. When the project is done, you add them to a main memory from the project's settings. The project memory is searched before the memories below.")
            : tl("Confirmed translations go only into the memories below that you update.")}
          {projectId &&
            !draft.projectMemory &&
            state.memories.some((m) => m.projectId === projectId) &&
            tl(" The project memories you have now are deleted when you save.")}
        </p>
      </div>
      {draft.targetLangs.map((l) => {
        const list = draft.memories[l] ?? [];
        return (
          <fieldset key={l} className="field memory-set">
            <legend>
              {tlx("{name} to {name2}", { name: lang(draft.sourceLang).name, name2: lang(l).name })}</legend>
            {list.length === 0 ? (
              <p className="muted">
                {draft.projectMemory
                  ? tl("No memory chosen. Matches come only from the project memory.")
                  : tl("No memory chosen. The editor will show no matches.")}</p>
            ) : (
              <div className="table-wrap">
                <table className="table memory-table">
                  <caption className="sr-only">
                    {tlx("Memories for {name}, highest priority first", { name: lang(l).name })}
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">{tl("Memory")}</th>
                      <th scope="col">{tl("Lookup")}</th>
                      <th scope="col">{tl("Concordance")}</th>
                      <th scope="col">{tl("Update")}</th>
                      <th scope="col">{tl("Penalty")}</th>
                      <th scope="col">
                        <span className="sr-only">{tl("Order and removal")}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {list.map((m, i) => {
                      const info = all.find((x) => x.name === m.memory);
                      const id = `mem-${l}-${i}`;
                      const set = (patch: Partial<MemorySetting>) =>
                        setFor(l, (ms) =>
                          ms.map((x, j) => (j === i ? { ...x, ...patch } : x)),
                        );
                      return (
                        <tr key={m.memory}>
                          <th scope="row" id={id}>
                            <span className="cell-title">{m.memory}</span>
                            <span className="cell-sub">
                              {info ? (
                                <>
                                  <LangChip code={info.sourceLang} /> →{" "}
                                  <LangChip code={info.targetLang} />
                                  {state.units
                                    ? ` ${plural(state.units[info.name] ?? 0, "unit", "units")}`
                                    : c &&
                                      ` ${plural(memoryCount(c, info, state.projects), "unit", "units")}`}
                                </>
                              ) : (
                                tl("This memory no longer exists.")
                              )}
                            </span>
                          </th>
                          {(["lookup", "concordance", "update"] as const).map(
                            (k) => (
                              <td key={k}>
                                <input
                                  type="checkbox"
                                  checked={m[k]}
                                  aria-label={`${k === "lookup" ? tl("Look up matches in") : k === "concordance" ? tl("Search with concordance in") : tl("Store confirmed translations in")} ${m.memory}`}
                                  onChange={(e) =>
                                    set({ [k]: e.target.checked })
                                  }
                                />
                              </td>
                            ),
                          )}
                          <td>
                            <input
                              type="number"
                              className="penalty"
                              min={0}
                              max={100}
                              value={m.penalty}
                              aria-label={tl("Penalty for {memory}, in points", { memory: m.memory })}
                              onChange={(e) =>
                                set({
                                  penalty:
                                    e.target.value === ""
                                      ? 0
                                      : Math.round(Number(e.target.value)),
                                })
                              }
                            />
                          </td>
                          <td className="row">
                            <button
                              type="button"
                              className="icon-button"
                              disabled={i === 0}
                              aria-label={tl("Move {memory} up", { memory: m.memory })}
                              onClick={() =>
                                setFor(l, (ms) => swap(ms, i, i - 1))
                              }
                            >
                              <ArrowUp size={14} aria-hidden="true" />
                            </button>
                            <button
                              type="button"
                              className="icon-button"
                              disabled={i === list.length - 1}
                              aria-label={tl("Move {memory} down", { memory: m.memory })}
                              onClick={() =>
                                setFor(l, (ms) => swap(ms, i, i + 1))
                              }
                            >
                              <ArrowDown size={14} aria-hidden="true" />
                            </button>
                            <button
                              type="button"
                              className="icon-button"
                              aria-label={tl("Remove {memory}", { memory: m.memory })}
                              onClick={() =>
                                setFor(l, (ms) => ms.filter((_, j) => j !== i))
                              }
                            >
                              <X size={14} aria-hidden="true" />
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            <AddMemory
              c={c}
              target={l}
              draft={draft}
              available={all.filter(
                (m) =>
                  fits(m, draft.sourceLang, l) &&
                  !list.some((x) => x.memory === m.name),
              )}
              taken={[
                ...state.memories.map((m) => m.name),
                ...created.map((n) => n.memory.name),
              ]}
              onAdd={(name) =>
                setFor(l, (ms) => [
                  ...ms,
                  newSetting(name, draft.projectMemory),
                ])
              }
              onCreate={(n) => {
                setCreated((xs) => [...xs, n]);
                setFor(l, (ms) => [
                  ...ms,
                  newSetting(n.memory.name, draft.projectMemory),
                ]);
              }}
            />
            <Problems messages={problemsAt(problems, `memories.${l}`)} />
          </fieldset>
        );
      })}
      <p className="help">
        {tl("A penalty takes points off a memory's matches, for memories you trust less: with 2 points, an exact match shows as 98% and is not inserted as an exact match.")}
      </p>
    </>
  );
}

/** Units in a memory, opening it in the core if it isn't yet. */
function memoryCount(
  c: CoreApi,
  m: TranslationMemory,
  projects: Project[],
): number {
  openMemory(c, m, projects);
  return tm.count(c, m.name);
}

function swap<T>(xs: T[], i: number, j: number): T[] {
  const out = [...xs];
  [out[i], out[j]] = [out[j]!, out[i]!];
  return out;
}

function AddMemory({
  c,
  target,
  draft,
  available,
  taken,
  onAdd,
  onCreate,
}: {
  c: CoreApi | null;
  target: LangCode;
  draft: Draft;
  available: TranslationMemory[];
  taken: string[];
  onAdd: (name: string) => void;
  onCreate: (m: NewMemory) => void;
}) {
  const [pick, setPick] = useState("");
  const [creating, setCreating] = useState(false);
  const pair = `${draft.sourceLang.toUpperCase()}-${target.toUpperCase()}`;
  const suggested = `${draft.client.trim() || draft.name.trim() || tl("Memory")} (${pair})`;
  const [name, setName] = useState(suggested);
  const [tmx, setTmx] = useState<File | null>(null);
  const [error, setError] = useState("");
  const id = `add-mem-${target}`;
  const clash = taken.some(
    (t) => t.toLowerCase() === name.trim().toLowerCase(),
  );
  const nameError = !name.trim()
    ? tl("Give the memory a name.")
    : clash
      ? tl("A memory with this name exists already.")
      : "";
  return (
    <div className="memory-add">
      <div className="add-lang">
        <label htmlFor={id} className="sr-only">
          {tlx("Add a memory for {name}", { name: lang(target).name })}</label>
        <select id={id} value={pick} onChange={(e) => setPick(e.target.value)}>
          <option value="">
            {available.length ? tl("Add a memory…") : tl("No other memory fits")}
          </option>
          {available.map((m) => (
            <option key={m.name} value={m.name}>
              {m.name}
              {m.client ? ` (${m.client})` : ""}
            </option>
          ))}
        </select>
        <button
          type="button"
          className="btn btn-small"
          disabled={!pick}
          onClick={() => {
            onAdd(pick);
            setPick("");
          }}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("Add")}
        </button>
        {!creating && (
          <button
            type="button"
            className="btn btn-quiet btn-small"
            onClick={() => {
              setName(suggested);
              setCreating(true);
            }}
          >
            {tl("New memory")}
          </button>
        )}
      </div>
      {creating && (
        <div
          className="memory-new"
          role="group"
          aria-label={tl("New memory for {name}", { name: lang(target).name })}
        >
          <div className="field">
            <label htmlFor={`${id}-name`}>{tl("Name of the new memory")}</label>
            <input
              id={`${id}-name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              aria-invalid={!!nameError}
              aria-describedby={nameError ? `${id}-name-err` : undefined}
            />
            {nameError && (
              <p id={`${id}-name-err`} className="error">
                {nameError}
              </p>
            )}
          </div>
          <div className="field">
            <label htmlFor={`${id}-tmx`}>
              {tlx("Fill it from a memory file {span}", { span: <span className="optional">{tl("optional")}</span> })}</label>
            <input
              id={`${id}-tmx`}
              type="file"
              accept={MEMORY_FILES}
              aria-describedby={`${id}-tmx-help`}
              onChange={(e) => setTmx(e.target.files?.[0] ?? null)}
            />
            <p id={`${id}-tmx-help`} className="help">
              {MEMORY_FILES_NOTE}
            </p>
          </div>
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
          <div className="row">
            <button
              type="button"
              className="btn btn-small btn-primary"
              disabled={!!nameError || !c}
              onClick={async () => {
                if (!c) return;
                const memory: TranslationMemory = {
                  name: name.trim(),
                  sourceLang: draft.sourceLang,
                  targetLang: target,
                  client: draft.client.trim() || undefined,
                  createdAt: new Date().toISOString(),
                };
                let units = 0;
                try {
                  units = openNewMemory(
                    c,
                    memory,
                    tmx ? new Uint8Array(await tmx.arrayBuffer()) : undefined,
                  );
                } catch (e) {
                  setError(
                    (tmx ? tl("Could not read {name}: {error}", { name: tmx.name, error: e instanceof Error ? e.message : String(e) }) : tl("Could not read the file: {error}", { error: e instanceof Error ? e.message : String(e) })),
                  );
                  return;
                }
                onCreate({ memory, units });
                setCreating(false);
                setTmx(null);
                setError("");
              }}
            >
              {tl("Create memory")}
            </button>
            <button
              type="button"
              className="btn btn-quiet btn-small"
              onClick={() => {
                setCreating(false);
                setError("");
              }}
            >
              {tl("Cancel")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * Where new terms go and which masters the project uses. On an existing
 * project the termbase setup itself stays; masters and updating can change.
 */
export function TerminologySection({
  draft,
  setDraft,
  problems,
  onTouched,
  existing,
}: {
  draft: Draft;
  setDraft: (f: (d: Draft) => Draft) => void;
  problems: Problem[];
  /** Called when the user changes the masters. */
  onTouched?: (what: "masters") => void;
  /** The project exists: its project termbase is in place. */
  existing?: boolean;
}) {
  const state = useSource();
  const w = useWords();
  const t = draft.terminology;
  const masters = state.termbases.filter((x) => x.kind === "master");
  const langs = [draft.sourceLang, ...draft.targetLangs];
  const set = (patch: Partial<Draft["terminology"]>) =>
    setDraft((d) => ({ ...d, terminology: { ...d.terminology, ...patch } }));

  return (
    <>
      <fieldset className="field">
        <legend>
          {t.mode === "direct" ? tl("Master termbases") : tl("Master termbases to use")}
        </legend>
        <p className="help">
          {tl("Masters you keep across projects, for a client, a domain or a set of languages.")}
        </p>
        {masters.length === 0 && (
          <p className="muted">
            {tl("There is no master termbase yet. The project gets a termbase of its own.")}
          </p>
        )}
        <ul className="choice-list">
          {masters.map((m) => {
            const chosen = t.masters.includes(m.id);
            const missing = langs.filter((l) => !m.languages.includes(l));
            const suggested = !!draft.client && m.client === draft.client;
            const count =
              m.entryCount ??
              state.entries.filter((e) => e.termbaseId === m.id).length;
            return (
              <li key={m.id}>
                <label className="choice">
                  <input
                    type="checkbox"
                    checked={chosen}
                    onChange={(e) => {
                      onTouched?.("masters");
                      const next = e.target.checked
                        ? [...t.masters, m.id]
                        : t.masters.filter((x) => x !== m.id);
                      set({
                        // Choosing a master for a project with only its own
                        // termbase layers the project over it; taking the
                        // last one away leaves only its own.
                        mode: next.length === 0 ? "own" : t.mode === "own" ? "layered" : t.mode,
                        masters: next,
                        addLanguages: t.addLanguages.filter((x) =>
                          next.includes(x),
                        ),
                        writeTo: next.includes(t.writeTo ?? "")
                          ? t.writeTo
                          : next[0],
                      });
                    }}
                  />
                  <span className="choice-body">
                    <span className="choice-title">
                      {m.name}
                      {suggested && (
                        <span className="tag tag-accent">
                          {tlx("Suggested for {client}", { client: m.client })}</span>
                      )}
                    </span>
                    <span className="choice-meta">
                      <LocationBadge location={m.location} server={m.server} />
                      <span>{plural(count, "entry", "entries")}</span>
                      <span className="lang-list">
                        {m.languages.map((l) => (
                          <LangChip key={l} code={l} />
                        ))}
                      </span>
                    </span>
                  </span>
                </label>
                {chosen && missing.length > 0 && (
                  <div className="choice-note choice-note-wrap master-langs">
                    <Info size={14} aria-hidden="true" />
                    <span>
                      {missing.length === 1
                        ? tl("{name} has no {language}.", { name: m.name, language: lang(missing[0]!).name })
                        : tl("{name} does not have the languages {languages}.", { name: m.name, languages: listOf(missing.map((c) => lang(c).name)) })}{" "}<label className="check check-inline">
                        <input
                          type="checkbox"
                          checked={t.addLanguages.includes(m.id)}
                          onChange={(e) =>
                            set({
                              addLanguages: e.target.checked
                                ? [...t.addLanguages, m.id]
                                : t.addLanguages.filter((x) => x !== m.id),
                            })
                          }
                        />
                        <span>
                          {(missing.length === 1 ? tl("Add it to {name}", { name: m.name }) : tl("Add them to {name}", { name: m.name }))}</span>
                      </label>
                      {!t.addLanguages.includes(m.id) && " "}
                      {!t.addLanguages.includes(m.id) &&
                        (t.mode === "direct" && t.writeTo === m.id
                          ? tl("New terms go into this master, so it needs the languages.")
                          : tl("Otherwise {name} is used for the languages it has, and terms in the others stay in the project termbase.", { name: m.name }))}
                    </span>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
        <Problems messages={problemsAt(problems, "terminology.masters")} />
        <Problems messages={problemsAt(problems, "terminology.addLanguages")} />
      </fieldset>

      {existing ? (
        <div className="field">
          <span className="field-label">{tl("Where new terms go")}</span>
          <p>
            {t.mode === "direct"
              ? tl("Straight into a master.")
              : t.mode === "own"
                ? tl("Into the project's own termbase, without masters.")
                : tl("Into the {termbase}, then merged into the masters.", { termbase: inSentence(w("projectTermbase")) })}{" "}
            <span className="muted">
              {tl("This stays as the project was set up, so the terms already collected stay where they are.")}
            </span>
          </p>
        </div>
      ) : (
        <fieldset className="field">
          <legend>{tl("Where new terms go")}</legend>
          <ul className="choice-list">
            {(
              [
                [
                  "layered",
                  <>
                    {w("projectTermbaseThenMerge")}{" "}<span className="tag">{tl("Recommended")}</span></>,
                  tl("New and changed terms stay in the project. When the project is done, you review them and merge into the masters. The masters stay unchanged until then."),
                ],
                [
                  "direct",
                  tl("Directly into a master"),
                  tl("New terms are saved to the master right away. Simple when you work alone on this client."),
                ],
                [
                  "own",
                  tl("Own project termbase only"),
                  tl("No master is used. You can turn it into a master or merge it into one later."),
                ],
              ] as const
            ).map(([mode, title, desc]) => {
              const needsMaster = mode !== "own";
              const off = needsMaster && t.masters.length === 0;
              return (
                <li key={mode}>
                  <label className={`choice${off ? " is-disabled" : ""}`}>
                    <input
                      type="radio"
                      name="mode"
                      value={mode}
                      checked={t.mode === mode}
                      disabled={off}
                      onChange={() => {
                        onTouched?.("masters");
                        set({
                          mode,
                          masters: mode === "own" ? [] : t.masters,
                          writeTo:
                            mode === "direct"
                              ? (t.writeTo ?? t.masters[0])
                              : undefined,
                        });
                      }}
                    />
                    <span className="choice-body">
                      <span className="choice-title">{title}</span>
                      <span className="choice-desc">{desc}</span>
                    </span>
                  </label>
                </li>
              );
            })}
          </ul>
          <Problems messages={problemsAt(problems, "terminology.mode")} />
        </fieldset>
      )}

      {t.mode === "direct" && t.masters.length > 1 && (
        <div className="field">
          <label htmlFor="direct-target">{tl("Master for new terms")}</label>
          <select
            id="direct-target"
            value={t.writeTo ?? ""}
            onChange={(e) => set({ writeTo: e.target.value })}
          >
            {t.masters.map((id) => (
              <option key={id} value={id}>
                {state.termbases.find((x) => x.id === id)?.name ?? id}
              </option>
            ))}
          </select>
        </div>
      )}
      <Problems messages={problemsAt(problems, "terminology.writeTo")} />


      {t.mode === "layered" && (
        <div className="field">
          <label className="check">
            <input
              type="checkbox"
              checked={t.autoUpdate}
              onChange={(e) => set({ autoUpdate: e.target.checked })}
            />
            <span>{w("autoUpdate")}</span>
          </label>
          <p className="help">
            {t.autoUpdate
              ? tl("Changes made to the masters since the project last opened are taken in when the editor opens.")
              : tl("Changes made to the masters are listed under {w} until you take them in yourself.", { w: w("reviewMerge") })}
          </p>
        </div>
      )}
    </>
  );
}

/** Which grammar service reads the project's translations, if any. */
function GrammarField({ qa, setQa, problems }: { qa: QaSettings; setQa: (q: QaSettings) => void; problems: string[] }) {
  const { state } = useLocalStore();
  const o = useContext(SourceCtx);
  const names = o ? (o.grammar ?? []) : state.providers.filter((p) => p.kind === "check").map((p) => p.name);
  const set = qa.grammar ?? "";
  const gone = !!set && !names.some((n) => n.toLowerCase() === set.toLowerCase());
  return (
    <fieldset className="field">
      <legend>{tl("Grammar")}</legend>
      <div className="field-row-inline">
        <label htmlFor="qa-grammar">{tl("Grammar service")}</label>
        <select
          id="qa-grammar"
          className="select-small"
          value={set}
          onChange={(e) => {
            const { grammar: _was, ...rest } = qa;
            setQa(e.target.value ? { ...rest, grammar: e.target.value } : rest);
          }}
        >
          <option value="">{names.length || gone ? tl("No grammar check") : tl("No grammar service yet")}</option>
          {gone && <option value={set}>{set}{" "}{tl("(no longer there)")}</option>}
          {names.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
      </div>
      <Problems messages={problems} />
      <p className="help">
        {o
          ? tlx("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under {settings}, by an administrator of the server.", { settings: <a href="#/settings">{tl("Settings")}</a> })
          : tlx("With a service, each translation is sent to it while you work on the segment, and what it finds is listed with the checks. Services are added under {settings}.", { settings: <a href="#/settings">{tl("Settings")}</a> })}
      </p>
    </fieldset>
  );
}

/** How many of the checks are on. */
export function checksOn(
  c: CoreApi,
  qa: QaSettings,
): { on: number; of: number } {
  const kinds = qaKinds(c);
  return {
    on: kinds.filter((k) => checkOn(qa, k)).length + (qa.uneditedFuzzy ? 1 : 0),
    of: kinds.length + 1,
  };
}

/** Dictionaries that spell a language: the same region first, then the language. */
export function dictionariesFor(
  all: SpellDictionary[],
  l: LangCode,
): SpellDictionary[] {
  return all
    .filter((d) => primary(d.lang) === primary(l))
    .sort((a, b) => Number(b.lang === l) - Number(a.lang === l));
}

/** Each target language gets the dictionary that fits it best, where it has none. */
export function withDictionaries(
  qa: QaSettings,
  targets: LangCode[],
  all: SpellDictionary[],
): QaSettings {
  const dictionaries: Record<LangCode, string> = {};
  for (const t of targets) {
    const set = qa.dictionaries[t];
    const best = dictionariesFor(all, t)[0]?.name;
    if (set && all.some((d) => d.name === set)) dictionaries[t] = set;
    else if (best) dictionaries[t] = best;
  }
  return JSON.stringify(dictionaries) === JSON.stringify(qa.dictionaries)
    ? qa
    : { ...qa, dictionaries };
}

const NEW_RULE: QaRule = {
  name: "",
  target: "",
  mode: "must-not",
  severity: "warning",
};

/**
 * Quality checks the editor runs: the core's checks, the spelling
 * dictionary for each target language, the project's own words and
 * rules, and how context matches are counted.
 */
export function ChecksSection({
  c,
  qa,
  setQa,
  context,
  setContext,
  targetLangs,
  problems,
}: {
  c: CoreApi | null;
  qa: QaSettings;
  setQa: (q: QaSettings) => void;
  context: Draft["context"];
  setContext: (r: Draft["context"]) => void;
  targetLangs: LangCode[];
  problems: Problem[];
}) {
  const { state } = useLocalStore();
  const included = useIncludedDictionaries();
  const includedLangs = new Set(included.map((d) => primary(d.lang))).size;
  const dictionaries = useMemo(
    () => allDictionaries(state.dictionaries, included),
    [state.dictionaries, included],
  );
  const kinds = c ? qaKinds(c) : [];
  const [word, setWord] = useState("");
  const spellingOn = kinds.some((k) => k.kind === "spelling" && checkOn(qa, k));
  const addWord = () => {
    const w = word.trim();
    if (!w || qa.words.includes(w)) return;
    setQa({ ...qa, words: [...qa.words, w] });
    setWord("");
  };
  const setRule = (i: number, patch: Partial<QaRule>) =>
    setQa({
      ...qa,
      rules: qa.rules.map((r, j) => (j === i ? { ...r, ...patch } : r)),
    });
  return (
    <>
      <fieldset className="field">
        <legend>{tl("Context matches")}</legend>
        <label className="choice">
          <input
            type="radio"
            name="ctx"
            checked={context === "previous"}
            onChange={() => setContext("previous")}
          />
          <span className="choice-body">
            <span className="choice-title">{tl("The segment before matches")}</span>
            <span className="choice-desc">
              {tl("The usual rule for a context match.")}
            </span>
          </span>
        </label>
        <label className="choice">
          <input
            type="radio"
            name="ctx"
            checked={context === "both"}
            onChange={() => setContext("both")}
          />
          <span className="choice-body">
            <span className="choice-title">
              {tl("The segments before and after match")}
            </span>
            <span className="choice-desc">{tl("Stricter: both neighbours must match.")}</span>
          </span>
        </label>
      </fieldset>
      <fieldset className="field">
        <legend>{tl("Quality checks while translating")}</legend>
        {!c ? (
          <p className="muted">{tl("Loading the checks…")}</p>
        ) : (
          <ul className="check-list">
            {kinds.map((k) => (
              <li key={k.kind}>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={checkOn(qa, k)}
                    onChange={(e) => setQa(setCheck(qa, k, e.target.checked))}
                  />
                  <span>
                    {k.name}
                    <span className="check-desc">
                      {k.description}
                      {k.kind === "length" &&
                        tl(" Limits come from the file (XLIFF maxwidth); documents have none.")}
                      {k.kind === "spelling" && tl(" With the dictionaries below.")}
                      {k.kind === "rule" && tl(" The rules below.")}
                    </span>
                  </span>
                </label>
              </li>
            ))}
            <li>
              <label className="check">
                <input
                  type="checkbox"
                  checked={qa.uneditedFuzzy}
                  onChange={(e) =>
                    setQa({ ...qa, uneditedFuzzy: e.target.checked })
                  }
                />
                <span>
                  {tlx("Unedited fuzzy matches{span}", { span: <span className="check-desc">
                    {tl("A fuzzy match is confirmed without a change.")}
                  </span> })}</span>
              </label>
            </li>
          </ul>
        )}
      </fieldset>

      <fieldset className="field">
        <legend>{tl("Subtitles")}</legend>
        <p className="help">
          {tl("Limits for subtitle and caption files, checked with their times: the longest line, the most lines, and how fast the subtitle must be read. Closed captions (SCC) also hold at most 32 characters a line.")}
        </p>
        {(
          [
            ["sub-line-length", "lineLength", tl("Characters in a line"), 1, 200, 1],
            ["sub-lines", "lines", tl("Lines in a subtitle"), 1, 10, 1],
            ["sub-cps", "cps", tl("Characters per second"), 1, 100, 0.5],
          ] as const
        ).map(([id, key, label, min, max, step]) => (
          <div key={id} className="inline-number subtitle-limit">
            <label htmlFor={id}>{label}</label>
            <input
              id={id}
              type="number"
              className="penalty"
              min={min}
              max={max}
              step={step}
              value={qa.subtitles[key]}
              onChange={(e) =>
                setQa({
                  ...qa,
                  subtitles: {
                    ...qa.subtitles,
                    [key]:
                      e.target.value === ""
                        ? 0
                        : key === "cps"
                          ? Number(e.target.value)
                          : Math.round(Number(e.target.value)),
                  },
                })
              }
            />
            <Problems messages={problemsAt(problems, `qa.subtitles.${key}`)} />
          </div>
        ))}
      </fieldset>

      <fieldset className="field">
        <legend>{tl("Spelling")}</legend>
        {!spellingOn && <p className="muted">{tl("The spelling check is off.")}</p>}
        <ul className="dict-list">
          {targetLangs.map((t) => {
            const fit = dictionariesFor(dictionaries, t);
            const id = `dict-${t}`;
            return (
              <li key={t} className="field-row-inline">
                <label htmlFor={id}>{lang(t).name}</label>
                <select
                  id={id}
                  className="select-small"
                  value={qa.dictionaries[t] ?? ""}
                  disabled={!spellingOn}
                  onChange={(e) => {
                    const dictionaries = { ...qa.dictionaries };
                    if (e.target.value) dictionaries[t] = e.target.value;
                    else delete dictionaries[t];
                    setQa({ ...qa, dictionaries });
                  }}
                >
                  <option value="">
                    {fit.length
                      ? tl("No spelling check")
                      : tl("No dictionary for this language")}
                  </option>
                  {fit.map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({lang(d.lang).name})
                    </option>
                  ))}
                </select>
                <Problems
                  messages={problemsAt(problems, `qa.dictionaries.${t}`)}
                />
              </li>
            );
          })}
        </ul>
        <p className="help">
          {includedLangs === 1
            ? tlx("A dictionary for one language comes with Interlinea. Others are added once under {settings}, from a LibreOffice extension (.oxt) or .aff and .dic files.", { settings: <a href="#/settings">{tl("Settings")}</a> })
            : includedLangs > 1
              ? tlx("Dictionaries for {n} languages come with Interlinea. Others are added once under {settings}, from a LibreOffice extension (.oxt) or .aff and .dic files.", { n: includedLangs, settings: <a href="#/settings">{tl("Settings")}</a> })
              : tlx("Dictionaries are added once under {settings}, from a LibreOffice extension (.oxt) or .aff and .dic files.", { settings: <a href="#/settings">{tl("Settings")}</a> })}
        </p>
        <div className="field">
          <span className="field-label" id="words-label">
            {tl("Words always spelled right in this project")}
          </span>
          {qa.words.length > 0 && (
            <ul className="word-list" aria-labelledby="words-label">
              {qa.words.map((w) => (
                <li key={w} className="tag">
                  {w}
                  <button
                    type="button"
                    className="icon-button"
                    aria-label={tl("Remove {w}", { w })}
                    onClick={() =>
                      setQa({ ...qa, words: qa.words.filter((x) => x !== w) })
                    }
                  >
                    <X size={12} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div className="add-lang">
            <label htmlFor="add-word" className="sr-only">
              {tl("Add a word")}
            </label>
            <input
              id="add-word"
              value={word}
              onChange={(e) => setWord(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addWord();
                }
              }}
              placeholder={tl("A name or term")}
            />
            <button
              type="button"
              className="btn btn-small"
              disabled={!word.trim()}
              onClick={addWord}
            >
              <Plus size={14} aria-hidden="true" />
              {tl("Add")}
            </button>
          </div>
          <Problems messages={problemsAt(problems, "qa.words")} />
        </div>
      </fieldset>

      <GrammarField qa={qa} setQa={setQa} problems={problemsAt(problems, "qa.grammar")} />

      <fieldset className="field">
        <legend>{tl("Rules of your own")}</legend>
        <p className="help">
          {tl("Regular expressions the translation must or must not match, only where the source matches one if you give it, or must match as often as the source matches.")}
        </p>
        {qa.rules.map((r, i) => {
          const id = `rule-${i}`;
          return (
            <div
              key={i}
              className="rule"
              role="group"
              aria-label={r.name || tl("Rule {n}", { n: i + 1 })}
            >
              <div className="field-grid">
                <div className="field">
                  <label htmlFor={`${id}-name`}>{tl("Name")}</label>
                  <input
                    id={`${id}-name`}
                    value={r.name}
                    onChange={(e) => setRule(i, { name: e.target.value })}
                  />
                </div>
                <div className="field">
                  <label htmlFor={`${id}-mode`}>{tl("The translation")}</label>
                  <select
                    id={`${id}-mode`}
                    value={r.mode}
                    onChange={(e) =>
                      setRule(i, { mode: e.target.value as QaRule["mode"] })
                    }
                  >
                    <option value="must-not">{tl("must not match")}</option>
                    <option value="must">{tl("must match")}</option>
                    <option value="count">
                      {tl("must match as often as the source")}
                    </option>
                  </select>
                </div>
                <div className="field">
                  <label htmlFor={`${id}-target`}>{tl("Translation pattern")}</label>
                  <input
                    id={`${id}-target`}
                    className="mono"
                    value={r.target}
                    onChange={(e) => setRule(i, { target: e.target.value })}
                  />
                </div>
                <div className="field">
                  <label htmlFor={`${id}-source`}>
                    {tlx("Source pattern {span}", { span: <span className="optional">
                      {r.mode === "count" ? tl("needed") : tl("optional")}
                    </span> })}</label>
                  <input
                    id={`${id}-source`}
                    className="mono"
                    value={r.source ?? ""}
                    onChange={(e) =>
                      setRule(i, { source: e.target.value || undefined })
                    }
                  />
                </div>
                <div className="field">
                  <label htmlFor={`${id}-sev`}>{tl("Shown as")}</label>
                  <select
                    id={`${id}-sev`}
                    value={r.severity ?? "warning"}
                    onChange={(e) =>
                      setRule(i, {
                        severity: e.target.value as "error" | "warning",
                      })
                    }
                  >
                    <option value="warning">{tl("Warning")}</option>
                    <option value="error">{tl("Error")}</option>
                  </select>
                </div>
                <div className="field">
                  <label htmlFor={`${id}-msg`}>
                    {tlx("Message {span}", { span: <span className="optional">{tl("optional")}</span> })}</label>
                  <input
                    id={`${id}-msg`}
                    value={r.message ?? ""}
                    onChange={(e) =>
                      setRule(i, { message: e.target.value || undefined })
                    }
                  />
                </div>
              </div>
              <Problems messages={problemsAt(problems, `qa.rules[${i}]`)} />
              <button
                type="button"
                className="btn btn-quiet btn-small"
                onClick={() =>
                  setQa({ ...qa, rules: qa.rules.filter((_, j) => j !== i) })
                }
              >
                <X size={14} aria-hidden="true" />
                {tl("Remove rule")}
              </button>
            </div>
          );
        })}
        <div>
          <button
            type="button"
            className="btn btn-small"
            onClick={() =>
              setQa({ ...qa, rules: [...qa.rules, { ...NEW_RULE }] })
            }
          >
            <Plus size={14} aria-hidden="true" />
            {tl("Add a rule")}
          </button>
        </div>
      </fieldset>
    </>
  );
}

/** The project's own rules for where sentences end. */
export function SegmentationSection({
  rules,
  setRules,
  problems,
  existing,
}: {
  rules: SegmentationRule[];
  setRules: (rules: SegmentationRule[]) => void;
  problems: Problem[];
  /** The project exists: its files are cut already. */
  existing?: boolean;
}) {
  const setRule = (i: number, patch: Partial<SegmentationRule>) => setRules(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <fieldset className="field">
      <legend>{tl("Segmentation rules of your own")}</legend>
      <p className="help">
        {tlx("Sentences end as is usual for the source language. A rule changes that at every place where the text before it ends with the first pattern and the text after it begins with the second (regular expressions; one may stay empty). Your rules come first. {cut} Files that bring their own segments keep them.", { cut: existing ? tl("They cut the documents added from now on; the files in the project stay as they are.") : tl("They cut the documents of the project.") })}
      </p>
      {rules.map((r, i) => {
        const id = `seg-rule-${i}`;
        return (
          <div key={i} className="rule" role="group" aria-label={tl("Segmentation rule {n}", { n: i + 1 })}>
            <div className="field-grid">
              <div className="field">
                <label htmlFor={`${id}-break`}>{tl("At this place")}</label>
                <select id={`${id}-break`} value={r.break ? "break" : "keep"} onChange={(e) => setRule(i, { break: e.target.value === "break" })}>
                  <option value="keep">{tl("a sentence does not end")}</option>
                  <option value="break">{tl("a sentence ends")}</option>
                </select>
              </div>
              <div className="field">
                <label htmlFor={`${id}-before`}>{tl("Text before the place")}</label>
                <input id={`${id}-before`} className="mono" value={r.before ?? ""} onChange={(e) => setRule(i, { before: e.target.value || undefined })} />
              </div>
              <div className="field">
                <label htmlFor={`${id}-after`}>{tl("Text after the place")}</label>
                <input id={`${id}-after`} className="mono" value={r.after ?? ""} onChange={(e) => setRule(i, { after: e.target.value || undefined })} />
              </div>
            </div>
            <Problems messages={problemsAt(problems, `segmentation[${i}]`)} />
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setRules(rules.filter((_, j) => j !== i))}>
              <X size={14} aria-hidden="true" />
              {tl("Remove rule")}
            </button>
          </div>
        );
      })}
      <Problems messages={problems.filter((p) => p.field === "segmentation").map((p) => p.message)} />
      <button type="button" className="btn btn-small" onClick={() => setRules([...rules, { break: false, before: "" }])}>
        <Plus size={14} aria-hidden="true" />
        {tl("Add a segmentation rule")}
      </button>
      <p className="help">
        {tlx("An abbreviation that should not end a sentence: “does not end”, before {code}, after {code2}. A new segment after every semicolon: “ends”, before {code3}, after {code4}.", { code: <code>\bca\.</code>, code2: <code>\s</code>, code3: <code>;</code>, code4: <code>\s</code> })}
      </p>
    </fieldset>
  );
}

/** Pre-translation: which matches go into empty segments, and which are confirmed. */
export function PrepareSection({
  prepare,
  setPrepare,
  problems,
  summary,
  existing,
}: {
  prepare: PrepareSettings;
  setPrepare: (p: PrepareSettings) => void;
  problems: Problem[];
  /** What pre-translation would do with these settings. */
  summary?: string;
  /**
   * The project exists: it is pre-translated on demand, so only the
   * matches to insert are set here.
   */
  existing?: boolean;
}) {
  return (
    <fieldset className="field">
      <legend>{existing ? tl("Matches to insert") : tl("Pre-translation")}</legend>
      {!existing && (
        <label className="check">
          <input
            type="checkbox"
            checked={prepare.pretranslate}
            onChange={(e) =>
              setPrepare({ ...prepare, pretranslate: e.target.checked })
            }
          />
          <span>{tl("Fill empty segments with memory matches")}</span>
        </label>
      )}
      {(prepare.pretranslate || existing) && (
        <div className={existing ? undefined : "nested"}>
          <div className="inline-number">
            <label htmlFor="pt-min">{tl("Lowest match to insert")}</label>
            <input
              id="pt-min"
              type="number"
              className="penalty"
              min={50}
              max={100}
              value={prepare.minScore}
              onChange={(e) =>
                setPrepare({
                  ...prepare,
                  minScore:
                    e.target.value === ""
                      ? 0
                      : Math.round(Number(e.target.value)),
                })
              }
            />
            <span>%</span>
          </div>
          <Problems messages={problemsAt(problems, "prepare.minScore")} />
          <label className="check">
            <input
              type="checkbox"
              checked={prepare.confirmContext}
              onChange={(e) =>
                setPrepare({ ...prepare, confirmContext: e.target.checked })
              }
            />
            <span>{tl("Confirm segments filled with a context match")}</span>
          </label>
          <label className="check">
            <input
              type="checkbox"
              checked={prepare.confirmExact}
              onChange={(e) =>
                setPrepare({ ...prepare, confirmExact: e.target.checked })
              }
            />
            <span>{tl("Confirm segments filled with a 100% match")}</span>
          </label>
          <p className="help">
            {tl("Other matches are inserted as drafts for you to check. Locked and translated segments are left alone.")}
          </p>
          {summary && (
            <p className="detect detect-info" role="status">
              <Info size={16} aria-hidden="true" />
              <span>{summary}</span>
            </p>
          )}
        </div>
      )}
    </fieldset>
  );
}
