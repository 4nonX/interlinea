import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowLeft,
  Download,
  FileText,
  PencilLine,
  Trash2,
} from "lucide-react";
import {
  fileTargets,
  freshCopy,
  lang,
  primary,
  projectMemoryName,
  type LangCode,
  type Project,
  type ProjectFile,
  type TranslationMemory,
} from "../data";
import { core, download, loadCore, tm, type CoreApi } from "../core";
import { useLocalStore } from "../store";
import { confirmedShare } from "../editor/model";
import {
  dropMemory,
  openMemory,
  openRefs,
  persistMemory,
} from "../editor/memory";
import {
  draftOf,
  memoryRefs,
  pretranslateFiles,
  problemsAt,
  projectMemoryOf,
  templateOf,
  validate,
  type Draft,
  type Problem,
} from "../project";
import {
  buildFiles,
  docName,
  docTarget,
  extractWordDocs,
  knownLang,
  type DraftDoc,
} from "../setup";
import {
  ChecksSection,
  MemoriesSection,
  PrepareSection,
  Problems,
  TargetPicker,
  TerminologySection,
  type NewMemory,
} from "./ProjectSections";
import { FilesField, fromLocalInput, toLocalInput } from "./NewProject";
import { ConfirmButton } from "./TermbaseDialogs";
import { LangChip, PageHeader } from "../components/ui";
import { fmtNum, plural } from "../format";
import { go } from "../App";

export function ProjectSettings({ projectId }: { projectId: string }) {
  const { state } = useLocalStore();
  const project = state.projects.find((p) => p.id === projectId);
  if (!project)
    return (
      <div className="page page-narrow">
        <div className="empty">
          <p className="empty-title">Project not found</p>
          <a className="btn btn-small" href="#/projects">
            Back to projects
          </a>
        </div>
      </div>
    );
  // A fresh form whenever the saved project changes underneath.
  return <SettingsForm key={project.id} project={project} />;
}

function SettingsForm({ project }: { project: Project }) {
  const { state, dispatch, notify } = useLocalStore();
  const [c, setC] = useState(() => core());
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

  const saved = useMemo(() => draftOf(project), [project]);
  const [draft, setDraft] = useState<Draft>(saved);
  const [added, setAdded] = useState<DraftDoc[]>([]);
  const [removed, setRemoved] = useState<Set<string>>(new Set());
  const [created, setCreated] = useState<NewMemory[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");

  const kept = project.files.filter((f) => !removed.has(f.id));
  const files: Draft["files"] = [
    ...kept.map((f) => ({
      name: f.name,
      role: f.role,
      targetLang: f.targetLang,
    })),
    ...added.map((d) => ({
      name: docName(d),
      role: d.role,
      targetLang: docTarget(d),
    })),
  ];
  const full: Draft = { ...draft, files };
  const fixedTargets = [
    ...new Set(
      files.map((f) => f.targetLang).filter((t): t is LangCode => !!t),
    ),
  ];
  const changed =
    JSON.stringify(full) !== JSON.stringify(saved) ||
    added.length > 0 ||
    removed.size > 0 ||
    created.length > 0;

  // Memory and dictionary settings exist only for the project's target languages.
  useEffect(() => {
    setDraft((d) => {
      const keys = Object.keys(d.memories);
      const memoriesFit =
        keys.length === d.targetLangs.length &&
        d.targetLangs.every((t) => keys.includes(t));
      const dictsFit = Object.keys(d.qa.dictionaries).every((l) =>
        d.targetLangs.includes(l),
      );
      if (memoriesFit && dictsFit) return d;
      return {
        ...d,
        memories: Object.fromEntries(
          d.targetLangs.map((t) => [t, d.memories[t] ?? []]),
        ),
        qa: {
          ...d.qa,
          dictionaries: Object.fromEntries(
            Object.entries(d.qa.dictionaries).filter(([l]) =>
              d.targetLangs.includes(l),
            ),
          ),
        },
      };
    });
  }, [draft.targetLangs]);

  const problems: Problem[] = useMemo(() => {
    if (!c) return [];
    const ps = validate(c, full, {
      memories: [...state.memories, ...created.map((n) => n.memory)],
      termbases: state.termbases,
      dictionaries: state.dictionaries,
    });
    if (
      state.projects.some(
        (p) =>
          p.id !== project.id &&
          p.name.toLowerCase() === full.name.trim().toLowerCase(),
      )
    )
      ps.push({ field: "name", message: "Another project has this name." });
    const otherSource = added.find(
      (d) =>
        d.role === "translatable" &&
        d.doc.bilingual &&
        knownLang(d.doc.bilingual.sourceLang) !== project.sourceLang,
    );
    if (otherSource)
      ps.push({
        field: "files",
        message: `${docName(otherSource)} is a bilingual file from ${lang(knownLang(otherSource.doc.bilingual!.sourceLang)).name}, but the project translates from ${lang(project.sourceLang).name}.`,
      });
    if (added.some((d) => d.doc.sdlPackage))
      ps.push({
        field: "files",
        message:
          "Files of a Trados package come with the project they were made for; start a new project from the package.",
      });
    return ps;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    c,
    JSON.stringify(full),
    state.memories,
    state.termbases,
    state.dictionaries,
    state.projects,
    created,
    added,
  ]);

  const removedTargets = project.targetLangs.filter(
    (t) => !draft.targetLangs.includes(t),
  );
  const lostWork = allSegmentsIn(project, removedTargets).filter(
    (s) => s.status !== "new",
  ).length;
  const removedWork = project.files
    .filter((f) => removed.has(f.id))
    .reduce(
      (n, f) =>
        n +
        Object.values(f.segments)
          .flat()
          .filter((s) => s.status !== "new").length,
      0,
    );

  async function save() {
    if (!c || problems.length) return;
    setSaving(true);
    setSaveError("");
    try {
      const now = new Date().toISOString();
      const newTargets = draft.targetLangs.filter(
        (t) => !project.targetLangs.includes(t),
      );
      // Files kept get segments for new languages and lose removed ones.
      const first =
        project.targetLangs.find((t) => draft.targetLangs.includes(t)) ??
        project.targetLangs[0]!;
      const keptFiles: ProjectFile[] = kept.map((f) => {
        if (
          f.role !== "translatable" ||
          f.targetLang ||
          !Object.keys(f.segments).length
        )
          return f;
        const from = f.segments[first] ?? Object.values(f.segments)[0] ?? [];
        const segments = Object.fromEntries(
          draft.targetLangs.map((t) => [t, f.segments[t] ?? freshCopy(from)]),
        );
        return { ...f, segments };
      });
      const ex = await extractWordDocs(added, project.sourceLang);
      const n = project.files.length;
      const addedFiles =
        buildFiles(
          added,
          ex,
          draft.targetLangs,
          (i) => `f${Date.now().toString(36)}-${n + i + 1}`,
        ) ?? [];
      const memoriesNow = [...state.memories, ...created.map((x) => x.memory)];
      for (const x of created) {
        dispatch({ type: "createMemory", memory: x.memory });
        await persistMemory(c, x.memory.name);
      }
      const own = state.memories.filter((m) => m.projectId === project.id);
      const removeMemories = own
        .filter(
          (m) =>
            !draft.projectMemory || !draft.targetLangs.includes(m.targetLang),
        )
        .map((m) => m.name);
      const addMemories: TranslationMemory[] = draft.projectMemory
        ? draft.targetLangs
            .filter(
              (t) =>
                !own.some(
                  (m) => m.targetLang === t && !removeMemories.includes(m.name),
                ),
            )
            .map((t) => ({
              name: unique(
                projectMemoryName(draft.name.trim(), t),
                memoriesNow,
              ),
              sourceLang: project.sourceLang,
              targetLang: t,
              client: draft.client.trim() || undefined,
              projectId: project.id,
              createdAt: now,
            }))
        : [];
      for (const name of removeMemories) await dropMemory(name);
      const t = draft.terminology;
      const next: Project = {
        ...project,
        name: draft.name.trim(),
        client: draft.client.trim() || undefined,
        description: draft.description.trim() || undefined,
        due: draft.due || undefined,
        targetLangs: draft.targetLangs,
        files: [...keptFiles, ...addedFiles],
        memories: draft.memories,
        projectMemory: draft.projectMemory,
        setup: {
          ...project.setup,
          masters: t.mode === "own" ? [] : t.masters,
          writeTo: t.mode === "direct" ? t.writeTo : undefined,
          autoUpdate: t.mode === "layered" && t.autoUpdate,
        },
        context: draft.context,
        qa: draft.qa,
        prepare: draft.prepare,
      };
      dispatch({
        type: "updateProject",
        change: {
          project: next,
          addLanguages: t.addLanguages,
          addMemories,
          removeMemories,
        },
      });
      notify(
        `Saved the settings of “${next.name}”` +
          (newTargets.length
            ? `; ${newTargets.map((x) => lang(x).name).join(", ")} added to every file`
            : ""),
      );
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="page page-narrow">
      <PageHeader title={`${project.name}: settings`}>
        <a className="btn btn-quiet" href="#/projects">
          <ArrowLeft size={14} aria-hidden="true" />
          Projects
        </a>
        <a className="btn" href={`#/editor/${project.id}`}>
          <PencilLine size={14} aria-hidden="true" />
          Open in the editor
        </a>
      </PageHeader>

      <section className="panel" aria-labelledby="ps-details">
        <h2 id="ps-details" className="panel-title">
          Details
        </h2>
        <div className="field-grid">
          <div className="field">
            <label htmlFor="ps-name">Project name</label>
            <input
              id="ps-name"
              value={draft.name}
              onChange={(e) =>
                setDraft((d) => ({ ...d, name: e.target.value }))
              }
              aria-invalid={problemsAt(problems, "name").length > 0}
            />
            <Problems messages={problemsAt(problems, "name")} />
          </div>
          <div className="field">
            <label htmlFor="ps-client">
              Client <span className="optional">optional</span>
            </label>
            <input
              id="ps-client"
              value={draft.client}
              onChange={(e) =>
                setDraft((d) => ({ ...d, client: e.target.value }))
              }
            />
            <Problems messages={problemsAt(problems, "client")} />
          </div>
          <div className="field">
            <label htmlFor="ps-due">
              Due <span className="optional">optional</span>
            </label>
            <input
              id="ps-due"
              type="datetime-local"
              value={toLocalInput(draft.due)}
              onChange={(e) =>
                setDraft((d) => ({ ...d, due: fromLocalInput(e.target.value) }))
              }
            />
            <Problems messages={problemsAt(problems, "due")} />
          </div>
          <div className="field">
            <label htmlFor="ps-desc">
              Description <span className="optional">optional</span>
            </label>
            <input
              id="ps-desc"
              value={draft.description}
              onChange={(e) =>
                setDraft((d) => ({ ...d, description: e.target.value }))
              }
            />
          </div>
        </div>
      </section>

      <section className="panel" aria-labelledby="ps-files">
        <h2 id="ps-files" className="panel-title">
          Files
        </h2>
        <div className="table-wrap">
          <table className="table file-table">
            <caption className="sr-only">Files of the project</caption>
            <thead>
              <tr>
                <th scope="col">File</th>
                <th scope="col" className="num">
                  Words
                </th>
                <th scope="col">Done</th>
                <th scope="col">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {project.files.map((f) => {
                const gone = removed.has(f.id);
                return (
                  <tr key={f.id} className={gone ? "is-removed" : undefined}>
                    <th scope="row">
                      <span className="file-name">
                        <FileText size={15} aria-hidden="true" />
                        <span className="cell-title">{f.name}</span>
                      </span>
                      <span className="cell-sub">
                        {f.role === "reference"
                          ? "Reference file"
                          : f.targetLang
                            ? `Bilingual file for ${lang(f.targetLang).name}`
                            : Object.keys(f.segments).length
                              ? "Translated into every target language"
                              : "No text the prototype can read"}
                        {gone && " (removed when you save)"}
                      </span>
                    </th>
                    <td className="num">{f.words ? fmtNum(f.words) : "–"}</td>
                    <td>
                      {fileTargets(f, project)
                        .filter((t) => f.segments[t])
                        .map((t) => (
                          <span key={t} className="file-progress">
                            <LangChip code={t} />{" "}
                            {Math.round(confirmedShare(f.segments[t]!) * 100)}%
                          </span>
                        ))}
                    </td>
                    <td>
                      <span className="row">
                        {(f.bytes || f.url) && (
                          <button
                            type="button"
                            className="btn btn-quiet btn-small"
                            onClick={() => void downloadOriginal(f)}
                          >
                            <Download size={14} aria-hidden="true" />
                            Original
                          </button>
                        )}
                        {gone ? (
                          <button
                            type="button"
                            className="btn btn-quiet btn-small"
                            onClick={() =>
                              setRemoved((r) => {
                                const n = new Set(r);
                                n.delete(f.id);
                                return n;
                              })
                            }
                          >
                            Keep
                          </button>
                        ) : (
                          <button
                            type="button"
                            className="btn btn-quiet btn-small"
                            onClick={() =>
                              setRemoved((r) => new Set(r).add(f.id))
                            }
                          >
                            <Trash2 size={14} aria-hidden="true" />
                            Remove
                          </button>
                        )}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <FilesField
          label="Add files"
          docs={added}
          setDocs={setAdded}
          existing={kept.map((f) => f.name)}
        />
        {removedWork > 0 && (
          <p className="detect detect-warn">
            <AlertTriangle size={16} aria-hidden="true" />
            <span>
              Removing files drops{" "}
              {plural(removedWork, "translated segment", "translated segments")}
              . Confirmed ones stay in the memories they were stored in.
            </span>
          </p>
        )}
        <Problems messages={problemsAt(problems, "files")} />
      </section>

      <section className="panel" aria-labelledby="ps-langs">
        <h2 id="ps-langs" className="panel-title">
          Languages
        </h2>
        <div className="field">
          <span className="field-label">Source language</span>
          <p className="lang-fixed">
            {lang(project.sourceLang).name}{" "}
            <LangChip code={project.sourceLang} />
          </p>
          <p className="help">
            The source language stays: every file was segmented by its rules.
          </p>
        </div>
        <fieldset className="field">
          <legend>Target languages</legend>
          <TargetPicker
            source={project.sourceLang}
            targets={draft.targetLangs}
            setTargets={(ts) => setDraft((d) => ({ ...d, targetLangs: ts }))}
            suggestions={project.targetLangs}
            fixed={fixedTargets}
          />
          <p className="help">
            A language you add gets every file to translate, with nothing
            translated yet.
          </p>
          {lostWork > 0 && (
            <p className="detect detect-warn">
              <AlertTriangle size={16} aria-hidden="true" />
              <span>
                Removing {removedTargets.map((x) => lang(x).name).join(", ")}{" "}
                drops{" "}
                {plural(lostWork, "translated segment", "translated segments")}
                {draft.projectMemory
                  ? " and the project memory of that language"
                  : ""}
                .
              </span>
            </p>
          )}
          <Problems messages={problemsAt(problems, "targetLangs")} />
        </fieldset>
      </section>

      <section className="panel" aria-labelledby="ps-mem">
        <h2 id="ps-mem" className="panel-title">
          Memories
        </h2>
        <MemoriesSection
          c={c}
          draft={draft}
          setDraft={setDraft}
          problems={problems}
          created={created}
          setCreated={setCreated}
          projectId={project.id}
        />
        {c && <ProjectMemoryTransfer c={c} project={project} />}
      </section>

      <section className="panel" aria-labelledby="ps-terms">
        <h2 id="ps-terms" className="panel-title">
          Terminology
        </h2>
        <TerminologySection
          draft={draft}
          setDraft={setDraft}
          problems={problems}
          existing
        />
      </section>

      <section className="panel" aria-labelledby="ps-checks">
        <h2 id="ps-checks" className="panel-title">
          Checks and analysis
        </h2>
        <ChecksSection
          c={c}
          targetLangs={draft.targetLangs}
          problems={problems}
          qa={draft.qa}
          setQa={(qa) => setDraft((d) => ({ ...d, qa }))}
          context={draft.context}
          setContext={(context) => setDraft((d) => ({ ...d, context }))}
        />
      </section>

      <section className="panel" aria-labelledby="ps-prep">
        <h2 id="ps-prep" className="panel-title">
          Pre-translation
        </h2>
        <PrepareSection
          prepare={draft.prepare}
          setPrepare={(prepare) => setDraft((d) => ({ ...d, prepare }))}
          problems={problems}
          existing
        />
        {c && <PretranslateNow c={c} project={project} unsaved={changed} />}
      </section>

      <div className="page-actions save-bar">
        {saveError && (
          <p className="error" role="alert">
            Could not save: {saveError}
          </p>
        )}
        {problems.length > 0 && changed && (
          <p className="error" role="alert">
            {plural(problems.length, "setting needs", "settings need")}{" "}
            attention before saving.
          </p>
        )}
        <button
          type="button"
          className="btn btn-quiet"
          disabled={!changed || saving}
          onClick={() => {
            setDraft(saved);
            setAdded([]);
            setRemoved(new Set());
            setCreated([]);
            setSaveError("");
          }}
        >
          Discard changes
        </button>
        <button
          type="button"
          className="btn btn-primary"
          disabled={!changed || saving || problems.length > 0 || !c}
          onClick={() => void save()}
        >
          {saving ? "Saving…" : "Save changes"}
        </button>
      </div>

      <section className="panel" aria-labelledby="ps-tpl">
        <h2 id="ps-tpl" className="panel-title">
          Template
        </h2>
        <SaveAsTemplate project={project} unsaved={changed} />
      </section>

      <section className="panel settings-danger" aria-labelledby="ps-del">
        <h2 id="ps-del" className="panel-title">
          Delete the project
        </h2>
        <p className="help">
          Deletes its files, translations, project termbase and project
          memories. Main memories and masters stay as they are.
        </p>
        <ConfirmButton
          label={
            <>
              <Trash2 size={14} aria-hidden="true" />
              Delete project
            </>
          }
          small={false}
          confirm={`Delete “${project.name}” and everything in it?`}
          action="Delete"
          onConfirm={() => {
            for (const m of state.memories.filter(
              (x) => x.projectId === project.id,
            ))
              void dropMemory(m.name);
            dispatch({ type: "deleteProject", projectId: project.id });
            notify(`Deleted the project “${project.name}”`);
            go({ page: "projects" });
          }}
        />
      </section>
    </div>
  );
}

function allSegmentsIn(p: Project, langs: LangCode[]) {
  return p.files.flatMap((f) => langs.flatMap((l) => f.segments[l] ?? []));
}

function unique(name: string, taken: TranslationMemory[]): string {
  let out = name;
  for (
    let n = 2;
    taken.some((m) => m.name.toLowerCase() === out.toLowerCase());
    n++
  )
    out = `${name} ${n}`;
  return out;
}

async function downloadOriginal(f: ProjectFile) {
  const bytes =
    f.bytes ??
    new Uint8Array(
      await (await fetch(new URL(f.url!, document.baseURI))).arrayBuffer(),
    );
  download(
    bytes,
    f.name.split("/").pop() ?? f.name,
    "application/octet-stream",
  );
}

/** Adds a project memory's units to a main memory, as Trados' "update main memories" does. */
function ProjectMemoryTransfer({
  c,
  project,
}: {
  c: CoreApi;
  project: Project;
}) {
  const { state, notify } = useLocalStore();
  const [targetOf, setTargetOf] = useState<Record<string, string>>({});
  const own = project.targetLangs
    .map((t) => projectMemoryOf(project, t, state.memories))
    .filter((m): m is TranslationMemory => !!m);
  if (!own.length) return null;
  return (
    <div className="field">
      <span className="field-label">Project memories</span>
      <ul className="settings-list">
        {own.map((m) => {
          openMemory(c, m, state.projects);
          const units = tm.count(c, m.name);
          const mains = state.memories.filter(
            (x) =>
              !x.projectId &&
              primary(x.sourceLang) === primary(m.sourceLang) &&
              primary(x.targetLang) === primary(m.targetLang),
          );
          const pick = targetOf[m.name] ?? mains[0]?.name ?? "";
          const id = `pm-${m.name.replace(/\W+/g, "-")}`;
          return (
            <li key={m.name} className="row">
              <span>
                <span className="cell-title">{m.name}</span>{" "}
                <span className="muted">{plural(units, "unit", "units")}</span>
              </span>
              {mains.length > 0 ? (
                <>
                  <label htmlFor={id} className="sr-only">
                    Main memory for the units of {m.name}
                  </label>
                  <select
                    id={id}
                    className="select-small"
                    value={pick}
                    onChange={(e) =>
                      setTargetOf((x) => ({ ...x, [m.name]: e.target.value }))
                    }
                  >
                    {mains.map((x) => (
                      <option key={x.name} value={x.name}>
                        {x.name}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    className="btn btn-small"
                    disabled={!units || !pick}
                    onClick={async () => {
                      const main = state.memories.find((x) => x.name === pick)!;
                      openMemory(c, main, state.projects);
                      const bytes = tm.exportTMX(c, m.name);
                      const st = tm.importTMX(c, main.name, bytes);
                      const kept = await persistMemory(c, main.name);
                      notify(
                        `Added ${plural(st.stored, "unit", "units")} of ${m.name} to ${main.name}` +
                          (kept
                            ? ""
                            : ". It is kept only until you close this page: this browser does not allow saving"),
                      );
                    }}
                  >
                    Add its units
                  </button>
                </>
              ) : (
                <span className="muted">
                  No main memory for {lang(m.targetLang).name} to add them to.
                </span>
              )}
            </li>
          );
        })}
      </ul>
      <p className="help">
        Units already in the main memory with the same translation are updated,
        not added twice.
      </p>
    </div>
  );
}

/** Runs pre-translation on the saved project for every target language. */
function PretranslateNow({
  c,
  project,
  unsaved,
}: {
  c: CoreApi;
  project: Project;
  unsaved: boolean;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const ready =
    project.prepare.minScore >= 50 && project.prepare.minScore <= 100;
  return (
    <div className="field">
      <div className="row row-start">
        <button
          type="button"
          className="btn"
          disabled={unsaved || !ready}
          onClick={() => {
            let files = project.files;
            let filled = 0;
            let confirmed = 0;
            for (const t of project.targetLangs) {
              const refs = openRefs(
                c,
                memoryRefs(project, t, "lookup", state.memories),
                state.memories,
                state.projects,
              );
              const r = pretranslateFiles(
                c,
                files,
                t,
                refs,
                project.prepare,
                project.context,
              );
              files = r.files;
              filled += r.filled;
              confirmed += r.confirmed;
            }
            if (filled)
              dispatch({
                type: "updateProject",
                change: {
                  project: { ...project, files },
                  addLanguages: [],
                  addMemories: [],
                  removeMemories: [],
                },
              });
            notify(
              filled
                ? `Pre-translated ${plural(filled, "segment", "segments")}, ${confirmed} confirmed`
                : "No empty segment has a match good enough to insert",
            );
          }}
        >
          Pre-translate now
        </button>
      </div>
      <p className="help">
        {unsaved
          ? "Save your changes first; pre-translation uses the saved settings."
          : `Fills the empty segments of every file and language with matches of ${project.prepare.minScore}% or more from the memories set for lookup.`}
      </p>
    </div>
  );
}

/** Saves the project's settings as a template for new projects. */
function SaveAsTemplate({
  project,
  unsaved,
}: {
  project: Project;
  unsaved: boolean;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const [name, setName] = useState(
    project.client ? `${project.client}` : project.name,
  );
  const clash = state.templates.find(
    (t) => t.name.toLowerCase() === name.trim().toLowerCase(),
  );
  return (
    <div className="field">
      <label htmlFor="tpl-from">Template name</label>
      <div className="row">
        <input
          id="tpl-from"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button
          type="button"
          className="btn"
          disabled={!name.trim() || unsaved}
          onClick={() => {
            const now = new Date().toISOString();
            dispatch({
              type: "saveTemplate",
              template: templateOf(
                clash?.id ?? `tpl-${Date.now().toString(36)}`,
                name,
                draftOf(project),
                now,
              ),
            });
            notify(
              clash
                ? `Updated the template “${name.trim()}”`
                : `Saved the template “${name.trim()}”`,
            );
          }}
        >
          {clash ? "Replace template" : "Save as template"}
        </button>
      </div>
      <p className="help">
        {unsaved
          ? "Save your changes first; the template takes the saved settings."
          : "Languages, memories, termbases, checks and pre-translation; not the files, name or due date. New projects can start from it."}
      </p>
    </div>
  );
}
