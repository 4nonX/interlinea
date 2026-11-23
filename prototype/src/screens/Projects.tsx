import { useRef, useState, type DragEvent } from "react";
import {
  FileText,
  FolderOpen,
  GitMerge,
  Settings2,
  Upload,
} from "lucide-react";
import { SAMPLE_DOCS, type Project } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges } from "../merge";
import { setPendingDocs } from "../draft";
import { readFiles } from "../detect";
import { chosenFiles, droppedFiles, type ChosenFile } from "../files";
import { fmtDate, fmtNum, plural } from "../format";
import { LangChip, LocationBadge, PageHeader } from "../components/ui";
import { go } from "../App";

export function Projects() {
  const { state } = useStore();
  const w = useWords();
  const input = useRef<HTMLInputElement>(null);
  const folder = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [reading, setReading] = useState(false);

  async function start(files: ChosenFile[]) {
    if (!files.length) return;
    setReading(true);
    try {
      setPendingDocs(await readFiles(files));
    } finally {
      setReading(false);
    }
    go({ page: "new" });
  }

  async function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    await start(await droppedFiles(e.dataTransfer));
  }

  function pendingChanges(projectId: string): number {
    const p = state.projects.find((x) => x.id === projectId)!;
    if (p.setup.mode !== "layered" || !p.setup.projectTermbaseId) return 0;
    const pe = state.entries.filter(
      (e) => e.termbaseId === p.setup.projectTermbaseId,
    );
    let n = 0;
    for (const mid of p.setup.masters) {
      const m = state.termbases.find((t) => t.id === mid)!;
      n += computeChanges(
        pe,
        m,
        state.entries.filter((e) => e.termbaseId === mid),
        state.baseSnapshots,
        state.branchedAt[p.id] ?? p.createdAt,
      ).length;
    }
    return n;
  }

  return (
    <div className="page">
      <PageHeader title="Projects" />

      <section
        className={`dropzone${dragging ? " is-dragging" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => void onDrop(e)}
        aria-labelledby="dz-title"
      >
        <Upload size={22} aria-hidden="true" className="dropzone-icon" />
        <div className="dropzone-text">
          <h2 id="dz-title">Start a project from files</h2>
          <p>
            {reading
              ? "Reading the files…"
              : "Drop files, a folder or a Trados package here. Interlinea detects the source language and suggests memories and termbases for the client."}
          </p>
        </div>
        <div className="row">
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => input.current?.click()}
            disabled={reading}
          >
            Choose files
          </button>
          <button
            type="button"
            className="btn"
            onClick={() => folder.current?.click()}
            disabled={reading}
          >
            <FolderOpen size={14} aria-hidden="true" />
            Choose a folder
          </button>
          <a className="btn btn-quiet" href="#/new">
            New project without files
          </a>
        </div>
        <input
          ref={input}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            const list = e.target.files ? chosenFiles(e.target.files) : [];
            e.target.value = "";
            void start(list);
          }}
        />
        <input
          ref={folder}
          type="file"
          multiple
          hidden
          {...{ webkitdirectory: "" }}
          onChange={(e) => {
            const list = e.target.files ? chosenFiles(e.target.files) : [];
            e.target.value = "";
            void start(list);
          }}
        />
      </section>

      <div className="samples">
        <span className="samples-label" id="samples-label">
          Or try a sample document:
        </span>
        <ul aria-labelledby="samples-label">
          {SAMPLE_DOCS.map((d) => (
            <li key={d.file}>
              <button
                type="button"
                className="btn btn-quiet btn-small"
                onClick={() => {
                  setPendingDocs([
                    {
                      file: d.file,
                      words: d.words,
                      text: d.text || null,
                      client: d.client,
                    },
                  ]);
                  go({ page: "new" });
                }}
              >
                <FileText size={14} aria-hidden="true" />
                {d.file}
              </button>
            </li>
          ))}
        </ul>
      </div>

      <section aria-labelledby="recent-title" className="section">
        <h2 id="recent-title" className="section-title">
          Recent projects
        </h2>
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Project</th>
                <th scope="col">Languages</th>
                <th scope="col">Terminology</th>
                <th scope="col" className="num">
                  Words
                </th>
                <th scope="col" className="num">
                  Done
                </th>
                <th scope="col">Due</th>
                <th scope="col">
                  <span className="sr-only">{w("projectChanges")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {state.projects.map((p) => {
                const masters = p.setup.masters
                  .map((id) => state.termbases.find((t) => t.id === id)!)
                  .filter(Boolean);
                const changes = pendingChanges(p.id);
                return (
                  <tr key={p.id}>
                    <td>
                      <a
                        className="cell-title cell-link"
                        href={`#/editor/${p.id}`}
                      >
                        {p.name}
                      </a>
                      <div className="cell-sub">{filesLine(p)}</div>
                    </td>
                    <td>
                      <span className="lang-pair">
                        <LangChip code={p.sourceLang} />
                        <span aria-label="to">→</span>
                        {p.targetLangs.map((c) => (
                          <LangChip key={c} code={c} />
                        ))}
                      </span>
                    </td>
                    <td>
                      <div className="cell-title">
                        {p.setup.mode === "direct"
                          ? "Writes to master"
                          : p.setup.mode === "own"
                            ? "Own termbase"
                            : w("projectTermbase")}
                      </div>
                      <div className="cell-sub">
                        {masters.length > 0 ? (
                          <>
                            {p.setup.mode === "direct"
                              ? ""
                              : `${w("basedOn")} `}
                            {masters.map((m) => m.name).join(", ")}
                          </>
                        ) : (
                          "No master"
                        )}{" "}
                        <LocationBadge location={p.setup.location} />
                      </div>
                    </td>
                    <td className="num">
                      {fmtNum(
                        p.files
                          .filter((f) => f.role === "translatable")
                          .reduce((n, f) => n + f.words, 0),
                      )}
                    </td>
                    <td className="num">{Math.round(p.progress * 100)}%</td>
                    <td>
                      {p.due ? (
                        fmtDate(p.due)
                      ) : (
                        <span className="muted">None</span>
                      )}
                    </td>
                    <td className="cell-action">
                      <span className="row">
                        {changes > 0 && (
                          <a className="btn btn-small" href={`#/merge/${p.id}`}>
                            <GitMerge size={14} aria-hidden="true" />
                            {w("reviewMerge")} ({changes})
                          </a>
                        )}
                        <a
                          className="btn btn-quiet btn-small"
                          href={`#/project/${p.id}`}
                          aria-label={`Settings of ${p.name}`}
                        >
                          <Settings2 size={14} aria-hidden="true" />
                          Settings
                        </a>
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function filesLine(p: Project): string {
  const tr = p.files.filter((f) => f.role === "translatable");
  const refs = p.files.length - tr.length;
  const first =
    tr.length === 1 ? tr[0]!.name : plural(tr.length, "file", "files");
  return refs
    ? `${first}, ${plural(refs, "reference file", "reference files")}`
    : first;
}
