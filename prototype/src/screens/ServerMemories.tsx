import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Download, Search, Upload } from "lucide-react";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import { ApiError, type CatalogMemory, type Role } from "../server";
import { download } from "../core";
import { coreToRuns } from "../editor/model";
import { fmtDate, fmtNum } from "../format";
import { LangChip } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { AccessPanel } from "./AccessPanel";
import { LANGUAGES } from "../data";

type ServerMemory = CatalogMemory & { createdAt: string; role: Role };

const plain = (runs: ReturnType<typeof coreToRuns>) => runs.filter((r) => typeof r === "string").join("");

/**
 * The memories on the team server the signed-in person may read: their
 * units searched, TMX in and out, access for their managers, new ones for
 * those who may make them.
 */
export function ServerMemories() {
  const { notify } = useStore();
  const r = useRemote();
  const [list, setList] = useState<ServerMemory[] | null>(null);
  const [problem, setProblem] = useState("");
  const [open, setOpen] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      setList(await r.api.memories());
      setProblem("");
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => (e.type === "memory" || e.type === "account") && void load()), [listen, load]);

  if (!r.conn) return null;
  const host = new URL(r.conn.url).host;
  const current = list?.find((m) => m.name === open) ?? null;
  return (
    <section className="section" aria-labelledby="mem-server">
      <h2 id="mem-server" className="section-title">
        On the team server
      </h2>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      {list === null ? (
        <p className="muted" role="status">
          Loading the memories on {host}…
        </p>
      ) : list.length === 0 ? (
        <p className="muted">No memories on {host} for you yet.</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Memory</th>
                <th scope="col">Languages</th>
                <th scope="col" className="num">
                  Units
                </th>
                <th scope="col">You may</th>
                <th scope="col">
                  <span className="sr-only">Open</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {list.map((m) => (
                <tr key={m.name} aria-current={m.name === open ? "true" : undefined}>
                  <th scope="row" className="cell-title">
                    {m.name}
                    <div className="cell-sub">Since {fmtDate(m.createdAt)}</div>
                  </th>
                  <td>
                    <span className="lang-pair">
                      <LangChip code={m.sourceLang} />
                      <span aria-label="to">→</span>
                      <LangChip code={m.targetLang} />
                    </span>
                  </td>
                  <td className="num">{fmtNum(m.units)}</td>
                  <td>{m.role}</td>
                  <td className="cell-action">
                    <button type="button" className="btn btn-quiet btn-small" onClick={() => setOpen(m.name === open ? null : m.name)}>
                      {m.name === open ? "Close" : "Open"}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {current && <ServerMemory key={current.name} memory={current} onChanged={load} onDeleted={async () => {
        setOpen(null);
        await load();
      }} />}
      <NewServerMemory onMade={async (name) => {
        await load();
        setOpen(name);
        notify(`Made the memory ${name} on ${host}`);
      }} />
    </section>
  );
}

function ServerMemory({ memory: m, onChanged, onDeleted }: { memory: ServerMemory; onChanged: () => Promise<void>; onDeleted: () => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const input = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [inTarget, setInTarget] = useState(false);
  const [hits, setHits] = useState<Awaited<ReturnType<typeof api.memoryConcordance>> | null>(null);
  const edit = ["edit", "approve", "manage"].includes(m.role);
  const manage = m.role === "manage";

  async function search(e: FormEvent) {
    e.preventDefault();
    try {
      setHits(await api.memoryConcordance(m.name, query, inTarget));
    } catch (err) {
      notify(errorText(err));
    }
  }

  async function importTMX(f: File) {
    try {
      const res = await api.importTMX(m.name, new Uint8Array(await f.arrayBuffer()));
      notify(`Read ${fmtNum(res.read)} units from ${f.name}; ${fmtNum(res.stored)} stored`);
      await onChanged();
    } catch (err) {
      notify(errorText(err));
    } finally {
      if (input.current) input.current.value = "";
    }
  }

  return (
    <div className="server-detail">
      <section className="panel" aria-labelledby="srvmem-title">
        <h3 id="srvmem-title" className="panel-title">
          {m.name}
        </h3>
        <form className="role-add" onSubmit={search} aria-label={`Search ${m.name}`}>
          <label className="field">
            <span className="field-label">Search the units</span>
            <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} />
          </label>
          <label className="check">
            <input type="checkbox" checked={inTarget} onChange={(e) => setInTarget(e.target.checked)} /> In the translations
          </label>
          <button type="submit" className="btn" disabled={!query.trim()}>
            <Search size={15} aria-hidden="true" /> Search
          </button>
        </form>
        {hits && (
          hits.length === 0 ? (
            <p className="muted">Nothing found.</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">
                    <LangChip code={m.sourceLang} />
                  </th>
                  <th scope="col">
                    <LangChip code={m.targetLang} />
                  </th>
                  <th scope="col">Changed</th>
                </tr>
              </thead>
              <tbody>
                {hits.map((h) => (
                  <tr key={h.unit.id}>
                    <td>{plain(coreToRuns(h.unit.source))}</td>
                    <td>{plain(coreToRuns(h.unit.target))}</td>
                    <td className="cell-sub">
                      {h.unit.changedBy ?? ""}
                      {h.unit.changedAt ? `, ${fmtDate(h.unit.changedAt)}` : ""}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        )}
        <div className="server-actions">
          <button
            type="button"
            className="btn"
            onClick={() => void api.exportMemory(m.name).then((b) => download(b, `${m.name}.tmx`, "application/x-tmx+xml"), (e) => notify(errorText(e)))}
          >
            <Download size={15} aria-hidden="true" /> Export TMX
          </button>
          {edit && (
            <>
              <input ref={input} type="file" accept=".tmx,application/xml" hidden aria-label={`TMX file for ${m.name}`} onChange={(e) => e.target.files?.[0] && void importTMX(e.target.files[0])} />
              <button type="button" className="btn" onClick={() => input.current?.click()}>
                <Upload size={15} aria-hidden="true" /> Import TMX
              </button>
            </>
          )}
          {manage && (
            <ConfirmButton
              small={false}
              label="Delete the memory"
              confirm={`Delete ${m.name} and its ${fmtNum(m.units)} units on the server for everyone?`}
              action="Delete"
              onConfirm={() =>
                void api.deleteMemory(m.name).then(
                  async () => {
                    notify(`Deleted ${m.name} on the server`);
                    await onDeleted();
                  },
                  (e) => notify(e instanceof ApiError && e.code === "inUse" ? e.message : errorText(e)),
                )
              }
            />
          )}
        </div>
      </section>
      {manage && <AccessPanel kind="memory" resource={m.name} title={`Who may use ${m.name}`} />}
    </div>
  );
}

function NewServerMemory({ onMade }: { onMade: (name: string) => Promise<void> }) {
  const r = useRemote();
  const [m, setM] = useState({ name: "", sourceLang: "de-DE", targetLang: "en-GB" });
  const [problem, setProblem] = useState("");
  const [shown, setShown] = useState(false);
  if (!shown)
    return (
      <button type="button" className="btn" onClick={() => setShown(true)}>
        New memory on the server
      </button>
    );
  async function make(e: FormEvent) {
    e.preventDefault();
    setProblem("");
    try {
      const made = await r.api!.createMemory({ ...m, name: m.name.trim() });
      setM({ ...m, name: "" });
      setShown(false);
      await onMade(made.name);
    } catch (err) {
      setProblem(err instanceof ApiError && err.status === 403 ? "You may not make memories on the server. Ask an administrator for the manage role on all memories." : errorText(err));
    }
  }
  return (
    <form className="role-add" onSubmit={make} aria-label="New memory on the server">
      <label className="field">
        <span className="field-label">Name</span>
        <input value={m.name} onChange={(e) => setM({ ...m, name: e.target.value })} />
      </label>
      <label>
        <span className="field-label">From</span>
        <select value={m.sourceLang} onChange={(e) => setM({ ...m, sourceLang: e.target.value })}>
          {LANGUAGES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.name}
            </option>
          ))}
        </select>
      </label>
      <label>
        <span className="field-label">To</span>
        <select value={m.targetLang} onChange={(e) => setM({ ...m, targetLang: e.target.value })}>
          {LANGUAGES.map((l) => (
            <option key={l.code} value={l.code}>
              {l.name}
            </option>
          ))}
        </select>
      </label>
      <button type="submit" className="btn btn-primary" disabled={!m.name.trim()}>
        Make the memory
      </button>
      <button type="button" className="btn btn-quiet" onClick={() => setShown(false)}>
        Cancel
      </button>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
    </form>
  );
}
