import {
  GrammarServices,
  ProgramAccess,
  TranslationServices,
} from "./Integrations";
import { useEffect, useRef, useState } from "react";
import { AlertTriangle, Upload } from "lucide-react";
import { DEFAULT_USER_NAME, LANGUAGES, lang, type LangCode } from "../data";
import { resetSavedWork, useStore } from "../store";
import { core, loadCore, type CoreApi } from "../core";
import {
  addDictionary,
  dropDictionary,
  findDictionaries,
  useIncludedDictionaries,
  type FoundDictionary,
} from "../editor/spelling";
import { fmtNum, plural } from "../format";
import { storageStatus } from "../persist";
import { ConfirmButton } from "./TermbaseDialogs";
import { LangChip, PageHeader } from "../components/ui";
import { TeamServer } from "./TeamServer";
import { fmtDate } from "../format";
import { browserLanguage, chooseLanguage, chosenLanguage, tl, tlx, UI_LANGUAGES, type UiLanguage } from "../i18n";
import { chooseAppearance, useAppearance, type Appearance as Look } from "../theme";

/** The language the interface is shown in. */
function InterfaceLanguage() {
  const chosen = chosenLanguage();
  const browser = UI_LANGUAGES.find((l) => l.code === browserLanguage())!.name;
  return (
    <section className="panel" aria-labelledby="s-ui-lang">
      <h2 id="s-ui-lang" className="panel-title">
        {tl("Interface language")}
      </h2>
      <p className="help">
        {tl("The language of menus, buttons and messages. Documents, projects and termbases keep their own languages.")}
      </p>
      <div className="field">
        <label htmlFor="ui-lang">{tl("Show Interlinea in")}</label>
        <select
          id="ui-lang"
          value={chosen}
          onChange={(e) => chooseLanguage(e.target.value as UiLanguage | "")}
        >
          <option value="">{tl("The browser's language ({name})", { name: browser })}</option>
          {UI_LANGUAGES.map((l) => (
            <option key={l.code} value={l.code} lang={l.code}>
              {l.name}
            </option>
          ))}
        </select>
      </div>
    </section>
  );
}

/** Light or dark, or as the system is set. */
function Appearance() {
  const { chosen, shown } = useAppearance();
  const choices: { value: Look; title: string; desc: string }[] = [
    {
      value: "",
      title: tl("Same as the system"),
      desc: shown === "dark" ? tl("Dark now, as the system is set.") : tl("Light now, as the system is set."),
    },
    { value: "light", title: tl("Light"), desc: tl("Dark text on light surfaces.") },
    { value: "dark", title: tl("Dark"), desc: tl("Light text on dark surfaces, easier on the eyes in a dim room.") },
  ];
  return (
    <section className="panel" aria-labelledby="s-look">
      <h2 id="s-look" className="panel-title">
        {tl("Appearance")}
      </h2>
      <fieldset className="field">
        <legend className="sr-only">{tl("Appearance")}</legend>
        {choices.map((c) => (
          <label className="choice" key={c.value || "system"}>
            <input
              type="radio"
              name="appearance"
              checked={chosen === c.value}
              onChange={() => chooseAppearance(c.value)}
            />
            <span className="choice-body">
              <span className="choice-title">{c.title}</span>
              <span className="choice-desc">{c.desc}</span>
            </span>
          </label>
        ))}
      </fieldset>
    </section>
  );
}

export function Settings() {
  const { state, dispatch } = useStore();
  return (
    <div className="page page-narrow">
      <PageHeader title={tl("Settings")} />

      <InterfaceLanguage />

      <Appearance />

      <YourName />

      <section className="panel" aria-labelledby="s-words">
        <h2 id="s-words" className="panel-title">
          {tl("Terminology wording")}
        </h2>
        <p className="help">
          {tl("Project termbases work like branches of a master. Choose how the interface names this. The behaviour is the same either way.")}
        </p>
        <fieldset className="field">
          <legend className="sr-only">{tl("Wording")}</legend>
          <label className="choice">
            <input
              type="radio"
              name="words"
              checked={!state.gitWords}
              onChange={() => dispatch({ type: "setGitWords", value: false })}
            />
            <span className="choice-body">
              <span className="choice-title">{tl("Plain words")}</span>
              <span className="choice-desc">
                {tl("Project termbase, review and merge, update from master, history.")}
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
              <span className="choice-title">{tl("Git terms")}</span>
              <span className="choice-desc">
                {tl("Branch, diff and merge, pull, log, ours and theirs.")}
              </span>
            </span>
          </label>
        </fieldset>
      </section>

      <section className="panel" aria-labelledby="s-editor">
        <h2 id="s-editor" className="panel-title">
          {tl("Editor")}
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
                {tl("Insert the best memory match when a segment opens")}
              </span>
              <span className="choice-desc">
                {tl("Only into empty segments. The match percentage stays next to the segment until you confirm.")}
              </span>
            </span>
          </label>
          <label className="choice">
            <input
              type="checkbox"
              checked={state.autoComplete}
              onChange={(e) =>
                dispatch({
                  type: "setEditorOption",
                  option: "autoComplete",
                  value: e.target.checked,
                })
              }
            />
            <span className="choice-body">
              <span className="choice-title">
                {tl("Complete words while you type")}
              </span>
              <span className="choice-desc">
                {tl("After two letters, the terms found for the segment and the words of its memory matches are offered. Tab or Enter takes one, Escape closes the list.")}
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
                {tl("Fill repetitions when you confirm")}
              </span>
              <span className="choice-desc">
                {tl("Other segments with the same text and tags get the same translation and are confirmed. Segments you already confirmed are left alone.")}
              </span>
            </span>
          </label>
        </div>
        <fieldset className="field">
          <legend className="field-label">
            {tl("Context matches (CM) in new projects")}
          </legend>
          <p className="help">
            {tl("Each project counts context matches its own way; change it in the project's settings.")}
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
                {tl("The segment before also matches")}
              </span>
              <span className="choice-desc">{tl("The usual rule.")}</span>
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
                {tl("The segments before and after also match")}
              </span>
              <span className="choice-desc">
                {tl("Stricter: such a match is often shown as 101%.")}
              </span>
            </span>
          </label>
        </fieldset>
      </section>

      <Templates />

      <Dictionaries />

      <TranslationServices />
      <GrammarServices />

      <ProgramAccess />

      <TeamServer />

      <SavedWork />

      <About />
    </div>
  );
}

function Dictionaries() {
  const { state, dispatch, notify } = useStore();
  const included = useIncludedDictionaries();
  const isIncluded = (name: string) =>
    included.some((d) => d.name.toLowerCase() === name.toLowerCase());
  const isAdded = (name: string) =>
    state.dictionaries.some((d) => d.name.toLowerCase() === name.toLowerCase());
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
          tl("The core could not be loaded: {x}", { x: e instanceof Error ? e.message : String(e) }),
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
        {tl("Spelling dictionaries")}
      </h2>
      <p className="help">
        {tl("Hunspell dictionaries, as LibreOffice and Firefox use them, checked by the core. Each project chooses one per target language in its checks; a new project takes the one that fits each language.")}
      </p>
      {included.length > 0 && (
        <details className="settings-details field">
          <summary>
            {included.length === 1
              ? tl("1 dictionary comes with Interlinea")
              : tl("{length} dictionaries come with Interlinea", { length: included.length })}
          </summary>
          <ul className="settings-list">
            {included.map((d) => {
              const users = usedBy(d.name);
              return (
                <li key={d.name} className="row">
                  <span>
                    <span className="cell-title">{d.name}</span>{" "}
                    <LangChip code={d.lang} />{" "}
                    <span className="muted">
                      {tlx("{words} words{x}", { words: fmtNum(d.words), x: isAdded(d.name)
                        ? tl(", replaced by the one you added")
                        : users.length
                          ? tl(", used by {join}", { join: users.join(", ") })
                          : "" })}</span>
                  </span>
                </li>
              );
            })}
          </ul>
          <p className="help">
            {tl("From the LibreOffice dictionaries, each with its authors' licence. They are fetched when a project first spells with them. Add one of the same name to use your own instead.")}
          </p>
        </details>
      )}
      {included.length > 0 && (
        <span className="field-label">{tl("Added in this browser")}</span>
      )}
      {state.dictionaries.length > 0 ? (
        <ul className="settings-list" aria-label={tl("Added dictionaries")}>
          {state.dictionaries.map((d) => {
            // With an included one of the name, projects keep a dictionary.
            const users = isIncluded(d.name) ? [] : usedBy(d.name);
            return (
              <li key={d.name} className="row">
                <span>
                  <span className="cell-title">{d.name}</span>{" "}
                  <LangChip code={d.lang} />{" "}
                  <span className="muted">
                    {tlx("{words} words{x}", { words: fmtNum(d.words), x: isIncluded(d.name)
                      ? tl(", used instead of the included one")
                      : users.length
                        ? tl(", used by {join}", { join: users.join(", ") })
                        : "" })}</span>
                </span>
                <ConfirmButton
                  label={tl("Delete")}
                  disabled={users.length > 0 || !c}
                  confirm={
                    isIncluded(d.name)
                      ? tl("Delete the dictionary {name}? The included one is used again.", { name: d.name })
                      : tl("Delete the dictionary {name}?", { name: d.name })
                  }
                  action={tl("Delete")}
                  onConfirm={async () => {
                    if (!c) return;
                    await dropDictionary(c, d.name);
                    dispatch({ type: "deleteDictionary", name: d.name });
                    notify(tl("Deleted the dictionary {name}", { name: d.name }));
                  }}
                />
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="muted">
          {included.length > 0 ? tl("None added.") : tl("No dictionary yet.")}
        </p>
      )}
      {state.dictionaries.some(
        (d) => !isIncluded(d.name) && usedBy(d.name).length,
      ) && (
        <p className="help">
          {tl("A dictionary a project or template uses can't be deleted; choose another there first.")}
        </p>
      )}
      {found.length > 0 && (
        <div className="field" role="group" aria-label={tl("Dictionaries to add")}>
          <span className="field-label">{tl("Found in the files")}</span>
          <ul className="settings-list">
            {found.map((d, i) => (
              <li key={d.name} className="row">
                <span className="cell-title">{d.name}</span>
                <label htmlFor={`dl-${i}`} className="sr-only">
                  {tlx("Language of {name}", { name: d.name })}</label>
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
                  {!d.pick && <option value="">{tl("Choose the language")}</option>}
                  {LANGUAGES.map((l) => (
                    <option key={l.code} value={l.code}>
                      {l.name}
                    </option>
                  ))}
                  {d.pick && !LANGUAGES.some((l) => l.code === d.pick) && (
                    <option value={d.pick}>{lang(d.pick).name}</option>
                  )}
                </select>
                {isAdded(d.name) ? (
                  <span className="muted">{tl("replaces the one of this name")}</span>
                ) : (
                  isIncluded(d.name) && (
                    <span className="muted">
                      {tl("used instead of the included one")}
                    </span>
                  )
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
                ? tl("Adding…")
                : tl("Add {length}", { length: plural(found.length, "dictionary", "dictionaries") })}
            </button>
            <button
              type="button"
              className="btn btn-quiet btn-small"
              onClick={() => setFound([])}
            >
              {tl("Cancel")}
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
        {tl("Add dictionaries")}
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
        {tl("Project templates")}
      </h2>
      {state.templates.length === 0 ? (
        <p className="help">
          {tl("No template yet. Save a project's settings as a template at the end of New project, or from a project's settings.")}
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
                  {tlx("{x}saved {createdAt}", { x: t.client ? `${t.client}, ` : "", createdAt: fmtDate(t.createdAt) })}</span>
              </span>
              <ConfirmButton
                label={tl("Delete")}
                confirm={tl("Delete the template “{name}”? Projects made from it stay as they are.", { name: t.name })}
                action={tl("Delete")}
                onConfirm={() => {
                  dispatch({ type: "deleteTemplate", templateId: t.id });
                  notify(tl("Deleted the template “{name}”", { name: t.name }));
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
        {tl("Your name")}
      </h2>
      <div className="field">
        <label htmlFor="user-name" className="field-label">
          {tl("Name")}
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
          {tl("Entries and translations you change on this computer are signed with it, in their history and in exported files. On a team server your account's name is used.")}
        </p>
      </div>
    </section>
  );
}

/** What Interlinea is in law: its licence, whose it is, and where its source is. */
function About() {
  return (
    <section className="panel" aria-labelledby="s-about">
      <h2 id="s-about" className="panel-title">
        {tl("About Interlinea")}
      </h2>
      <p className="help">
        {tl("Interlinea is free software under the GNU Affero General Public License, version 3: you may use it, pass it on and change it under the terms of that licence. It comes without warranty.")}
      </p>
      <p className="help">{tl("Copyright © 2026 Dan Dreßen.")}</p>
      <div className="row">
        <a
          className="btn btn-small"
          href="LICENSE.txt"
          target="_blank"
          rel="noreferrer"
        >
          {tl("Read the licence")}
        </a>
        <a
          className="btn btn-small"
          href="https://github.com/4nonX/interlinea"
          target="_blank"
          rel="noreferrer"
        >
          {tl("Source code")}
        </a>
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
        {tl("Saved work")}
      </h2>
      {st.status === "ok" ? (
        <p className="help">
          {tl("Projects, translations, termbases, memories, templates and these settings are saved in this browser as you work, and are here again on your next visit. They stay on this computer.")}
        </p>
      ) : (
        <p className="detect detect-warn" role="status">
          <AlertTriangle size={16} aria-hidden="true" />
          <span>
            {tlx("This browser does not let Interlinea save ({error}). Your work is kept only until you close the page. Allow site data for this page, or leave private browsing, to keep it.", { error: st.error })}
          </span>
        </p>
      )}
      <ConfirmButton
        label={tl("Delete everything kept in this browser")}
        confirm={tl("Delete all your projects, translations, termbases, memories and settings in this browser?")}
        action={tl("Delete everything")}
        onConfirm={() => {
          void resetSavedWork(dispatch).then(() =>
            notify(tl("Everything kept in this browser was deleted")),
          );
        }}
        small={false}
      />
    </section>
  );
}
