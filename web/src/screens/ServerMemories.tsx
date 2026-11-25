import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Download, Upload } from "lucide-react";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import { allows, ApiError, type CatalogMemory, type Role, type ServerUnit } from "../server";
import { MemoryUnits, SavedElsewhere, type UnitsBackend } from "./MemoryUnits";
import { AlignDocuments } from "./Alignment";
import { download, importNotice, MEMORY_FILES, MEMORY_FILES_NOTE } from "../core";
import { fmtDate, fmtNum } from "../format";
import { LangChip } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { AccessPanel } from "./AccessPanel";
import { LANGUAGES } from "../data";

type ServerMemory = CatalogMemory & { createdAt: string; role: Role };


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
  const [aligning, setAligning] = useState(false);

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
  const api = r.api;
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
      }} beside={
        <button type="button" className="btn" onClick={() => setAligning(true)}>
          Align documents
        </button>
      } />
      {api && aligning && (
        <AlignDocuments
          choice={{
            memories: (list ?? [])
              .filter((m) => allows(m.role, "edit"))
              .map((m) => ({ name: m.name, sourceLang: m.sourceLang, targetLang: m.targetLang, add: async (units, overwrite) => void (await api.addUnits(m.name, units, overwrite)) })),
            taken: (name) => (list ?? []).some((m) => m.name.toLowerCase() === name.toLowerCase()),
            make: async (name, sourceLang, targetLang) => {
              try {
                const made = await api.createMemory({ name, sourceLang, targetLang });
                return { name: made.name, sourceLang, targetLang, add: async (units, overwrite) => void (await api.addUnits(made.name, units, overwrite)) };
              } catch (err) {
                throw err instanceof ApiError && err.status === 403 ? new Error("You may not make memories on the server. Ask an administrator for the manage role on all memories.") : err;
              }
            },
          }}
          onClose={() => setAligning(false)}
          onAdded={async (n, name) => {
            setAligning(false);
            await load();
            setOpen(name);
            notify(`Added ${n === 1 ? "1 unit" : `${fmtNum(n)} units`} to ${name} on ${host}`);
          }}
        />
      )}
    </section>
  );
}

function ServerMemory({ memory: m, onChanged, onDeleted }: { memory: ServerMemory; onChanged: () => Promise<void>; onDeleted: () => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const input = useRef<HTMLInputElement>(null);
  const edit = allows(m.role, "edit");
  const manage = allows(m.role, "manage");
  const backend = useMemo<UnitsBackend>(
    () => ({
      search: async (query, inTarget) => (await api.memoryConcordance(m.name, query, inTarget)).map((h) => h.unit),
      add: async (source, target, overwrite) => {
        await api.addUnit(m.name, [{ text: source }], [{ text: target }], overwrite);
      },
      addMany: async (units, overwrite) => {
        await api.addUnits(m.name, units, overwrite);
      },
      change: async (u, target) => {
        try {
          await api.updateUnit(m.name, u as ServerUnit, target);
        } catch (e) {
          if (e instanceof ApiError && e.status === 409) throw new SavedElsewhere();
          throw e;
        }
      },
      remove: (u) => api.deleteUnit(m.name, u.id),
    }),
    [api, m.name],
  );

  async function importFile(f: File) {
    try {
      const res = await api.importMemory(m.name, new Uint8Array(await f.arrayBuffer()));
      notify(importNotice(res, m.name, f.name));
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
        <MemoryUnits name={m.name} sourceLang={m.sourceLang} targetLang={m.targetLang} edit={edit} backend={backend} notify={notify} onChanged={onChanged} />
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
              <input ref={input} type="file" accept={MEMORY_FILES} hidden aria-label={`Memory file for ${m.name}`} onChange={(e) => e.target.files?.[0] && void importFile(e.target.files[0])} />
              <button type="button" className="btn" title={MEMORY_FILES_NOTE} aria-label={`Import a memory file into ${m.name}`} onClick={() => input.current?.click()}>
                <Upload size={15} aria-hidden="true" /> Import
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

function NewServerMemory({ onMade, beside }: { onMade: (name: string) => Promise<void>; beside?: ReactNode }) {
  const r = useRemote();
  const [m, setM] = useState({ name: "", sourceLang: "de-DE", targetLang: "en-GB" });
  const [problem, setProblem] = useState("");
  const [shown, setShown] = useState(false);
  if (!shown)
    return (
      <div className="row">
        <button type="button" className="btn" onClick={() => setShown(true)}>
          New memory on the server
        </button>
        {beside}
      </div>
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
