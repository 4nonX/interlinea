import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Columns2, Download, Plus, Trash2, Upload } from "lucide-react";
import {
  knownLanguage,
  LANGUAGES,
  lang,
  type LangCode,
  type TranslationMemory,
} from "../data";
import { core, download, importNotice, loadCore, MEMORY_FILES, MEMORY_FILES_NOTE, memoryFileName, tm, WORDFAST_TYPE, type CoreApi } from "../core";
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
import { AlignDocuments, type AlignChoice, type AlignInto } from "./Alignment";
import { LangChip, PageHeader } from "../components/ui";
import { fmtDate, fmtNum, plural } from "../format";
import { tl, tlx } from "../i18n";

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
          tl("The memories could not be loaded: {error}", { error: e instanceof Error ? e.message : String(e) }),
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
        title={tl("Memories")}
        description={tl("Translation memories, in the core. Projects choose which to use for each target language.")}
      />
      <ServerMemories />
      {!c ? (
        <p className="muted" role="status">
          {tl("Loading the memories…")}
        </p>
      ) : (
        <>
          <section className="section" aria-labelledby="mem-main">
            <h2 id="mem-main" className="section-title">
              {tl("Main memories on this computer")}
            </h2>
            <MemoryTable
              key={version}
              c={c}
              memories={main}
              open={open}
              onOpen={toggle}
              onChange={() => setVersion((v) => v + 1)}
            />
            <NewMemoryForm c={c} onCreated={() => setVersion((v) => v + 1)} beside={<AlignHere c={c} memories={main} onAdded={() => setVersion((v) => v + 1)} />} />
          </section>
          {own.length > 0 && (
            <section className="section" aria-labelledby="mem-own">
              <h2 id="mem-own" className="section-title">
                {tl("Project memories")}
              </h2>
              <p className="help">
                {tl("Each collects one project's confirmed translations. Add them to a main memory from the project's settings; they are deleted with the project.")}
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

/** Documents aligned into a memory on this computer, an existing one or one made for them. */
function AlignHere({ c, memories, onAdded }: { c: CoreApi; memories: TranslationMemory[]; onAdded: () => void }) {
  const { state, dispatch, notify } = useLocalStore();
  const [open, setOpen] = useState(false);
  const into = (m: TranslationMemory): AlignInto => ({
    name: m.name,
    sourceLang: m.sourceLang,
    targetLang: m.targetLang,
    add: async (units, overwrite) => {
      openMemory(c, m, state.projects);
      const createdAt = new Date().toISOString();
      tm.add(c, m.name, units.map((u) => ({ ...u, createdBy: state.userName, createdAt })), overwrite);
      if (!(await persistMemory(c, m.name))) notify(tl("It is kept only until you close this page: this browser does not allow saving"));
    },
  });
  const choice: AlignChoice = {
    memories: memories.map(into),
    taken: (name) => state.memories.some((m) => m.name.toLowerCase() === name.toLowerCase()),
    make: async (name, sourceLang, targetLang) => {
      const memory: TranslationMemory = { name, sourceLang, targetLang, createdAt: new Date().toISOString() };
      openNewMemory(c, memory);
      dispatch({ type: "createMemory", memory });
      return into(memory);
    },
  };
  return (
    <>
      <button type="button" className="btn btn-small" onClick={() => setOpen(true)}>
        <Columns2 size={14} aria-hidden="true" />
        {tl("Align documents")}
      </button>
      {open && (
        <AlignDocuments
          choice={choice}
          onClose={() => setOpen(false)}
          onAdded={(n, name) => {
            setOpen(false);
            notify(tl("Added {n} to {name}", { n: plural(n, "unit", "units"), name }));
            onAdded();
          }}
        />
      )}
    </>
  );
}

/** The units of a memory on this computer, kept by the core in the page. */
function LocalMemory({ c, memory: m, onChanged }: { c: CoreApi; memory: TranslationMemory; onChanged: () => void }) {
  const { state, notify } = useLocalStore();
  const backend = useMemo<UnitsBackend>(() => {
    openMemory(c, m, state.projects);
    const saved = async () => {
      if (!(await persistMemory(c, m.name))) notify(tl("It is kept only until you close this page: this browser does not allow saving"));
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
  if (!memories.length) return <p className="muted">{tl("No memory yet.")}</p>;
  return (
    <>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{tl("Memory")}</th>
              <th scope="col">{tl("Languages")}</th>
              <th scope="col" className="num">
                {tl("Units")}
              </th>
              <th scope="col">{tl("Used by")}</th>
              <th scope="col">
                <span className="sr-only">{tl("Actions")}</span>
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
                      {m.client ? tl("{client}, created {createdAt}", { client: m.client, createdAt: fmtDate(m.createdAt) }) : tl("Created {createdAt}", { createdAt: fmtDate(m.createdAt) })}</span>
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
                        ...inTemplates.map((t) => tl("template {name}", { name: t.name })),
                      ].join(", ")
                    ) : (
                      <span className="muted">{tl("No project")}</span>
                    )}
                  </td>
                  <td>
                    <span className="row">
                      <button
                        type="button"
                        className="btn btn-quiet btn-small"
                        aria-label={m.name === open ? tl("Close {name}", { name: m.name }) : tl("Open {name}", { name: m.name })}
                        onClick={() => onOpen(m.name)}
                      >
                        {m.name === open ? tl("Close") : tl("Open")}
                      </button>
                      {!m.projectId && (
                        <button
                          type="button"
                          className="btn btn-small"
                          aria-label={tl("Import a memory file into {name}", { name: m.name })}
                          title={MEMORY_FILES_NOTE}
                          onClick={() => {
                            setImportInto(m.name);
                            input.current?.click();
                          }}
                        >
                          <Upload size={14} aria-hidden="true" />
                          {tl("Import")}
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
                        {tl("Export TMX")}
                      </button>
                      <button
                        type="button"
                        className="btn btn-quiet btn-small"
                        disabled={units === 0}
                        aria-label={tl("Export {name} as a .txt memory", { name: m.name })}
                        onClick={() =>
                          download(
                            tm.exportWordfast(c, m.name),
                            memoryFileName(m.name, "txt"),
                            WORDFAST_TYPE,
                          )
                        }
                      >
                        <Download size={14} aria-hidden="true" />
                        {tl("Export .txt")}
                      </button>
                      {!m.projectId && (
                        <ConfirmButton
                          label={
                            <>
                              <Trash2 size={14} aria-hidden="true" />
                              {tl("Delete")}
                            </>
                          }
                          disabled={users.length > 0 || inTemplates.length > 0}
                          confirm={tl("Delete {name} and its {units}?", { name: m.name, units: plural(units, "unit", "units") })}
                          action={tl("Delete")}
                          onConfirm={() => {
                            void dropMemory(m.name);
                            dispatch({ type: "deleteMemory", name: m.name });
                            notify(tl("Deleted the memory {name}", { name: m.name }));
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
          accept={MEMORY_FILES}
          hidden
          aria-label={tl("Memory file to import")}
          onChange={async (e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (!f || !importInto) return;
            try {
              const st = tm.importFile(
                c,
                importInto,
                new Uint8Array(await f.arrayBuffer()),
              );
              const kept = st.stored === 0 || (await persistMemory(c, importInto));
              notify(
                importNotice(st, importInto, f.name) + (kept ? "" : " " + tl("It is kept only until you close this page: this browser does not allow saving.")),
              );
              onChange();
            } catch (err) {
              notify(
                tl("Could not import {name}: {error}", { name: f.name, error: err instanceof Error ? err.message : String(err) }),
              );
            }
          }}
        />
      </div>
      {memories.some((m) => !m.projectId) && (
        <p className="help">
          {tl("A memory a project or template uses can't be deleted; remove it there first.")}
        </p>
      )}
    </>
  );
}

function NewMemoryForm({
  c,
  onCreated,
  beside,
}: {
  c: CoreApi;
  onCreated: () => void;
  /** What stands next to the button while the form is closed. */
  beside?: ReactNode;
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
    ? tl("Give the memory a name.")
    : taken
      ? tl("A memory with this name exists already.")
      : source === target
        ? tl("Source and target language must differ.")
        : "";
  if (!open)
    return (
      <div className="row">
        <button
          type="button"
          className="btn btn-small"
          onClick={() => setOpen(true)}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("New memory")}
        </button>
        {beside}
      </div>
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
            tl("Could not read {name}: {error}", { name: tmx?.name, error: err instanceof Error ? err.message : String(err) }),
          );
          return;
        }
        dispatch({ type: "createMemory", memory });
        const kept = await persistMemory(c, memory.name);
        notify(
          (units ? tl("Created the memory {name} with {units}.", { name: memory.name, units: plural(units, "unit", "units") }) : tl("Created the memory {name}.", { name: memory.name })) + (kept ? "" : " " + tl("It is kept only until you close this page: this browser does not allow saving.")),
        );
        setOpen(false);
        setName("");
        setTmx(null);
        setError("");
        onCreated();
      }}
    >
      <h3 id="nm-title" className="panel-title">
        {tl("New memory")}
      </h3>
      <div className="field-grid">
        <div className="field">
          <label htmlFor="nm-name">{tl("Name")}</label>
          <input
            id="nm-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-invalid={!!problem && !!name}
          />
        </div>
        <div className="field">
          <label htmlFor="nm-client">
            {tlx("Client {span}", { span: <span className="optional">{tl("optional")}</span> })}</label>
          <input
            id="nm-client"
            value={client}
            onChange={(e) => setClient(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="nm-src">{tl("Source language")}</label>
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
          <label htmlFor="nm-tgt">{tl("Target language")}</label>
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
            {tlx("Fill it from a memory file {span}", { span: <span className="optional">{tl("optional")}</span> })}</label>
          <input
            id="nm-tmx"
            type="file"
            accept={MEMORY_FILES}
            aria-describedby="nm-tmx-help"
            onChange={async (e) => {
              const f = e.target.files?.[0] ?? null;
              setTmx(f);
              setError("");
              if (!f) return;
              // The file's own name and languages, where it gives them, fill
              // what is still as it was.
              try {
                const info = tm.inspectFile(c, new Uint8Array(await f.arrayBuffer()));
                if (!name.trim() && info.name) setName(info.name);
                const src = info.sourceLang && knownLanguage(info.sourceLang);
                const tgt = info.targetLang && knownLanguage(info.targetLang);
                if (src) setSource(src);
                if (tgt) setTarget(tgt);
              } catch (err) {
                setError(tl("Could not read {name}: {error}", { name: f.name, error: err instanceof Error ? err.message : String(err) }));
              }
            }}
          />
          <p id="nm-tmx-help" className="help">
            {MEMORY_FILES_NOTE}
          </p>
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
          {tl("Create memory")}
        </button>
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => setOpen(false)}
        >
          {tl("Cancel")}
        </button>
      </div>
      <p className="help">
        {tlx("{name} to {name2}. Projects with these languages can use it.", { name: lang(source).name, name2: lang(target).name })}
      </p>
    </form>
  );
}
