import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { Download, PenLine, Trash2, UserPlus } from "lucide-react";
import { lang } from "../data";
import { useStore } from "../store";
import { errorText, serverNumber, useRemote } from "../remote";
import { allows, ApiError, type Assignment, type Presence, type ServerProject, type Task } from "../server";
import { download } from "../core";
import { fmtDate, plural } from "../format";
import { LangChip, PageHeader } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { fromLocalInput, toLocalInput } from "./NewProject";
import { workScope } from "./Projects";
import { go } from "../App";
import { AccessPanel } from "./AccessPanel";
import { ServerProjectSettings } from "./ServerProjectSettings";
import { tl, tlx } from "../i18n";

const TASK_TEXT: Record<Task, string> = { translate: tl("Translate"), review: tl("Review") };

/**
 * A project on the team server: its details, files, the work given out in
 * it and who is working now. Managers change the details and the work;
 * everyone with work reports it done.
 */
export function ServerProjectPage({ projectId }: { projectId: string }) {
  const { state, notify } = useStore();
  const r = useRemote();
  const n = serverNumber(projectId)!;
  const loaded = state.projects.find((p) => p.id === projectId);
  const [project, setProject] = useState<ServerProject | null>(null);
  const [work, setWork] = useState<Assignment[]>([]);
  const [present, setPresent] = useState<Presence[]>([]);
  const [problem, setProblem] = useState("");

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      const [p, ws, ps] = await Promise.all([r.api.project(n), r.api.assignments(n), r.api.presence(n)]);
      setProject(p);
      setWork(ws);
      setPresent(ps);
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api, n]);

  const listen = r.listen;
  useEffect(() => {
    void load();
    return listen((e) => {
      if ((e.type === "project" && e.change.projectId === n) || (e.type === "presence" && e.projectId === n)) void load();
    });
  }, [load, listen, n]);

  if (problem && !project)
    return (
      <div className="page page-narrow">
        <p className="error" role="alert">
          {problem}
        </p>
      </div>
    );
  if (!project || !r.conn)
    return (
      <p className="boot" role="status">
        {tl("Loading the project…")}
      </p>
    );
  const s = project.settings;
  const manager = allows(project.access.role, "manage");
  const host = new URL(r.conn.url).host;

  return (
    <div className="page page-narrow">
      <PageHeader title={s.name} />
      <p className="help">
        {s.client ? tl("On the team server {host}, for {client}. Made by {createdBy} on {createdAt}.", { host, client: s.client, createdBy: project.createdBy, createdAt: fmtDate(project.createdAt) }) : tl("On the team server {host}. Made by {createdBy} on {createdAt}.", { host, createdBy: project.createdBy, createdAt: fmtDate(project.createdAt) })}
      </p>
      <div className="panel-actions server-project-actions">
        <a className="btn btn-primary" href={`#/editor/${projectId}`}>
          <PenLine size={15} aria-hidden="true" />{" "}{tl("Open in the editor")}
        </a>
      </div>

      <Details project={project} manager={manager} onSaved={load} />

      {manager ? (
        <ServerProjectSettings project={project} onSaved={load} />
      ) : (
      <section className="panel" aria-labelledby="sp-files">
        <h2 id="sp-files" className="panel-title">
          {tl("Files")}
        </h2>
        <ul className="plain-list">
          {s.files.map((f) => (
            <li key={f.name} className="server-file">
              <span className="cell-title">{f.name}</span>
              <span className="cell-sub">
                {f.role === "reference"
                  ? tl("Reference file")
                  : f.targetLang
                    ? tl("Translated into {name} only", { name: lang(f.targetLang).name })
                    : tl("Translated into {join}", { join: s.targetLangs.map((t) => lang(t).name).join(", ") })}
              </span>
              <button
                type="button"
                className="btn btn-quiet btn-small"
                onClick={() =>
                  void r.api!.fileBytes(n, f.name).then(
                    (b) => download(b, f.name.split("/").pop()!, "application/octet-stream"),
                    (e) => notify(errorText(e)),
                  )
                }
              >
                <Download size={14} aria-hidden="true" />{" "}{tl("Original")}
              </button>
            </li>
          ))}
        </ul>
      </section>
      )}

      <Work project={project} work={work} manager={manager} onChanged={load} segmentsOf={(file, l) => {
        const fid = loaded?.remote && Object.entries(loaded.remote.files).find(([, x]) => x === file)?.[0];
        return (fid && loaded?.files.find((f) => f.id === fid)?.segments[l]?.map((x) => x.id)) || [];
      }} />

      <section className="panel" aria-labelledby="sp-now">
        <h2 id="sp-now" className="panel-title">
          {tl("Working now")}
        </h2>
        {present.length === 0 ? (
          <p className="muted">{tl("Nobody has the project open.")}</p>
        ) : (
          <ul className="plain-list">
            {present.map((x) => (
              <li key={x.userId}>
                <span className="cell-title">{x.userName}</span>{" "}
                <span className="cell-sub">
                  <LangChip code={x.targetLang} />{" "}
                  {tl("{file}, segment {segment}, since {since}", { file: x.file, segment: x.segment, since: fmtDate(x.since) })}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {manager && <AccessPanel kind="project" resource={String(n)} title={tl("People with access")} />}

      {manager && (
        <section className="panel" aria-labelledby="sp-delete">
          <h2 id="sp-delete" className="panel-title">
            {tl("Delete the project")}
          </h2>
          <p className="help">
            {tl("The project, its files and translations and the work given out in it are removed from the server for everyone. Its memories and termbase stay; delete them on their own if nothing else uses them.")}
          </p>
          <ConfirmButton
            small={false}
            label={
              <>
                <Trash2 size={15} aria-hidden="true" />{" "}{tl("Delete the project")}
              </>
            }
            confirm={tl("Delete “{name}” on {host} for everyone?", { name: s.name, host })}
            action={tl("Delete")}
            onConfirm={() =>
              void r.api!.deleteProject(n).then(
                () => {
                  notify(tl("Deleted the project “{name}” on the server", { name: s.name }));
                  void r.refresh();
                  go({ page: "projects" });
                },
                (e) => notify(errorText(e)),
              )
            }
          />
        </section>
      )}
    </div>
  );
}

function Details({ project, manager, onSaved }: { project: ServerProject; manager: boolean; onSaved: () => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const s = project.settings;
  const [d, setD] = useState({ name: s.name, client: s.client ?? "", description: s.description ?? "", due: s.due ?? "" });
  const [problems, setProblems] = useState<Record<string, string>>({});
  useEffect(() => {
    setD({ name: s.name, client: s.client ?? "", description: s.description ?? "", due: s.due ?? "" });
  }, [s.name, s.client, s.description, s.due]);
  const changed = d.name !== s.name || d.client !== (s.client ?? "") || d.description !== (s.description ?? "") || d.due !== (s.due ?? "");

  async function save(e: FormEvent) {
    e.preventDefault();
    setProblems({});
    try {
      await r.api!.updateSettings(project.id, { ...s, name: d.name.trim(), client: d.client.trim(), description: d.description.trim(), due: d.due }, project.version);
      await r.open(project.id).catch(() => undefined);
      notify(tl("Saved the project's details on the server"));
      await onSaved();
    } catch (err) {
      if (err instanceof ApiError && err.problems.length) setProblems(Object.fromEntries(err.problems.map((p) => [p.field, p.message])));
      else notify(errorText(err));
    }
  }

  return (
    <section className="panel" aria-labelledby="sp-details">
      <h2 id="sp-details" className="panel-title">
        {tl("Details")}
      </h2>
      <form className="server-form" onSubmit={save}>
        <label className="field">
          <span className="field-label">{tl("Name")}</span>
          <input value={d.name} disabled={!manager} onChange={(e) => setD({ ...d, name: e.target.value })} aria-invalid={!!problems.name} />
          {problems.name && <span className="error">{problems.name}</span>}
        </label>
        <label className="field">
          <span className="field-label">{tl("Client")}</span>
          <input value={d.client} disabled={!manager} onChange={(e) => setD({ ...d, client: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">{tl("Description")}</span>
          <textarea value={d.description} disabled={!manager} rows={2} onChange={(e) => setD({ ...d, description: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">{tl("Due")}</span>
          <input type="datetime-local" value={toLocalInput(d.due)} disabled={!manager} onChange={(e) => setD({ ...d, due: fromLocalInput(e.target.value) })} />
          {problems.due && <span className="error">{problems.due}</span>}
        </label>
        <p className="cell-sub">
          {tlx("{sourceLang} to {join}. Memories, terminology and checks are set when the project is made.", { sourceLang: s.sourceLang, join: s.targetLangs.join(", ") })}
        </p>
        {manager && (
          <div className="server-actions">
            <button type="submit" className="btn btn-primary" disabled={!changed || !d.name.trim()}>
              {tl("Save details")}
            </button>
          </div>
        )}
      </form>
    </section>
  );
}

interface WorkForm {
  userId: string;
  task: Task;
  file: string;
  lang: string;
  from: string;
  to: string;
  due: string;
}

const EMPTY_FORM: WorkForm = { userId: "", task: "translate", file: "", lang: "", from: "", to: "", due: "" };

function Work({
  project,
  work,
  manager,
  onChanged,
  segmentsOf,
}: {
  project: ServerProject;
  work: Assignment[];
  manager: boolean;
  onChanged: () => Promise<void>;
  segmentsOf: (file: string, lang: string) => string[];
}) {
  const { notify } = useStore();
  const r = useRemote();
  const s = project.settings;
  const [f, setF] = useState<WorkForm>(EMPTY_FORM);
  const [problem, setProblem] = useState("");
  const me = r.conn!.user.id;
  const translatable = s.files.filter((x) => x.role === "translatable");
  const fileLangs = useMemo(() => {
    const file = translatable.find((x) => x.name === f.file);
    return file?.targetLang ? [file.targetLang] : s.targetLangs;
  }, [f.file, translatable, s.targetLangs]);
  const segs = f.file && (f.lang || fileLangs.length === 1) ? segmentsOf(f.file, f.lang || fileLangs[0]!) : [];

  async function give(e: FormEvent) {
    e.preventDefault();
    setProblem("");
    try {
      await r.api!.assign(project.id, {
        userId: Number(f.userId),
        task: f.task,
        file: f.file || undefined,
        targetLang: f.lang || undefined,
        fromSegment: f.from || undefined,
        toSegment: f.to || (f.from ? f.from : undefined),
        due: f.due || undefined,
      });
      setF(EMPTY_FORM);
      notify(tl("Work given out"));
      await onChanged();
      await r.refresh();
    } catch (err) {
      setProblem(errorText(err));
    }
  }

  async function setDone(a: Assignment, done: boolean) {
    try {
      await r.api!.updateAssignment(project.id, { ...a, done });
      notify(done ? tl("Reported done") : tl("Reported not done"));
      await onChanged();
      await r.refresh();
    } catch (err) {
      notify(errorText(err));
    }
  }

  return (
    <section className="panel" aria-labelledby="sp-work">
      <h2 id="sp-work" className="panel-title">
        {tl("Work")}
      </h2>
      {work.length === 0 ? (
        <p className="muted">{tl("No work given out yet.")}</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{tl("Who")}</th>
                <th scope="col">{tl("Task")}</th>
                <th scope="col">{tl("Part")}</th>
                <th scope="col">{tl("Done")}</th>
                <th scope="col">
                  <span className="sr-only">{tl("Remove")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {work.map((a) => (
                <tr key={a.id}>
                  <th scope="row" className="cell-title">
                    {a.userName}
                  </th>
                  <td>{TASK_TEXT[a.task]}</td>
                  <td>{workScope(a)}</td>
                  <td>
                    <input
                      type="checkbox"
                      checked={a.done}
                      disabled={!manager && a.userId !== me}
                      onChange={(e) => void setDone(a, e.target.checked)}
                      aria-label={tl("{userName}'s work is done", { userName: a.userName })}
                    />
                  </td>
                  <td className="cell-action">
                    {manager && (
                      <ConfirmButton
                        label={tl("Take back")}
                        confirm={tl("Take this work back from {userName}?", { userName: a.userName })}
                        action={tl("Take back")}
                        onConfirm={() =>
                          void r.api!.unassign(project.id, a.id).then(
                            async () => {
                              await onChanged();
                              await r.refresh();
                            },
                            (e) => notify(errorText(e)),
                          )
                        }
                      />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {manager && (
        <form className="server-form" onSubmit={give} aria-label={tl("Give out work")}>
          <h3 className="field-label">{tl("Give out work")}</h3>
          <label className="field">
            <span className="field-label">{tl("Who")}</span>
            <select value={f.userId} onChange={(e) => setF({ ...f, userId: e.target.value })}>
              <option value="">{tl("Choose someone")}</option>
              {r.people.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({p.username})
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span className="field-label">{tl("Task")}</span>
            <select value={f.task} onChange={(e) => setF({ ...f, task: e.target.value as Task })}>
              <option value="translate">{tl("Translate")}</option>
              <option value="review">{tl("Review")}</option>
            </select>
          </label>
          <label className="field">
            <span className="field-label">{tl("File")}</span>
            <select value={f.file} onChange={(e) => setF({ ...f, file: e.target.value, lang: "", from: "", to: "" })}>
              <option value="">{tl("All files")}</option>
              {translatable.map((x) => (
                <option key={x.name} value={x.name}>
                  {x.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span className="field-label">{tl("Language")}</span>
            <select value={f.lang} onChange={(e) => setF({ ...f, lang: e.target.value, from: "", to: "" })}>
              <option value="">{fileLangs.length === 1 ? lang(fileLangs[0]!).name : tl("All languages")}</option>
              {fileLangs.length > 1 &&
                fileLangs.map((t) => (
                  <option key={t} value={t}>
                    {lang(t).name}
                  </option>
                ))}
            </select>
          </label>
          {f.file && segs.length > 0 && (
            <div className="field-row-inline">
              <label>
                <span className="field-label">{tl("From segment")}</span>
                <select value={f.from} onChange={(e) => setF({ ...f, from: e.target.value, to: e.target.value && !f.to ? segs[segs.length - 1]! : f.to })}>
                  <option value="">{tl("The whole file")}</option>
                  {segs.map((id, i) => (
                    <option key={id} value={id}>
                      {i + 1}
                    </option>
                  ))}
                </select>
              </label>
              {f.from && (
                <label>
                  <span className="field-label">{tl("To segment")}</span>
                  <select value={f.to} onChange={(e) => setF({ ...f, to: e.target.value })}>
                    {segs.slice(segs.indexOf(f.from)).map((id) => (
                      <option key={id} value={id}>
                        {segs.indexOf(id) + 1}
                      </option>
                    ))}
                  </select>
                </label>
              )}
            </div>
          )}
          <label className="field">
            <span className="field-label">{tl("Due (optional)")}</span>
            <input type="datetime-local" value={toLocalInput(f.due)} onChange={(e) => setF({ ...f, due: fromLocalInput(e.target.value) })} />
          </label>
          {problem && (
            <p className="error" role="alert">
              {problem}
            </p>
          )}
          <div className="server-actions">
            <button type="submit" className="btn btn-primary" disabled={!f.userId}>
              <UserPlus size={15} aria-hidden="true" />{" "}{tl("Give out")}
            </button>
          </div>
          <p className="cell-sub">
            {tlx("{length} in this project. Each segment has one translator and one reviewer.", { length: plural(work.length, "piece of work", "pieces of work") })}
          </p>
        </form>
      )}
    </section>
  );
}
