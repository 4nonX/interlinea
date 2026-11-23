import { useRef, useState, type DragEvent } from "react";
import { FileText, GitMerge, Upload } from "lucide-react";
import { SAMPLE_DOCS } from "../data";
import { useStore, useWords } from "../store";
import { computeChanges } from "../merge";
import { setPendingDoc } from "../draft";
import { readDocument } from "../detect";
import { fmtDate, fmtNum } from "../format";
import { LangChip, LocationBadge, PageHeader } from "../components/ui";
import { go } from "../App";

export function Projects() {
  const { state } = useStore();
  const w = useWords();
  const input = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  async function start(file: File) {
    setPendingDoc(await readDocument(file));
    go({ page: "new" });
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    const f = e.dataTransfer.files[0];
    if (f) void start(f);
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
        onDrop={onDrop}
        aria-labelledby="dz-title"
      >
        <Upload size={22} aria-hidden="true" className="dropzone-icon" />
        <div className="dropzone-text">
          <h2 id="dz-title">Start a project from a document</h2>
          <p>
            Drop a file here. Interlinea detects the source language and
            suggests termbases for the client.
          </p>
        </div>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => input.current?.click()}
        >
          Choose document
        </button>
        <input
          ref={input}
          type="file"
          hidden
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void start(f);
            e.target.value = "";
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
                  setPendingDoc({
                    file: d.file,
                    words: d.words,
                    text: d.text || null,
                    client: d.client,
                  });
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
                      <div className="cell-sub">{p.file}</div>
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
                    <td className="num">{fmtNum(p.words)}</td>
                    <td className="num">{Math.round(p.progress * 100)}%</td>
                    <td>
                      {p.due ? (
                        fmtDate(p.due)
                      ) : (
                        <span className="muted">None</span>
                      )}
                    </td>
                    <td className="cell-action">
                      {changes > 0 && (
                        <a className="btn btn-small" href={`#/merge/${p.id}`}>
                          <GitMerge size={14} aria-hidden="true" />
                          {w("reviewMerge")} ({changes})
                        </a>
                      )}
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
