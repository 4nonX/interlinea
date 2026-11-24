import { useEffect, useRef, useState } from "react";
import { AlertTriangle, Upload } from "lucide-react";
import { DEFAULT_USER_NAME, LANGUAGES, lang, type LangCode } from "../data";
import { resetSavedWork, useStore } from "../store";
import { core, loadCore, type CoreApi } from "../core";
import {
  addDictionary,
  dropDictionary,
  findDictionaries,
  type FoundDictionary,
} from "../editor/spelling";
import { fmtNum, plural } from "../format";
import { storageStatus } from "../persist";
import { ConfirmButton } from "./TermbaseDialogs";
import { LangChip, PageHeader } from "../components/ui";
import { TeamServer } from "./TeamServer";
import { fmtDate } from "../format";

export function Settings() {
  const { state, dispatch } = useStore();
  return (
    <div className="page page-narrow">
      <PageHeader title="Settings" />

      <YourName />

      <section className="panel" aria-labelledby="s-words">
        <h2 id="s-words" className="panel-title">
          Terminology wording
        </h2>
        <p className="help">
          Project termbases work like branches of a master. Choose how the
          interface names this. The behaviour is the same either way.
        </p>
        <fieldset className="field">
          <legend className="sr-only">Wording</legend>
          <label className="choice">
            <input
              type="radio"
              name="words"
              checked={!state.gitWords}
              onChange={() => dispatch({ type: "setGitWords", value: false })}
            />
            <span className="choice-body">
              <span className="choice-title">Plain words</span>
              <span className="choice-desc">
                Project termbase, review and merge, update from master, history.
              </span>
            </span>
          </label>
          <label className="choice">
            <input
              type="radio"
              name="words"
              checked={state.gitWords}
              onChange={() => dispatch({ type: "setGitWords", value: true })}
            />
            <span className="choice-body">
              <span className="choice-title">Git terms</span>
              <span className="choice-desc">
                Branch, diff and merge, pull, log, ours and theirs.
              </span>
            </span>
          </label>
        </fieldset>
      </section>

      <section className="panel" aria-labelledby="s-editor">
        <h2 id="s-editor" className="panel-title">
          Editor
        </h2>
        <div className="field">
          <label className="choice">
            <input
              type="checkbox"
              checked={state.autoInsert}
              onChange={(e) =>
                dispatch({
                  type: "setEditorOption",
                  option: "autoInsert",
                  value: e.target.checked,
                })
              }
            />
            <span className="choice-body">
              <span className="choice-title">
                Insert the best memory match when a segment opens
              </span>
              <span className="choice-desc">
                Only into empty segments. The match percentage stays next to the
                segment until you confirm.
              </span>
            </span>
          </label>
          <label className="choice">
            <input
              type="checkbox"
              checked={state.autoPropagate}
              onChange={(e) =>
                dispatch({
                  type: "setEditorOption",
                  option: "autoPropagate",
                  value: e.target.checked,
                })
              }
            />
            <span className="choice-body">
              <span className="choice-title">
                Fill repetitions when you confirm
              </span>
              <span className="choice-desc">
                Other segments with the same text and tags get the same
                translation and are confirmed. Segments you already confirmed
                are left alone.
              </span>
            </span>
          </label>
        </div>
        <fieldset className="field">
          <legend className="field-label">
            Context matches (CM) in new projects
          </legend>
          <p className="help">
            Each project counts context matches its own way; change it in the
            project's settings.
          </p>
          <label className="choice">
            <input
              type="radio"
              name="ctx"
              checked={state.contextRule === "previous"}
              onChange={() =>
                dispatch({ type: "setContextRule", value: "previous" })
              }
            />
            <span className="choice-body">
              <span className="choice-title">
                The segment before also matches
              </span>
              <span className="choice-desc">The usual rule.</span>
            </span>
          </label>
          <label className="choice">
            <input
              type="radio"
              name="ctx"
              checked={state.contextRule === "both"}
              onChange={() =>
                dispatch({ type: "setContextRule", value: "both" })
              }
            />
            <span className="choice-body">
              <span className="choice-title">
                The segments before and after also match
              </span>
              <span className="choice-desc">
                Stricter: such a match is often shown as 101%.
              </span>
            </span>
          </label>
        </fieldset>
      </section>

      <Templates />

      <Dictionaries />

      <TeamServer />

      <SavedWork />
    </div>
  );
}

function Dictionaries() {
  const { state, dispatch, notify } = useStore();
  const [c, setC] = useState<CoreApi | null>(() => core());
  const input = useRef<HTMLInputElement>(null);
  const [found, setFound] = useState<(FoundDictionary & { pick: LangCode })[]>(
    [],
  );
  const [problems, setProblems] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (c) return;
    let live = true;
    loadCore().then(
      (x) => live && setC(x),
      (e: unknown) =>
        live &&
        notify(
          `The core could not be loaded: ${e instanceof Error ? e.message : String(e)}`,
        ),
    );
    return () => {
      live = false;
    };
  }, [c, notify]);

  const usedBy = (name: string) => [
    ...state.projects
      .filter((p) => Object.values(p.qa.dictionaries).includes(name))
      .map((p) => p.name),
    ...state.templates
      .filter((t) => Object.values(t.qa.dictionaries).includes(name))
      .map((t) => `template ${t.name}`),
  ];

  async function add() {
    if (!c) return;
    setBusy(true);
    const added: string[] = [];
    const failed: string[] = [];
    let kept = true;
    for (const d of found) {
      try {
        const r = await addDictionary(c, d, d.pick);
        dispatch({ type: "addDictionary", dictionary: r.dictionary });
        added.push(
          `${d.name} (${plural(r.dictionary.words, "word", "words")})`,
        );
        kept = kept && r.kept;
      } catch (e) {
        failed.push(`${d.name}: ${e instanceof Error ? e.message : String(e)}`);
      }
    }
    setFound([]);
    setProblems(failed);
    setBusy(false);
    if (added.length)
      notify(
        `Added ${added.join(", ")}` +
          (kept
            ? ""
            : ". They are kept only until you close this page: this browser does not allow saving"),
      );
  }

  return (
    <section className="panel" aria-labelledby="s-dict">
      <h2 id="s-dict" className="panel-title">
        Spelling dictionaries
      </h2>
      <p className="help">
        Hunspell dictionaries, as LibreOffice and Firefox use them, checked by
        the core. Each project chooses one per target language in its checks.
      </p>
      {state.dictionaries.length > 0 ? (
        <ul className="settings-list">
          {state.dictionaries.map((d) => {
            const users = usedBy(d.name);
            return (
              <li key={d.name} className="row">
                <span>
                  <span className="cell-title">{d.name}</span>{" "}
                  <LangChip code={d.lang} />{" "}
                  <span className="muted">
                    {fmtNum(d.words)} words
                    {users.length ? `, used by ${users.join(", ")}` : ""}
                  </span>
                </span>
                <ConfirmButton
                  label="Delete"
                  disabled={users.length > 0 || !c}
                  confirm={`Delete the dictionary ${d.name}?`}
                  action="Delete"
                  onConfirm={async () => {
                    if (!c) return;
                    await dropDictionary(c, d.name);
                    dispatch({ type: "deleteDictionary", name: d.name });
                    notify(`Deleted the dictionary ${d.name}`);
                  }}
                />
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="muted">No dictionary yet.</p>
      )}
      {state.dictionaries.some((d) => usedBy(d.name).length) && (
        <p className="help">
          A dictionary a project or template uses can't be deleted; choose
          another there first.
        </p>
      )}
      {found.length > 0 && (
        <div className="field" role="group" aria-label="Dictionaries to add">
          <span className="field-label">Found in the files</span>
          <ul className="settings-list">
            {found.map((d, i) => (
              <li key={d.name} className="row">
                <span className="cell-title">{d.name}</span>
                <label htmlFor={`dl-${i}`} className="sr-only">
                  Language of {d.name}
                </label>
                <select
                  id={`dl-${i}`}
                  className="select-small"
                  value={d.pick}
                  onChange={(e) =>
                    setFound((f) =>
                      f.map((x, j) =>
                        j === i ? { ...x, pick: e.target.value } : x,
                      ),
                    )
                  }
                >
                  {!d.pick && <option value="">Choose the language</option>}
                  {LANGUAGES.map((l) => (
                    <option key={l.code} value={l.code}>
                      {l.name}
                    </option>
                  ))}
                  {d.pick && !LANGUAGES.some((l) => l.code === d.pick) && (
                    <option value={d.pick}>{lang(d.pick).name}</option>
                  )}
                </select>
                {state.dictionaries.some((x) => x.name === d.name) && (
                  <span className="muted">replaces the one of this name</span>
                )}
              </li>
            ))}
          </ul>
          <div className="row">
            <button
              type="button"
              className="btn btn-primary btn-small"
              disabled={busy || found.some((d) => !d.pick)}
              onClick={() => void add()}
            >
              {busy
                ? "Adding…"
                : `Add ${plural(found.length, "dictionary", "dictionaries")}`}
            </button>
            <button
              type="button"
              className="btn btn-quiet btn-small"
              onClick={() => setFound([])}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
      {problems.map((p) => (
        <p key={p} className="error" role="alert">
          {p}
        </p>
      ))}
      <button
        type="button"
        className="btn btn-small"
        disabled={!c || busy}
        onClick={() => input.current?.click()}
      >
        <Upload size={14} aria-hidden="true" />
        Add dictionaries
      </button>
      <input
        ref={input}
        type="file"
        multiple
        hidden
        accept=".oxt,.zip,.aff,.dic"
        onChange={async (e) => {
          const files = e.target.files ? [...e.target.files] : [];
          e.target.value = "";
          if (!c || !files.length) return;
          const r = await findDictionaries(c, files);
          setFound(r.found.map((d) => ({ ...d, pick: d.lang })));
          setProblems(r.problems);
        }}
      />
    </section>
  );
}

function Templates() {
  const { state, dispatch, notify } = useStore();
  return (
    <section className="panel" aria-labelledby="s-tpl">
      <h2 id="s-tpl" className="panel-title">
        Project templates
      </h2>
      {state.templates.length === 0 ? (
        <p className="help">
          No template yet. Save a project's settings as a template at the end of
          New project, or from a project's settings.
        </p>
      ) : (
        <ul className="settings-list">
          {state.templates.map((t) => (
            <li key={t.id} className="row">
              <span>
                <span className="cell-title">{t.name}</span>{" "}
                <span className="lang-pair">
                  <LangChip code={t.sourceLang} /> →{" "}
                  {t.targetLangs.map((l) => (
                    <LangChip key={l} code={l} />
                  ))}
                </span>{" "}
                <span className="muted">
                  {t.client ? `${t.client}, ` : ""}saved {fmtDate(t.createdAt)}
                </span>
              </span>
              <ConfirmButton
                label="Delete"
                confirm={`Delete the template “${t.name}”? Projects made from it stay as they are.`}
                action="Delete"
                onConfirm={() => {
                  dispatch({ type: "deleteTemplate", templateId: t.id });
                  notify(`Deleted the template “${t.name}”`);
                }}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** The name work on this computer is signed with. */
function YourName() {
  const { state, dispatch } = useStore();
  const [name, setName] = useState(
    state.userName === DEFAULT_USER_NAME ? "" : state.userName,
  );
  return (
    <section className="panel" aria-labelledby="s-name">
      <h2 id="s-name" className="panel-title">
        Your name
      </h2>
      <div className="field">
        <label htmlFor="user-name" className="field-label">
          Name
        </label>
        <input
          id="user-name"
          value={name}
          placeholder={DEFAULT_USER_NAME}
          autoComplete="name"
          onChange={(e) => setName(e.target.value)}
          onBlur={() => dispatch({ type: "setUserName", name })}
        />
        <p className="help">
          Entries and translations you change on this computer are signed with
          it, in their history and in exported files. On a team server your
          account's name is used.
        </p>
      </div>
    </section>
  );
}

function SavedWork() {
  const { dispatch, notify } = useStore();
  const st = storageStatus();
  return (
    <section className="panel" aria-labelledby="s-saved">
      <h2 id="s-saved" className="panel-title">
        Saved work
      </h2>
      {st.status === "ok" ? (
        <p className="help">
          Projects, translations, termbases, memories, templates and these
          settings are saved in this browser as you work, and are here again on
          your next visit. They stay on this computer.
        </p>
      ) : (
        <p className="detect detect-warn" role="status">
          <AlertTriangle size={16} aria-hidden="true" />
          <span>
            This browser does not let Interlinea save ({st.error}). Your work
            is kept only until you close the page. Allow site data for this
            page, or leave private browsing, to keep it.
          </span>
        </p>
      )}
      <ConfirmButton
        label="Delete everything kept in this browser"
        confirm="Delete all your projects, translations, termbases, memories and settings in this browser?"
        action="Delete everything"
        onConfirm={() => {
          void resetSavedWork(dispatch).then(() =>
            notify("Everything kept in this browser was deleted"),
          );
        }}
        small={false}
      />
    </section>
  );
}
