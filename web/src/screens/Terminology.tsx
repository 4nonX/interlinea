import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Copy,
  Download,
  GitBranch,
  History,
  Merge,
  Pencil,
  Plus,
  Search,
  Settings2,
  Trash2,
  Upload,
} from "lucide-react";
import {
  lang,
  LANGUAGES,
  primary,
  type Entry,
  type InfoType,
  type LangCode,
  type Termbase,
  type TermStatus,
} from "../data";
import { useStore, useWords } from "../store";
import { fmtTime, plural } from "../format";
import {
  InLang,
  LangChip,
  LocationBadge,
  PageHeader,
  StatusBadge,
} from "../components/ui";
import { go } from "../App";
import {
  ConfirmButton,
  ImportDialog,
  SettingsDialog,
  useCore,
} from "./TermbaseDialogs";
import { fromCoreEntry, LEVEL_TO_CORE, sameTerm, term, toCoreEntry } from "../termio";
import { AddFieldForm, FieldValue } from "./TermbaseFields";
import { LocalDeleted, LocalHistory, LocalMasterDetail, LocalMasterList, useLocalMasters } from "./LocalTermbase";
import { errorText, useRemote } from "../remote";
import { ReloadButton, ServerReviewNote, ServerTermbaseGroup } from "./ServerTerminology";
import {
  may,
  masterKey,
  ReadStoreEntry,
  ServerBranchNote,
  ServerDeleted,
  ServerHistory,
  ServerMasterDetail,
  ServerMasterList,
  ServerTermbaseSettings,
  useMasterEntries,
} from "./ServerTermbase";
import { roleText } from "./Accounts";
import { NewTermbaseDialog } from "./NewTermbase";
import { ExportTermbaseDialog } from "./ExportTermbase";
import { tl, tlx } from "../i18n";

type DialogKind = "import" | "import-new" | "export" | "settings" | null;

/**
 * Termbases: a termbase on the team server (srv:<name>) is opened from
 * there first, with its entries; changes to it are saved there.
 */
export function Terminology({
  termbaseId,
  entryId,
}: {
  termbaseId?: string;
  entryId?: string;
}) {
  const { state } = useStore();
  const r = useRemote();
  const wanted = termbaseId?.startsWith("srv:") ? termbaseId : undefined;
  const loaded = !wanted || state.termbases.some((t) => t.id === wanted);
  const [problem, setProblem] = useState("");
  const openTermbase = r.openTermbase;
  useEffect(() => {
    if (!wanted || !r.conn || loaded) return;
    setProblem("");
    openTermbase(wanted.slice(4)).catch((e: unknown) => setProblem(errorText(e)));
  }, [wanted, r.conn, loaded, openTermbase]);
  if (wanted && !loaded)
    return (
      <div className="page page-narrow">
        <PageHeader title={tl("Terminology")} />
        {!r.conn ? (
          <p>
            {tlx("This termbase is on a team server. Sign in to it in {a}.", { a: <a href="#/settings">{tl("Settings")}</a> })}
          </p>
        ) : problem ? (
          <p className="error" role="alert">
            {problem}
          </p>
        ) : (
          <p className="muted" role="status">
            {tlx("Opening the termbase from {host}…", { host: new URL(r.conn.url).host })}
          </p>
        )}
      </div>
    );
  const tb =
    state.termbases.find((t) => t.id === termbaseId) ??
    state.termbases.find((t) => t.kind === "master" && !t.remote) ??
    state.termbases.find((t) => !t.remote || t.opened);
  if (!tb) return <NoTermbase />;
  return <TerminologyPage tb={tb} entryId={entryId} />;
}

/** The page before there is any termbase: the list to make or import one. */
function NoTermbase() {
  const [importing, setImporting] = useState(false);
  return (
    <div className="page page-wide">
      <PageHeader title={tl("Terminology")} />
      <div className="workspace">
        <TermbaseList current="" onImport={() => setImporting(true)} />
        <section className="pane pane-entries" aria-label={tl("Entries")}>
          <div className="empty">
            <p className="empty-title">{tl("No termbase yet")}</p>
            <p>{tl("Make a master on the left, or import a termbase file.")}</p>
          </div>
        </section>
      </div>
      {importing && <ImportDialog onClose={() => setImporting(false)} />}
    </div>
  );
}

function TerminologyPage({ tb, entryId }: { tb: Termbase; entryId?: string }) {
  const { state } = useStore();
  const entries = useMemo(
    () => state.entries.filter((e) => e.termbaseId === tb.id),
    [state.entries, tb.id],
  );
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<TermStatus | "all">("all");
  const [view, setView] = useState<"entries" | "duplicates" | "deleted" | "master">("entries");
  const [dialog, setDialog] = useState<DialogKind>(null);
  useEffect(() => setView("entries"), [tb.id]);
  // What the signed-in person may do in a termbase on the team server.
  const edit = may(tb, "edit");
  const manage = may(tb, "manage");
  // A project termbase on the server: its masters' entries, in a view of their own.
  const layered = useMasterEntries(tb);
  const [masterSel, setMasterSel] = useState("");
  const masterEntry = layered.masters.find((e) => masterKey(e) === masterSel) ?? layered.masters[0];
  const serverProject = !!tb.remote && tb.kind === "project";
  // The same for a project termbase kept in this browser, over masters kept here.
  const local = useLocalMasters(tb);
  const localProject = !tb.remote && tb.kind === "project" && (tb.basedOn ?? []).length > 0;
  const localMaster = local.masters.find((m) => m.entry.id === masterSel) ?? local.masters[0];
  const layeredProject = serverProject || localProject;
  const masterCount = serverProject ? layered.masters.length : local.masters.length;
  useEffect(() => {
    if ((view === "master" && !layeredProject) || (view === "duplicates" && !edit)) setView("entries");
  }, [view, layeredProject, edit]);

  const filtered = useMemo(() => {
    const q = query.trim().toLocaleLowerCase();
    return entries.filter((e) => {
      const terms = Object.values(e.terms).flat();
      if (status !== "all" && !terms.some((t) => t.status === status))
        return false;
      if (!q) return true;
      return (
        terms.some((t) => t.text.toLocaleLowerCase().includes(q)) ||
        Object.values(e.concept).some((v) => v.toLocaleLowerCase().includes(q))
      );
    });
  }, [entries, query, status]);

  const selected = entries.find((e) => e.id === entryId) ?? filtered[0];

  return (
    <div className="page page-wide">
      <PageHeader title={tl("Terminology")} />
      <div className="workspace">
        <TermbaseList
          current={tb.id}
          onImport={() => setDialog("import-new")}
        />

        <section className="pane pane-entries" aria-labelledby="entries-title">
          <div className="pane-head">
            <div className="pane-head-row">
              <h2 id="entries-title" className="pane-title">
                {tb.name}
              </h2>
              <div
                className="pane-tools"
                role="toolbar"
                aria-label={`${tb.name} tools`}
              >
                {edit && (
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    onClick={() => setDialog("import")}
                  >
                    <Upload size={14} aria-hidden="true" />
                    {tl("Import")}
                  </button>
                )}
                <button
                  type="button"
                  className="btn btn-quiet btn-small"
                  onClick={() => setDialog("export")}
                  disabled={entries.length === 0}
                >
                  <Download size={14} aria-hidden="true" />
                  {tl("Export")}
                </button>
                {tb.remote && <ReloadButton tb={tb} />}
                {manage && (
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    onClick={() => setDialog("settings")}
                  >
                    <Settings2 size={14} aria-hidden="true" />
                    {tl("Settings")}
                  </button>
                )}
              </div>
            </div>
            <div className="pane-sub">
              <LocationBadge location={tb.location} server={tb.server} />
              <span>
                {plural(entries.length, "entry", "entries")}
              </span>
              {tb.languages.length > 0 && (
                <span className="lang-list">
                  {tb.languages.map((l) => (
                    <LangChip key={l} code={l} />
                  ))}
                </span>
              )}
              {tb.remote && !manage && (
                <span className="tag" title={tl("What you may do in this termbase")}>
                  {(tb.role ?? "read").split("+").map(roleText).join(", ")}
                </span>
              )}
            </div>
            {tb.remote ? (
              <>
                <ServerBranchNote tb={tb} />
                <ServerReviewNote tb={tb} />
              </>
            ) : (
              tb.kind === "project" && <ProjectBranchNote tb={tb} />
            )}
          </div>
          <div
            className="tabs tabs-small"
            role="tablist"
            aria-label={tl("Entry lists")}
          >
            <button
              type="button"
              role="tab"
              aria-selected={view === "entries"}
              onClick={() => setView("entries")}
            >
              {tl("Entries")}
            </button>
            {edit && (
              <button
                type="button"
                role="tab"
                aria-selected={view === "duplicates"}
                onClick={() => setView("duplicates")}
                disabled={entries.length < 2}
              >
                <Copy size={14} aria-hidden="true" />
                {tl("Duplicates")}
              </button>
            )}
            <button type="button" role="tab" aria-selected={view === "deleted"} onClick={() => setView("deleted")}>
              {tl("Deleted")}
            </button>
            {layeredProject && (
              <button
                type="button"
                role="tab"
                aria-selected={view === "master"}
                aria-label={tl("The master's entries ({masterCount})", { masterCount })}
                onClick={() => setView("master")}
              >
                {tlx("Master ({masterCount})", { masterCount: masterCount })}
              </button>
            )}
          </div>
          {view === "duplicates" ? (
            <Duplicates tb={tb} entries={entries} />
          ) : view === "deleted" ? (
            tb.remote ? <ServerDeleted tb={tb} /> : <LocalDeleted tb={tb} />
          ) : view === "master" && localProject ? (
            <LocalMasterList tb={tb} masters={local.masters} removals={local.removals} selected={localMaster?.entry.id ?? ""} onSelect={setMasterSel} />
          ) : view === "master" ? (
            <ServerMasterList
              tb={tb}
              masters={layered.masters}
              removals={layered.removals}
              selected={masterEntry ? masterKey(masterEntry) : ""}
              onSelect={setMasterSel}
              reload={layered.reload}
            />
          ) : (
            <>
              <div className="filters">
                <div className="search">
                  <Search size={15} aria-hidden="true" />
                  <label htmlFor="q" className="sr-only">
                    {tl("Search terms and fields")}
                  </label>
                  <input
                    id="q"
                    type="search"
                    placeholder={tl("Search terms and fields")}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                </div>
                <label htmlFor="status" className="sr-only">
                  {tl("Status")}
                </label>
                <select
                  id="status"
                  value={status}
                  onChange={(e) =>
                    setStatus(e.target.value as TermStatus | "all")
                  }
                >
                  <option value="all">{tl("All statuses")}</option>
                  <option value="approved">{tl("Approved")}</option>
                  <option value="proposed">{tl("Proposed")}</option>
                  <option value="forbidden">{tl("Forbidden")}</option>
                </select>
              </div>
              {edit && <NewEntry tb={tb} initial={query} />}
              {filtered.length === 0 ? (
                <div className="empty">
                  {entries.length === 0 && !edit ? (
                    <>
                      <p className="empty-title">{tl("No entries yet")}</p>
                      <p>
                        {layeredProject && masterCount > 0
                          ? tl("The project has no entries of its own. The master's are under Master.")
                          : tl("Those who may edit this termbase add its entries.")}
                      </p>
                    </>
                  ) : entries.length === 0 ? (
                    <>
                      <p className="empty-title">{tl("No entries yet")}</p>
                      <p>
                        {layeredProject && masterCount > 0
                          ? tl("The project has no entries of its own yet: it uses the master's, which are under Master. Add an entry above, or change one of the master's for this project.")
                          : tl("Add an entry above, import a TBX, .sdltb or XLSX file, or add terms while you translate.")}
                      </p>
                      <button
                        type="button"
                        className="btn btn-small"
                        onClick={() => setDialog("import")}
                      >
                        <Upload size={14} aria-hidden="true" />
                        {tl("Import a file")}
                      </button>
                    </>
                  ) : (
                    <>
                      <p className="empty-title">{tlx("Nothing matches “{query}”", { query: query })}</p>
                      <button
                        type="button"
                        className="btn btn-small"
                        onClick={() => {
                          setQuery("");
                          setStatus("all");
                        }}
                      >
                        {tl("Clear search")}
                      </button>
                    </>
                  )}
                </div>
              ) : (
                <ul className="entry-list" aria-label={tl("Entries in {name}", { name: tb.name })}>
                  {filtered.map((e) => (
                    <EntryRow
                      key={e.id}
                      entry={e}
                      tb={tb}
                      selected={selected?.id === e.id}
                    />
                  ))}
                </ul>
              )}
            </>
          )}
        </section>

        {view === "master" && localProject && localMaster ? (
          <LocalMasterDetail
            key={localMaster.entry.id}
            tb={tb}
            item={localMaster}
            onTaken={(id) => {
              setView("entries");
              go({ page: "terminology", termbaseId: tb.id, entryId: id });
            }}
          />
        ) : view === "master" && masterEntry ? (
          <ServerMasterDetail key={masterKey(masterEntry)} tb={tb} entry={masterEntry} reload={layered.reload} onTaken={() => setView("entries")} />
        ) : selected && view !== "master" ? (
          <EntryDetail key={selected.id} entry={selected} tb={tb} />
        ) : (
          <section className="pane pane-detail" aria-label={tl("Entry")} />
        )}
      </div>

      {dialog === "import" && (
        <ImportDialog tb={tb} onClose={() => setDialog(null)} />
      )}
      {dialog === "import-new" && (
        <ImportDialog onClose={() => setDialog(null)} />
      )}
      {dialog === "export" && (
        <ExportTermbaseDialog
          tb={tb}
          entries={entries}
          shown={filtered}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog === "settings" &&
        (tb.remote ? (
          <ServerTermbaseSettings tb={tb} entries={entries} onClose={() => setDialog(null)} />
        ) : (
          <SettingsDialog tb={tb} onClose={() => setDialog(null)} />
        ))}
    </div>
  );
}

function TermbaseList({
  current,
  onImport,
}: {
  current: string;
  onImport: () => void;
}) {
  const { state } = useStore();
  const w = useWords();
  const r = useRemote();
  const [creating, setCreating] = useState<"master" | "project" | null>(null);
  const groups: [string, Termbase[]][] = [
    [tl("Masters"), state.termbases.filter((t) => t.kind === "master" && !t.remote)],
    [
      `${w("projectTermbase")}s`,
      state.termbases.filter((t) => t.kind === "project" && !t.remote),
    ],
  ];
  return (
    <nav className="pane pane-tbs" aria-label={tl("Termbases")}>
      {groups.filter(([, list]) => list.length > 0).map(([label, list]) => (
        <div key={label} className="tb-group">
          <h2 className="tb-group-title">{label}</h2>
          <ul>
            {list.map((t) => {
              const n = state.entries.filter(
                (e) => e.termbaseId === t.id,
              ).length;
              return (
                <li key={t.id}>
                  <a
                    href={`#/terminology/${t.id}`}
                    aria-current={t.id === current ? "page" : undefined}
                    className="tb-link"
                  >
                    <span className="tb-name">
                      {t.name.replace(/ \(project\)$/, "")}
                    </span>
                    <span className="tb-meta">
                      <LocationBadge location={t.location} server={t.server} />
                      <span className="tb-count">
                        {n.toLocaleString("en-GB")}
                      </span>
                    </span>
                  </a>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
      <ServerTermbaseGroup current={current} />
      {creating && <NewTermbaseDialog kind={creating} onClose={() => setCreating(null)} />}
        <div className="tb-actions">
          <button
            type="button"
            className="btn btn-quiet btn-small tb-new"
            onClick={() => setCreating("master")}
          >
            <Plus size={14} aria-hidden="true" />
            {tl("New master")}
          </button>
          {r.api && (
            <button type="button" className="btn btn-quiet btn-small tb-new" onClick={() => setCreating("project")}>
              <GitBranch size={14} aria-hidden="true" />
              {tl("New project termbase")}
            </button>
          )}
          <button
            type="button"
            className="btn btn-quiet btn-small tb-new"
            onClick={onImport}
          >
            <Upload size={14} aria-hidden="true" />
            {tl("Import termbase")}
          </button>
        </div>
    </nav>
  );
}

function ProjectBranchNote({ tb }: { tb: Termbase }) {
  const { state } = useStore();
  const w = useWords();
  const masters = (tb.basedOn ?? [])
    .map((id) => state.termbases.find((t) => t.id === id)!)
    .filter(Boolean);
  if (!masters.length) return null;
  return (
    <p className="branch-note">
      <GitBranch size={14} aria-hidden="true" />
      <span>
        {tlx("{w} {join}. Shows only this project's new and changed entries.", { w: w("basedOn"), join: masters.map((m) => m.name).join(", ") })}
      </span>
      {tb.projectId && (
        <a href={`#/merge/${tb.projectId}`} className="link">
          {w("reviewMerge")}
        </a>
      )}
    </p>
  );
}

function sourceLangOf(tb: Termbase): string {
  return tb.languages[0] ?? "de-DE";
}

function headTerm(
  entry: Entry,
  tb: Termbase,
): { code: LangCode; text: string } | undefined {
  const src = sourceLangOf(tb);
  const t = entry.terms[src]?.[0];
  if (t) return { code: src, text: t.text };
  for (const [code, ts] of Object.entries(entry.terms))
    if (ts[0]) return { code, text: ts[0].text };
  return undefined;
}

function NewEntry({ tb, initial }: { tb: Termbase; initial: string }) {
  const { state, dispatch, notify } = useStore();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [code, setCode] = useState<LangCode>(sourceLangOf(tb));
  const [st, setSt] = useState<TermStatus>("proposed");
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  useEffect(() => setCode(sourceLangOf(tb)), [tb]);
  if (!open)
    return (
      <div className="new-entry-bar">
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => {
            setText(initial.trim());
            setOpen(true);
          }}
        >
          <Plus size={14} aria-hidden="true" />
          {tl("New entry")}
        </button>
      </div>
    );
  const langs = tb.languages.length
    ? tb.languages
    : LANGUAGES.map((l) => l.code);
  return (
    <form
      className="new-entry"
      onSubmit={(e) => {
        e.preventDefault();
        const v = text.trim();
        if (!v) return;
        const now = new Date().toISOString();
        const id = `e-${Date.now().toString(36)}`;
        dispatch({
          type: "createEntry",
          entry: {
            id,
            termbaseId: tb.id,
            version: 1,
            concept: {},
            terms: { [code]: [{ text: v, status: st }] },
            history: [{ id: `c-${id}`, at: now, by: state.userName, summary: tl("Created") }],
          },
        });
        if (!tb.languages.includes(code))
          dispatch({
            type: "updateTermbase",
            termbaseId: tb.id,
            patch: { languages: [...tb.languages, code] },
          });
        notify(tl("Added “{v}”", { v }));
        setOpen(false);
        setText("");
        go({ page: "terminology", termbaseId: tb.id, entryId: id });
      }}
    >
      <label htmlFor="ne-text">{tl("First term")}</label>
      <div className="row row-start">
        <input
          id="ne-text"
          ref={ref}
          value={text}
          onChange={(e) => setText(e.target.value)}
          lang={primary(code)}
        />
        <label htmlFor="ne-lang" className="sr-only">
          {tl("Language")}
        </label>
        <select
          id="ne-lang"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        >
          {langs.map((l) => (
            <option key={l} value={l}>
              {lang(l).name}
            </option>
          ))}
        </select>
        <label htmlFor="ne-status" className="sr-only">
          {tl("Status")}
        </label>
        <select
          id="ne-status"
          value={st}
          onChange={(e) => setSt(e.target.value as TermStatus)}
        >
          <option value="approved">{tl("Approved")}</option>
          <option value="proposed">{tl("Proposed")}</option>
          <option value="forbidden">{tl("Forbidden")}</option>
        </select>
      </div>
      <p className="help">{tl("Add the other languages and fields in the entry.")}</p>
      <div className="row">
        <button
          type="button"
          className="btn btn-quiet btn-small"
          onClick={() => setOpen(false)}
        >
          {tl("Cancel")}
        </button>
        <button
          type="submit"
          className="btn btn-primary btn-small"
          disabled={!text.trim()}
        >
          {tl("Add entry")}
        </button>
      </div>
    </form>
  );
}

function EntryRow({
  entry,
  tb,
  selected,
}: {
  entry: Entry;
  tb: Termbase;
  selected: boolean;
}) {
  const src = sourceLangOf(tb);
  const srcTerms = entry.terms[src] ?? [];
  const others = tb.languages.filter((l) => l !== src);
  const head = headTerm(entry, tb);
  return (
    <li>
      <a
        href={`#/terminology/${tb.id}/${entry.id}`}
        className="entry-row"
        aria-current={selected ? "true" : undefined}
      >
        <span className="entry-head">
          {srcTerms.length ? (
            <InLang code={src} className="entry-term">
              {srcTerms[0]!.text}
            </InLang>
          ) : head ? (
            <InLang code={head.code} className="entry-term">
              {head.text}
            </InLang>
          ) : (
            <span className="missing">{tl("No term")}</span>
          )}
          {srcTerms.length > 1 && (
            <span className="muted"> +{srcTerms.length - 1}</span>
          )}
        </span>
        <span className="entry-targets">
          {others.map((l) => {
            const ts = entry.terms[l] ?? [];
            return (
              <span key={l} className="entry-target">
                <LangChip code={l} />
                {ts.length ? (
                  <InLang code={l}>
                    {ts
                      .filter((t) => t.status !== "forbidden")
                      .map((t) => t.text)
                      .join(", ") || ts[0]!.text}
                  </InLang>
                ) : (
                  <span className="missing">{tl("missing")}</span>
                )}
              </span>
            );
          })}
        </span>
        {entry.concept.domain && (
          <span className="entry-domain">{entry.concept.domain}</span>
        )}
      </a>
    </li>
  );
}

function EntryDetail({ entry, tb }: { entry: Entry; tb: Termbase }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const [tab, setTab] = useState<"entry" | "history">("entry");
  const conceptTypes = tb.infoTypes.filter((i) => i.level === "concept");
  const langTypes = tb.infoTypes.filter((i) => i.level === "language");
  const termTypes = tb.infoTypes.filter((i) => i.level === "term");
  const languages = [
    ...new Set([...tb.languages, ...Object.keys(entry.terms)]),
  ];
  const base = entry.base
    ? state.termbases.find((t) => t.id === entry.base!.termbaseId)
    : undefined;
  const now = () => new Date().toISOString();
  const latest = entry.history[entry.history.length - 1];
  const head = headTerm(entry, tb);

  const saveTerms = (
    code: LangCode,
    list: Entry["terms"][string],
    summary: string,
  ) => {
    const terms = { ...entry.terms };
    if (list.length) terms[code] = list;
    else delete terms[code];
    dispatch({
      type: "saveEntry",
      entryId: entry.id,
      content: { concept: entry.concept, terms, langFields: entry.langFields },
      summary,
      now: now(),
    });
    if (list.length && !tb.languages.includes(code))
      dispatch({
        type: "updateTermbase",
        termbaseId: tb.id,
        patch: { languages: [...tb.languages, code] },
      });
  };
  const saveLangField = (
    code: LangCode,
    field: string,
    value: string,
    label: string,
  ) => {
    const langFields = {
      ...entry.langFields,
      [code]: { ...entry.langFields?.[code], [field]: value },
    };
    dispatch({
      type: "saveEntry",
      entryId: entry.id,
      content: { concept: entry.concept, terms: entry.terms, langFields },
      summary: value
        ? `Set ${label} (${lang(code).name})`
        : `Cleared ${label} (${lang(code).name})`,
      now: now(),
    });
  };

  return (
    <section className="pane pane-detail" aria-labelledby="detail-title">
      <div className="pane-head">
        <div className="pane-head-row">
          <h2 id="detail-title" className="pane-title">
            {head ? <InLang code={head.code}>{head.text}</InLang> : tl("Entry")}
          </h2>
          {may(tb, "edit") && <ConfirmButton
            label={
              <>
                <Trash2 size={14} aria-hidden="true" />
                {tl("Delete entry")}
              </>
            }
            confirm={tb.remote ? tl("Delete this entry? It can be brought back from Deleted, without its history.") : tl("Delete this entry? It can be brought back from Deleted.")}
            action={tl("Delete")}
            onConfirm={() => {
              dispatch({ type: "deleteEntries", entryIds: [entry.id] });
              notify(tl("Deleted the entry{x}", { x: head ? ` “${head.text}”` : "" }));
              go({ page: "terminology", termbaseId: tb.id });
            }}
          />}
        </div>
        <div className="pane-sub">
          {latest && (
            <span>
              {tlx("Last changed {at} by {by}", { at: fmtTime(latest.at), by: latest.by })}</span>
          )}
          {base && (
            <span>
              {w("basedOn")} {base.name}
            </span>
          )}
          {entry.externalId && (
            <span className="muted">
              {tlx("ID in the imported file: {externalId}", { externalId: entry.externalId })}</span>
          )}
          {tb.kind === "project" && !entry.base && (
            <span className="tag">{tl("Not in a master yet")}</span>
          )}
        </div>
      </div>

      <div className="tabs" role="tablist" aria-label={tl("Entry views")}>
        <button
          type="button"
          role="tab"
          id="tab-entry"
          aria-selected={tab === "entry"}
          aria-controls="panel-entry"
          onClick={() => setTab("entry")}
        >
          {tl("Entry")}
        </button>
        <button
          type="button"
          role="tab"
          id="tab-history"
          aria-selected={tab === "history"}
          aria-controls="panel-history"
          onClick={() => setTab("history")}
        >
          <History size={14} aria-hidden="true" />
          {w("history")} ({tb.remote ? Math.max(1, entry.version ?? 1) : entry.history.length})
        </button>
      </div>

      {tab === "entry" && !may(tb, "edit") ? (
        <div role="tabpanel" id="panel-entry" aria-labelledby="tab-entry" className="detail-body">
          <ReadStoreEntry tb={tb} entry={entry} />
        </div>
      ) : tab === "history" && tb.remote ? (
        <div role="tabpanel" id="panel-history" aria-labelledby="tab-history" className="detail-body">
          <ServerHistory tb={tb} entry={entry} />
        </div>
      ) : tab === "entry" ? (
        <div
          role="tabpanel"
          id="panel-entry"
          aria-labelledby="tab-entry"
          className="detail-body"
        >
          <dl className="fields">
            {conceptTypes.map((it) => (
              <FieldRow
                key={it.id}
                it={it}
                value={entry.concept[it.id] ?? ""}
                onSave={(v) =>
                  dispatch({
                    type: "setField",
                    entryId: entry.id,
                    field: it.id,
                    value: v,
                    now: now(),
                  })
                }
              />
            ))}
          </dl>
          <AddInfoType tb={tb} />

          {languages.map((code) => {
            const terms = entry.terms[code] ?? [];
            return (
              <section
                key={code}
                className="lang-block"
                aria-label={lang(code).name}
              >
                <h3 className="lang-title">
                  {lang(code).name}
                  <LangChip code={code} />
                </h3>
                {langTypes.length > 0 && (
                  <dl className="fields fields-term">
                    {langTypes.map((it) => (
                      <FieldRow
                        key={it.id}
                        it={it}
                        value={entry.langFields?.[code]?.[it.id] ?? ""}
                        onSave={(v) => saveLangField(code, it.id, v, it.name)}
                      />
                    ))}
                  </dl>
                )}
                {terms.length === 0 ? (
                  <p className="missing">{tl("No term yet")}</p>
                ) : (
                  <ul className="terms">
                    {terms.map((t, i) => (
                      <TermItem
                        key={`${t.text}-${i}`}
                        code={code}
                        text={t.text}
                        status={t.status}
                        others={terms
                          .filter((_, j) => j !== i)
                          .map((x) => x.text)}
                        onStatus={(s) =>
                          saveTerms(
                            code,
                            terms.map((x, j) =>
                              j === i ? { ...x, status: s } : x,
                            ),
                            tl("Set “{text}” to {s}", { text: t.text, s }),
                          )
                        }
                        onRename={(v) =>
                          saveTerms(
                            code,
                            terms.map((x, j) =>
                              j === i ? { ...x, text: v } : x,
                            ),
                            tl("Changed “{text}” to “{v}”", { text: t.text, v }),
                          )
                        }
                        onRemove={() => {
                          saveTerms(
                            code,
                            terms.filter((_, j) => j !== i),
                            tl("Removed “{text}”", { text: t.text }),
                          );
                          notify(tl("Removed “{text}”", { text: t.text }));
                        }}
                      >
                        {termTypes.length > 0 && (
                          <dl className="fields fields-term">
                            {termTypes.map((it) => (
                              <FieldRow
                                key={it.id}
                                it={it}
                                value={t.fields?.[it.id] ?? ""}
                                onSave={(v) =>
                                  dispatch({
                                    type: "setTermField",
                                    entryId: entry.id,
                                    langCode: code,
                                    termIndex: i,
                                    field: it.id,
                                    value: v,
                                    now: now(),
                                  })
                                }
                              />
                            ))}
                          </dl>
                        )}
                      </TermItem>
                    ))}
                  </ul>
                )}
                <AddTerm
                  code={code}
                  existing={terms.map((t) => t.text)}
                  onAdd={(text, s) =>
                    saveTerms(
                      code,
                      [...terms, { text, status: s }],
                      tl("Added “{text}”", { text }),
                    )
                  }
                />
              </section>
            );
          })}
          <AddLanguage
            tb={tb}
            used={languages}
            onAdd={(code) =>
              dispatch({
                type: "updateTermbase",
                termbaseId: tb.id,
                patch: { languages: [...tb.languages, code] },
              })
            }
          />

          {entry.history.find((c) => c.segment) && (
            <section className="context" aria-label={tl("Context")}>
              <h3 className="lang-title">{tl("Found in")}</h3>
              {entry.history
                .filter((c) => c.segment)
                .map((c) => (
                  <figure key={c.id} className="context-item">
                    <blockquote lang={primary(sourceLangOf(tb))}>
                      {c.segment}
                    </blockquote>
                    <figcaption>
                      {c.project}, {fmtTime(c.at)}
                    </figcaption>
                  </figure>
                ))}
            </section>
          )}
        </div>
      ) : (
        <div
          role="tabpanel"
          id="panel-history"
          aria-labelledby="tab-history"
          className="detail-body"
        >
          <LocalHistory tb={tb} entry={entry} />
        </div>
      )}
    </section>
  );
}

function TermItem({
  code,
  text,
  status,
  others,
  onStatus,
  onRename,
  onRemove,
  children,
}: {
  code: LangCode;
  text: string;
  status: TermStatus;
  others: string[];
  onStatus: (s: TermStatus) => void;
  onRename: (v: string) => void;
  onRemove: () => void;
  children?: ReactNode;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(text);
  const id = useId();
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (editing) ref.current?.select();
  }, [editing]);
  const dup = others.some((o) => sameTerm(o, draft));
  return (
    <li className="term">
      {editing ? (
        <form
          className="term-edit"
          onSubmit={(e) => {
            e.preventDefault();
            const v = draft.trim();
            if (!v || dup) return;
            if (v !== text) onRename(v);
            setEditing(false);
          }}
        >
          <label htmlFor={`${id}-t`} className="sr-only">
            {tl("Term")}
          </label>
          <input
            id={`${id}-t`}
            ref={ref}
            value={draft}
            lang={primary(code)}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) =>
              e.key === "Escape" && (setDraft(text), setEditing(false))
            }
            aria-invalid={dup}
          />
          <button
            type="submit"
            className="btn btn-primary btn-small"
            disabled={!draft.trim() || dup}
          >
            {tl("Save")}
          </button>
          <button
            type="button"
            className="btn btn-quiet btn-small"
            onClick={() => {
              setDraft(text);
              setEditing(false);
            }}
          >
            {tl("Cancel")}
          </button>
          {dup && (
            <p className="error">
              {tlx("This entry already has “{trim}” in {name}.", { trim: draft.trim(), name: lang(code).name })}
            </p>
          )}
        </form>
      ) : (
        <div className="term-line">
          <InLang code={code} className="term-text">
            {text}
          </InLang>
          <StatusBadge status={status} />
          <span className="term-tools">
            <label htmlFor={`${id}-s`} className="sr-only">
              {tlx("Status of {text}", { text: text })}</label>
            <select
              id={`${id}-s`}
              className="select-small"
              value={status}
              onChange={(e) => onStatus(e.target.value as TermStatus)}
            >
              <option value="approved">{tl("Approved")}</option>
              <option value="proposed">{tl("Proposed")}</option>
              <option value="forbidden">{tl("Forbidden")}</option>
            </select>
            <button
              type="button"
              className="icon-button"
              onClick={() => setEditing(true)}
              aria-label={tl("Edit {text}", { text })}
            >
              <Pencil size={14} aria-hidden="true" />
            </button>
            <ConfirmButton
              label={
                <>
                  <Trash2 size={14} aria-hidden="true" />
                  <span className="sr-only">{tl("Remove")}{" "}{text}</span>
                </>
              }
              confirm={tl("Remove “{text}”?", { text })}
              onConfirm={onRemove}
            />
          </span>
        </div>
      )}
      {children}
    </li>
  );
}

function AddTerm({
  code,
  existing,
  onAdd,
}: {
  code: LangCode;
  existing: string[];
  onAdd: (text: string, s: TermStatus) => void;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [st, setSt] = useState<TermStatus>("proposed");
  const id = useId();
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  const dup = existing.some((o) => sameTerm(o, text));
  if (!open)
    return (
      <button
        type="button"
        className="btn btn-quiet btn-tiny"
        onClick={() => setOpen(true)}
      >
        <Plus size={13} aria-hidden="true" />
        {existing.length ? tl("Add a synonym") : tl("Add a term")}
      </button>
    );
  return (
    <form
      className="term-edit"
      onSubmit={(e) => {
        e.preventDefault();
        const v = text.trim();
        if (!v || dup) return;
        onAdd(v, st);
        setText("");
        setOpen(false);
      }}
    >
      <label htmlFor={`${id}-t`} className="sr-only">
        {tlx("New term in {name}", { name: lang(code).name })}</label>
      <input
        id={`${id}-t`}
        ref={ref}
        value={text}
        lang={primary(code)}
        placeholder={tl("Term in {name}", { name: lang(code).name })}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => e.key === "Escape" && setOpen(false)}
        aria-invalid={dup}
      />
      <label htmlFor={`${id}-s`} className="sr-only">
        {tl("Status")}
      </label>
      <select
        id={`${id}-s`}
        className="select-small"
        value={st}
        onChange={(e) => setSt(e.target.value as TermStatus)}
      >
        <option value="approved">{tl("Approved")}</option>
        <option value="proposed">{tl("Proposed")}</option>
        <option value="forbidden">{tl("Forbidden")}</option>
      </select>
      <button
        type="submit"
        className="btn btn-primary btn-small"
        disabled={!text.trim() || dup}
      >
        {tl("Add")}
      </button>
      <button
        type="button"
        className="btn btn-quiet btn-small"
        onClick={() => setOpen(false)}
      >
        {tl("Cancel")}
      </button>
      {dup && <p className="error">{tlx("This entry already has “{trim}”.", { trim: text.trim() })}</p>}
    </form>
  );
}

function AddLanguage({
  tb,
  used,
  onAdd,
}: {
  tb: Termbase;
  used: LangCode[];
  onAdd: (code: LangCode) => void;
}) {
  const options = LANGUAGES.filter((l) => !used.includes(l.code));
  if (!options.length) return null;
  return (
    <div className="add-lang">
      <label htmlFor="ed-add-lang" className="sr-only">
        {tlx("Add a language to {name}", { name: tb.name })}</label>
      <select
        id="ed-add-lang"
        value=""
        onChange={(e) => e.target.value && onAdd(e.target.value)}
      >
        <option value="">{tl("Add a language to the termbase…")}</option>
        {options.map((l) => (
          <option key={l.code} value={l.code}>
            {l.name}
          </option>
        ))}
      </select>
    </div>
  );
}

/**
 * Entries that share a term, as MultiTerm's duplicate filter and memoQ's
 * "find duplicates" show them, with a merge into one entry.
 */
function Duplicates({ tb, entries }: { tb: Termbase; entries: Entry[] }) {
  const { dispatch, notify } = useStore();
  const c = useCore();
  const [code, setCode] = useState<string>("");
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const core = useMemo(
    () => entries.map((e) => toCoreEntry(e, tb)),
    [entries, tb],
  );
  const groups = useMemo(
    () => (c ? term.duplicates(c, core, code) : []),
    [c, core, code],
  );
  const visible = groups.filter(
    (g) => !dismissed.has(g.map((i) => entries[i]!.id).join("|")),
  );
  const ids = useMemo(
    () => new Map(tb.infoTypes.map((i) => [i.name, i.id])),
    [tb.infoTypes],
  );
  return (
    <div className="duplicates">
      <div className="filters">
        <label htmlFor="dup-lang" className="inline-label">
          {tl("Shared term in")}
        </label>
        <select
          id="dup-lang"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        >
          <option value="">{tl("Any language")}</option>
          {tb.languages.map((l) => (
            <option key={l} value={l}>
              {lang(l).name}
            </option>
          ))}
        </select>
      </div>
      {!c ? (
        <p className="help">{tl("Loading the core…")}</p>
      ) : visible.length === 0 ? (
        <div className="empty">
          <p className="empty-title">{tl("No duplicates")}</p>
          <p>
            {code ? tl("No two entries share a term in {language}.", { language: lang(code).name }) : tl("No two entries share a term.")}
          </p>
        </div>
      ) : (
        <ul className="dup-list" aria-label={tl("Groups of duplicate entries")}>
          {visible.map((g) => {
            const group = g.map((i) => entries[i]!);
            const key = group.map((e) => e.id).join("|");
            // The oldest entry is kept, so links to it stay valid.
            const ordered = [...group].sort((a, b) =>
              (a.history[0]?.at ?? "").localeCompare(b.history[0]?.at ?? ""),
            );
            return (
              <li key={key} className="dup-group">
                <ul className="dup-entries">
                  {ordered.map((e) => (
                    <li key={e.id}>
                      <a
                        href={`#/terminology/${tb.id}/${e.id}`}
                        className="dup-entry"
                      >
                        {Object.entries(e.terms).map(([l, ts]) => (
                          <span key={l} className="entry-target">
                            <LangChip code={l} />
                            <InLang code={l}>
                              {ts.map((t) => t.text).join(", ")}
                            </InLang>
                          </span>
                        ))}
                      </a>
                    </li>
                  ))}
                </ul>
                <div className="row row-start">
                  <button
                    type="button"
                    className="btn btn-small"
                    onClick={() => {
                      const [keep, ...rest] = ordered;
                      let merged = toCoreEntry(keep!, tb);
                      for (const r of rest)
                        merged = term.merge(c, merged, toCoreEntry(r, tb));
                      dispatch({
                        type: "mergeEntries",
                        keepId: keep!.id,
                        removeIds: rest.map((r) => r.id),
                        content: fromCoreEntry(merged, ids, "proposed"),
                        summary: tl("Merged with {x}", { x: plural(rest.length, "duplicate entry", "duplicate entries") }),
                        now: new Date().toISOString(),
                      });
                      notify(tl("Merged {length} entries into one", { length: group.length }));
                    }}
                  >
                    <Merge size={14} aria-hidden="true" />
                    {tl("Merge into one entry")}
                  </button>
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    onClick={() => setDismissed((d) => new Set([...d, key]))}
                  >
                    {tl("Not duplicates")}
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function FieldRow({
  it,
  value,
  onSave,
}: {
  it: InfoType;
  value: string;
  onSave: (v: string) => void;
}) {
  const id = useId();
  return (
    <div className="field-row">
      <dt>
        <label htmlFor={id}>{it.name}</label>
      </dt>
      <dd>
        <FieldValue it={it} value={value} onSave={onSave} id={id} long={it.level === "concept"} />
      </dd>
    </div>
  );
}

/**
 * Adds a field to the termbase from an entry. On the team server it goes
 * into the termbase's definition at once, with its kind and values.
 */
function AddInfoType({ tb }: { tb: Termbase }) {
  const { dispatch, notify } = useStore();
  const r = useRemote();
  return (
    <AddFieldForm
      termbase={tb.name}
      taken={tb.infoTypes.map((i) => i.name)}
      onAdd={(f) => {
        if (tb.remote && r.api) {
          const api = r.api;
          void api
            .termbase(tb.name)
            .then((d) =>
              api.setDefinition(tb.name, {
                ...d.definition,
                fields: [...(d.definition.fields ?? []), { name: f.name, level: LEVEL_TO_CORE[f.level], kind: f.kind, ...(f.kind === "picklist" ? { values: f.values } : {}) }],
              }),
            )
            .then(
              () => {
                notify(tl("Added field “{name}” to {name2}", { name: f.name, name2: tb.name }));
                return r.openTermbase(tb.name);
              },
              (e) => notify(errorText(e)),
            );
          return;
        }
        dispatch({
          type: "addInfoType",
          termbaseId: tb.id,
          infoType: {
            id: `it-${Date.now().toString(36)}`,
            name: f.name,
            level: f.level,
            kind: f.kind,
            ...(f.kind === "picklist" ? { values: f.values } : {}),
          },
        });
        notify(tl("Added field “{name}” to {name2}", { name: f.name, name2: tb.name }));
      }}
    />
  );
}
