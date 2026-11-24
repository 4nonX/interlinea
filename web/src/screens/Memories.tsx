import { useEffect, useMemo, useRef, useState } from "react";
import { Download, Plus, Trash2, Upload } from "lucide-react";
import {
  LANGUAGES,
  lang,
  type LangCode,
  type TranslationMemory,
} from "../data";
import { core, download, loadCore, tm, type CoreApi } from "../core";
import { ServerMemories } from "./ServerMemories";
import { useLocalStore } from "../store";
import {
  dropMemory,
  openMemory,
  openNewMemory,
  persistMemory,
} from "../editor/memory";
import { ConfirmButton } from "./TermbaseDialogs";
import { MemoryUnits, type UnitsBackend } from "./MemoryUnits";
import { LangChip, PageHeader } from "../components/ui";
import { fmtDate, fmtNum, plural } from "../format";

export function Memories() {
  const { state, notify } = useLocalStore();
  const [c, setC] = useState(() => core());
  const [version, setVersion] = useState(0);
  useEffect(() => {
    if (c) return;
    let live = true;
    loadCore().then(
      (x) => live && setC(x),
      (e: unknown) =>
        live &&
        notify(
          `The memories could not be loaded: ${e instanceof Error ? e.message : String(e)}`,
        ),
    );
    return () => {
      live = false;
    };
  }, [c, notify]);

  const main = state.memories.filter((m) => !m.projectId);
  const own = state.memories.filter((m) => m.projectId);
  // The memory whose units are shown, under both lists.
  const [open, setOpen] = useState<string | null>(null);
  const current = state.memories.find((m) => m.name === open);
  const toggle = (name: string) => setOpen((o) => (o === name ? null : name));

  return (
    <div className="page">
      <PageHeader
        title="Memories"
        description="Translation memories, in the core. Projects choose which to use for each target language."
      />
      <ServerMemories />
      {!c ? (
        <p className="muted" role="status">
          Loading the memories…
        </p>
      ) : (
        <>
          <section className="section" aria-labelledby="mem-main">
            <h2 id="mem-main" className="section-title">
              Main memories on this computer
            </h2>
            <MemoryTable
              key={version}
              c={c}
              memories={main}
              open={open}
              onOpen={toggle}
              onChange={() => setVersion((v) => v + 1)}
            />
            <NewMemoryForm c={c} onCreated={() => setVersion((v) => v + 1)} />
          </section>
          {own.length > 0 && (
            <section className="section" aria-labelledby="mem-own">
              <h2 id="mem-own" className="section-title">
                Project memories
              </h2>
              <p className="help">
                Each collects one project's confirmed translations. Add them to
                a main memory from the project's settings; they are deleted with
                the project.
              </p>
              <MemoryTable
                key={version}
                c={c}
                memories={own}
                open={open}
                onOpen={toggle}
                onChange={() => setVersion((v) => v + 1)}
              />
            </section>
          )}
          {current && <LocalMemory key={current.name} c={c} memory={current} onChanged={() => setVersion((v) => v + 1)} />}
        </>
      )}
    </div>
  );
}

/** The units of a memory on this computer, kept by the core in the page. */
function LocalMemory({ c, memory: m, onChanged }: { c: CoreApi; memory: TranslationMemory; onChanged: () => void }) {
  const { state, notify } = useLocalStore();
  const backend = useMemo<UnitsBackend>(() => {
    openMemory(c, m, state.projects);
    const saved = async () => {
      if (!(await persistMemory(c, m.name))) notify("It is kept only until you close this page: this browser does not allow saving");
    };
    return {
      search: async (query, inTarget) =>
        tm.concordance(c, m.name, query, inTarget, 50).map((u) => ({
          ...u,
          id: u.id ?? 0,
          changedBy: u.changedBy ?? u.createdBy,
          changedAt: u.changedAt ?? u.createdAt,
        })),
      add: async (source, target, overwrite) => {
        tm.add(c, m.name, [{ source: [{ text: source }], target: [{ text: target }], createdBy: state.userName, createdAt: new Date().toISOString() }], overwrite);
        await saved();
      },
      addMany: async (units, overwrite) => {
        const createdAt = new Date().toISOString();
        tm.add(c, m.name, units.map((u) => ({ ...u, createdBy: state.userName, createdAt })), overwrite);
        await saved();
      },
      change: async (u, target) => {
        tm.setTarget(c, m.name, u.id, target, state.userName);
        await saved();
      },
      remove: async (u) => {
        tm.remove(c, m.name, u.id);
        await saved();
      },
    };
    // The projects only say where a project memory starts from.
  }, [c, m.name]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="server-detail">
      <section className="panel" aria-labelledby="localmem-title">
        <h3 id="localmem-title" className="panel-title">
          {m.name}
        </h3>
        <MemoryUnits name={m.name} sourceLang={m.sourceLang} targetLang={m.targetLang} edit backend={backend} notify={notify} onChanged={onChanged} />
      </section>
    </div>
  );
}

function MemoryTable({
  c,
  memories,
  open,
  onOpen,
  onChange,
}: {
  c: CoreApi;
  memories: TranslationMemory[];
  open: string | null;
  onOpen: (name: string) => void;
  onChange: () => void;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const input = useRef<HTMLInputElement>(null);
  const [importInto, setImportInto] = useState("");
  if (!memories.length) return <p className="muted">No memory yet.</p>;
  return (
    <>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th scope="col">Memory</th>
              <th scope="col">Languages</th>
              <th scope="col" className="num">
                Units
              </th>
              <th scope="col">Used by</th>
              <th scope="col">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {memories.map((m) => {
              openMemory(c, m, state.projects);
              const units = tm.count(c, m.name);
              const users = state.projects.filter(
                (p) =>
                  p.id === m.projectId ||
                  Object.values(p.memories).some((ms) =>
                    ms.some((x) => x.memory === m.name),
                  ),
              );
              const inTemplates = state.templates.filter((t) =>
                Object.values(t.memories).some((ms) =>
                  ms.some((x) => x.memory === m.name),
                ),
              );
              return (
                <tr key={m.name}>
                  <th scope="row">
                    <span className="cell-title">{m.name}</span>
                    <span className="cell-sub">
                      {m.client ? `${m.client}, ` : ""}created{" "}
                      {fmtDate(m.createdAt)}
                    </span>
                  </th>
                  <td>
                    <span className="lang-pair">
                      <LangChip code={m.sourceLang} /> →{" "}
                      <LangChip code={m.targetLang} />
                    </span>
                  </td>
                  <td className="num">{fmtNum(units)}</td>
                  <td>
                    {users.length || inTemplates.length ? (
                      [
                        ...users.map((p) => p.name),
                        ...inTemplates.map((t) => `template ${t.name}`),
                      ].join(", ")
                    ) : (
                      <span className="muted">No project</span>
                    )}
                  </td>
                  <td>
                    <span className="row">
                      <button
                        type="button"
                        className="btn btn-quiet btn-small"
                        aria-label={m.name === open ? `Close ${m.name}` : `Open ${m.name}`}
                        onClick={() => onOpen(m.name)}
                      >
                        {m.name === open ? "Close" : "Open"}
                      </button>
                      {!m.projectId && (
                        <button
                          type="button"
                          className="btn btn-small"
                          onClick={() => {
                            setImportInto(m.name);
                            input.current?.click();
                          }}
                        >
                          <Upload size={14} aria-hidden="true" />
                          Import TMX
                        </button>
                      )}
                      <button
                        type="button"
                        className="btn btn-quiet btn-small"
                        disabled={units === 0}
                        onClick={() =>
                          download(
                            tm.exportTMX(c, m.name),
                            `${m.name.replace(/[^\p{L}\p{N}]+/gu, "_")}.tmx`,
                            "application/x-tmx+xml",
                          )
                        }
                      >
                        <Download size={14} aria-hidden="true" />
                        Export TMX
                      </button>
                      {!m.projectId && (
                        <ConfirmButton
                          label={
                            <>
                              <Trash2 size={14} aria-hidden="true" />
                              Delete
                            </>
                          }
                          disabled={users.length > 0 || inTemplates.length > 0}
                          confirm={`Delete ${m.name} and its ${plural(units, "unit", "units")}?`}
                          action="Delete"
                          onConfirm={() => {
                            void dropMemory(m.name);
                            dispatch({ type: "deleteMemory", name: m.name });
                            notify(`Deleted the memory ${m.name}`);
                            onChange();
                          }}
                        />
                      )}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <input
          ref={input}
          type="file"
          accept=".tmx,application/x-tmx+xml,text/xml"
          hidden
          onChange={async (e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (!f || !importInto) return;
            try {
              const st = tm.importTMX(
                c,
                importInto,
                new Uint8Array(await f.arrayBuffer()),
              );
              const kept = await persistMemory(c, importInto);
              notify(
                `Imported ${plural(st.stored, "unit", "units")} into ${importInto}${st.skipped ? `; ${st.skipped} without both languages skipped` : ""}${kept ? "" : ". It is kept only until you close this page: this browser does not allow saving"}`,
              );
              onChange();
            } catch (err) {
              notify(
                `Could not import ${f.name}: ${err instanceof Error ? err.message : String(err)}`,
              );
            }
          }}
        />
      </div>
      {memories.some((m) => !m.projectId) && (
        <p className="help">
          A memory a project or template uses can't be deleted; remove it there
          first.
        </p>
      )}
    </>
  );
}

function NewMemoryForm({
  c,
  onCreated,
}: {
  c: CoreApi;
  onCreated: () => void;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [source, setSource] = useState<LangCode>("de-DE");
  const [target, setTarget] = useState<LangCode>("en-GB");
  const [client, setClient] = useState("");
  const [tmx, setTmx] = useState<File | null>(null);
  const [error, setError] = useState("");
  const taken = state.memories.some(
    (m) => m.name.toLowerCase() === name.trim().toLowerCase(),
  );
  const problem = !name.trim()
    ? "Give the memory a name."
    : taken
      ? "A memory with this name exists already."
      : source === target
        ? "Source and target language must differ."
        : "";
  if (!open)
    return (
      <button
        type="button"
        className="btn btn-small"
        onClick={() => setOpen(true)}
      >
        <Plus size={14} aria-hidden="true" />
        New memory
      </button>
    );
  return (
    <form
      className="panel"
      aria-labelledby="nm-title"
      onSubmit={async (e) => {
        e.preventDefault();
        if (problem) return;
        const memory: TranslationMemory = {
          name: name.trim(),
          sourceLang: source,
          targetLang: target,
          client: client.trim() || undefined,
          createdAt: new Date().toISOString(),
        };
        let units = 0;
        try {
          units = openNewMemory(
            c,
            memory,
            tmx ? new Uint8Array(await tmx.arrayBuffer()) : undefined,
          );
        } catch (err) {
          setError(
            `Could not read ${tmx?.name}: ${err instanceof Error ? err.message : String(err)}`,
          );
          return;
        }
        dispatch({ type: "createMemory", memory });
        const kept = await persistMemory(c, memory.name);
        notify(
          `Created the memory ${memory.name}${units ? ` with ${plural(units, "unit", "units")}` : ""}${kept ? "" : ". It is kept only until you close this page: this browser does not allow saving"}`,
        );
        setOpen(false);
        setName("");
        setTmx(null);
        setError("");
        onCreated();
      }}
    >
      <h3 id="nm-title" className="panel-title">
        New memory
      </h3>
      <div className="field-grid">
        <div className="field">
          <label htmlFor="nm-name">Name</label>
          <input
            id="nm-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-invalid={!!problem && !!name}
          />
        </div>
        <div className="field">
          <label htmlFor="nm-client">
            Client <span className="optional">optional</span>
          </label>
          <input
            id="nm-client"
            value={client}
            onChange={(e) => setClient(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="nm-src">Source language</label>
          <select
            id="nm-src"
            value={source}
            onChange={(e) => setSource(e.target.value)}
          >
            {LANGUAGES.map((l) => (
              <option key={l.code} value={l.code}>
                {l.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="nm-tgt">Target language</label>
          <select
            id="nm-tgt"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            {LANGUAGES.map((l) => (
              <option key={l.code} value={l.code}>
                {l.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="nm-tmx">
            Fill it from a TMX file <span className="optional">optional</span>
          </label>
          <input
            id="nm-tmx"
            type="file"
            accept=".tmx,application/x-tmx+xml,text/xml"
            onChange={(e) => setTmx(e.target.files?.[0] ?? null)}
          />
        </div>
      </div>
      {(problem && name) || error ? (
        <p className="error" role="alert">
          {error || problem}
        </p>
      ) : null}
      <div className="row">
        <button
          type="submit"
          className="btn btn-primary btn-small"
          disabled={!!problem}
        >
          Create memory
        </button>
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => setOpen(false)}
        >
          Cancel
        </button>
      </div>
      <p className="help">
        {lang(source).name} to {lang(target).name}. Projects with these
        languages can use it.
      </p>
    </form>
  );
}
