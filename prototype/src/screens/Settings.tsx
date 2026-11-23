import { AlertTriangle, Cloud } from "lucide-react";
import { SERVER } from "../data";
import { resetSavedWork, useStore } from "../store";
import { storageStatus } from "../persist";
import { ConfirmButton } from "./TermbaseDialogs";
import { LangChip, PageHeader } from "../components/ui";
import { fmtDate } from "../format";

export function Settings() {
  const { state, dispatch } = useStore();
  return (
    <div className="page page-narrow">
      <PageHeader title="Settings" />

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
              <span className="choice-desc">As in Trados Studio.</span>
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
                Stricter, as memoQ counts 101% matches.
              </span>
            </span>
          </label>
        </fieldset>
      </section>

      <Templates />

      <section className="panel" aria-labelledby="s-servers">
        <h2 id="s-servers" className="panel-title">
          Servers
        </h2>
        <ul className="server-list">
          <li>
            <Cloud size={18} aria-hidden="true" />
            <div>
              <div className="cell-title">{SERVER}</div>
              <div className="cell-sub">
                Connected as Dan. Sample server in this prototype.
              </div>
            </div>
            <span className="status status-approved">Connected</span>
          </li>
        </ul>
      </section>

      <SavedWork />
    </div>
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
            This browser does not let the prototype save ({st.error}). Your work
            is kept only until you close the page. Allow site data for this
            page, or leave private browsing, to keep it.
          </span>
        </p>
      )}
      <ConfirmButton
        label="Start again from the sample data"
        confirm="Delete all your projects, translations and termbase changes?"
        action="Delete and start again"
        onConfirm={() => {
          void resetSavedWork(dispatch).then(() =>
            notify("Started again from the sample data"),
          );
        }}
        small={false}
      />
    </section>
  );
}
