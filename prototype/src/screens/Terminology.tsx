import { useEffect, useId, useMemo, useRef, useState } from "react";
import { GitBranch, History, Plus, Search } from "lucide-react";
import { lang, primary, type Entry, type InfoType, type Location, type Termbase, type TermStatus } from "../data";
import { useStore, useWords } from "../store";
import { fmtTime, plural } from "../format";
import { InLang, LangChip, LocationBadge, PageHeader, StatusBadge } from "../components/ui";
import { go } from "../App";

export function Terminology({ termbaseId, entryId }: { termbaseId?: string; entryId?: string }) {
  const { state } = useStore();
  const tb = state.termbases.find((t) => t.id === termbaseId) ?? state.termbases.find((t) => t.kind === "master")!;
  const entries = state.entries.filter((e) => e.termbaseId === tb.id);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<TermStatus | "all">("all");

  const filtered = useMemo(() => {
    const q = query.trim().toLocaleLowerCase();
    return entries.filter((e) => {
      const terms = Object.values(e.terms).flat();
      if (status !== "all" && !terms.some((t) => t.status === status)) return false;
      if (!q) return true;
      return terms.some((t) => t.text.toLocaleLowerCase().includes(q)) || Object.values(e.concept).some((v) => v.toLocaleLowerCase().includes(q));
    });
  }, [entries, query, status]);

  const selected = entries.find((e) => e.id === entryId) ?? filtered[0];

  return (
    <div className="page page-wide">
      <PageHeader title="Terminology" />
      <div className="workspace">
        <TermbaseList current={tb.id} />

        <section className="pane pane-entries" aria-labelledby="entries-title">
          <div className="pane-head">
            <h2 id="entries-title" className="pane-title">
              {tb.name}
            </h2>
            <div className="pane-sub">
              <LocationBadge location={tb.location} server={tb.server} />
              <span>{plural(entries.length + (tb.extraEntries ?? 0), "entry", "entries")}</span>
              {tb.extraEntries ? <span className="muted">({entries.length} in this prototype)</span> : null}
            </div>
            {tb.kind === "project" && <ProjectBranchNote tb={tb} />}
          </div>
          <div className="filters">
            <div className="search">
              <Search size={15} aria-hidden="true" />
              <label htmlFor="q" className="sr-only">
                Search terms and fields
              </label>
              <input id="q" type="search" placeholder="Search terms and fields" value={query} onChange={(e) => setQuery(e.target.value)} />
            </div>
            <label htmlFor="status" className="sr-only">
              Status
            </label>
            <select id="status" value={status} onChange={(e) => setStatus(e.target.value as TermStatus | "all")}>
              <option value="all">All statuses</option>
              <option value="approved">Approved</option>
              <option value="proposed">Proposed</option>
              <option value="forbidden">Forbidden</option>
            </select>
          </div>
          {filtered.length === 0 ? (
            <div className="empty">
              {entries.length === 0 ? (
                <>
                  <p className="empty-title">No entries yet</p>
                  <p>Terms you add while translating land here, with the sentence they came from. There is nothing to set up first.</p>
                </>
              ) : (
                <>
                  <p className="empty-title">Nothing matches “{query}”</p>
                  <button type="button" className="btn btn-small" onClick={() => { setQuery(""); setStatus("all"); }}>
                    Clear search
                  </button>
                </>
              )}
            </div>
          ) : (
            <ul className="entry-list" aria-label={`Entries in ${tb.name}`}>
              {filtered.map((e) => (
                <EntryRow key={e.id} entry={e} tb={tb} selected={selected?.id === e.id} />
              ))}
            </ul>
          )}
        </section>

        {selected ? <EntryDetail key={selected.id} entry={selected} tb={tb} /> : <section className="pane pane-detail" aria-label="Entry" />}
      </div>
    </div>
  );
}

function TermbaseList({ current }: { current: string }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [loc, setLoc] = useState<Location>("local");
  const nameRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (creating) nameRef.current?.focus();
  }, [creating]);
  const groups: [string, Termbase[]][] = [
    ["Masters", state.termbases.filter((t) => t.kind === "master")],
    [`${w("projectTermbase")}s`, state.termbases.filter((t) => t.kind === "project")],
  ];
  return (
    <nav className="pane pane-tbs" aria-label="Termbases">
      {groups.map(([label, list]) => (
        <div key={label} className="tb-group">
          <h2 className="tb-group-title">{label}</h2>
          <ul>
            {list.map((t) => {
              const n = state.entries.filter((e) => e.termbaseId === t.id).length + (t.extraEntries ?? 0);
              return (
                <li key={t.id}>
                  <a href={`#/terminology/${t.id}`} aria-current={t.id === current ? "page" : undefined} className="tb-link">
                    <span className="tb-name">{t.name.replace(/ \(project\)$/, "")}</span>
                    <span className="tb-meta">
                      <LocationBadge location={t.location} server={t.server} />
                      <span className="tb-count">{n.toLocaleString("en-GB")}</span>
                    </span>
                  </a>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
      {creating ? (
        <form
          className="tb-create"
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim()) return;
            const id = `tb-${Date.now().toString(36)}`;
            dispatch({ type: "createMaster", termbase: { id, name: name.trim(), kind: "master", location: loc, server: loc === "server" ? "terms.kanzlei-weber.example" : undefined, languages: [], infoTypes: [] } });
            notify(`Created master “${name.trim()}”`);
            setCreating(false);
            setName("");
            go({ page: "terminology", termbaseId: id });
          }}
        >
          <label htmlFor="new-tb">Name</label>
          <input id="new-tb" ref={nameRef} value={name} onChange={(e) => setName(e.target.value)} placeholder="For example a client" />
          <div className="segmented segmented-small" role="radiogroup" aria-label="Store on">
            <label className="segment">
              <input type="radio" name="newloc" checked={loc === "local"} onChange={() => setLoc("local")} />
              <span>Local</span>
            </label>
            <label className="segment">
              <input type="radio" name="newloc" checked={loc === "server"} onChange={() => setLoc("server")} />
              <span>Server</span>
            </label>
          </div>
          <p className="help">Languages and fields are added as you use it.</p>
          <div className="row">
            <button type="submit" className="btn btn-primary btn-small" disabled={!name.trim()}>
              Create master
            </button>
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setCreating(false)}>
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <button type="button" className="btn btn-quiet btn-small tb-new" onClick={() => setCreating(true)}>
          <Plus size={14} aria-hidden="true" />
          New master
        </button>
      )}
    </nav>
  );
}

function ProjectBranchNote({ tb }: { tb: Termbase }) {
  const { state } = useStore();
  const w = useWords();
  const masters = (tb.basedOn ?? []).map((id) => state.termbases.find((t) => t.id === id)!).filter(Boolean);
  if (!masters.length) return null;
  return (
    <p className="branch-note">
      <GitBranch size={14} aria-hidden="true" />
      <span>
        {w("basedOn")} {masters.map((m) => m.name).join(", ")}. Shows only this project's new and changed entries.
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

function EntryRow({ entry, tb, selected }: { entry: Entry; tb: Termbase; selected: boolean }) {
  const src = sourceLangOf(tb);
  const srcTerms = entry.terms[src] ?? [];
  const others = tb.languages.filter((l) => l !== src);
  return (
    <li>
      <a href={`#/terminology/${tb.id}/${entry.id}`} className="entry-row" aria-current={selected ? "true" : undefined}>
        <span className="entry-head">
          <InLang code={src} className="entry-term">
            {srcTerms[0]?.text ?? "(no term)"}
          </InLang>
          {srcTerms.length > 1 && <span className="muted"> +{srcTerms.length - 1}</span>}
        </span>
        <span className="entry-targets">
          {others.map((l) => {
            const ts = entry.terms[l] ?? [];
            return (
              <span key={l} className="entry-target">
                <LangChip code={l} />
                {ts.length ? (
                  <InLang code={l}>{ts.filter((t) => t.status !== "forbidden").map((t) => t.text).join(", ") || ts[0]!.text}</InLang>
                ) : (
                  <span className="missing">missing</span>
                )}
              </span>
            );
          })}
        </span>
        {entry.concept.domain && <span className="entry-domain">{entry.concept.domain}</span>}
      </a>
    </li>
  );
}

function EntryDetail({ entry, tb }: { entry: Entry; tb: Termbase }) {
  const { state, dispatch } = useStore();
  const w = useWords();
  const [tab, setTab] = useState<"entry" | "history">("entry");
  const conceptTypes = tb.infoTypes.filter((i) => i.level === "concept");
  const termTypes = tb.infoTypes.filter((i) => i.level !== "concept");
  const languages = [...new Set([...tb.languages, ...Object.keys(entry.terms)])];
  const base = entry.base ? state.termbases.find((t) => t.id === entry.base!.termbaseId) : undefined;
  const now = () => new Date().toISOString();
  const latest = entry.history[entry.history.length - 1];

  return (
    <section className="pane pane-detail" aria-labelledby="detail-title">
      <div className="pane-head">
        <h2 id="detail-title" className="pane-title">
          <InLang code={sourceLangOf(tb)}>{entry.terms[sourceLangOf(tb)]?.[0]?.text ?? "Entry"}</InLang>
        </h2>
        <div className="pane-sub">
          {latest && (
            <span>
              Last changed {fmtTime(latest.at)} by {latest.by}
            </span>
          )}
          {base && (
            <span>
              {w("basedOn")} {base.name}
            </span>
          )}
          {tb.kind === "project" && !entry.base && <span className="tag">Not in a master yet</span>}
        </div>
      </div>

      <div className="tabs" role="tablist" aria-label="Entry views">
        <button type="button" role="tab" id="tab-entry" aria-selected={tab === "entry"} aria-controls="panel-entry" onClick={() => setTab("entry")}>
          Entry
        </button>
        <button type="button" role="tab" id="tab-history" aria-selected={tab === "history"} aria-controls="panel-history" onClick={() => setTab("history")}>
          <History size={14} aria-hidden="true" />
          {w("history")} ({entry.history.length})
        </button>
      </div>

      {tab === "entry" ? (
        <div role="tabpanel" id="panel-entry" aria-labelledby="tab-entry" className="detail-body">
          <dl className="fields">
            {conceptTypes.map((it) => (
              <FieldRow key={it.id} it={it} value={entry.concept[it.id] ?? ""} onSave={(v) => dispatch({ type: "setField", entryId: entry.id, field: it.id, value: v, now: now() })} />
            ))}
          </dl>
          <AddInfoType tb={tb} />

          {languages.map((code) => {
            const terms = entry.terms[code] ?? [];
            return (
              <section key={code} className="lang-block" aria-label={lang(code).name}>
                <h3 className="lang-title">
                  {lang(code).name}
                  <LangChip code={code} />
                </h3>
                {terms.length === 0 ? (
                  <p className="missing">No term yet</p>
                ) : (
                  <ul className="terms">
                    {terms.map((t, i) => (
                      <li key={t.text} className="term">
                        <div className="term-line">
                          <InLang code={code} className="term-text">
                            {t.text}
                          </InLang>
                          <StatusBadge status={t.status} />
                        </div>
                        {termTypes.length > 0 && (
                          <dl className="fields fields-term">
                            {termTypes.map((it) => (
                              <FieldRow
                                key={it.id}
                                it={it}
                                value={t.fields?.[it.id] ?? ""}
                                onSave={(v) => dispatch({ type: "setTermField", entryId: entry.id, langCode: code, termIndex: i, field: it.id, value: v, now: now() })}
                              />
                            ))}
                          </dl>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            );
          })}

          {entry.history.find((c) => c.segment) && (
            <section className="context" aria-label="Context">
              <h3 className="lang-title">Found in</h3>
              {entry.history
                .filter((c) => c.segment)
                .map((c) => (
                  <figure key={c.id} className="context-item">
                    <blockquote lang={primary(sourceLangOf(tb))}>{c.segment}</blockquote>
                    <figcaption>
                      {c.project}, {fmtTime(c.at)}
                    </figcaption>
                  </figure>
                ))}
            </section>
          )}
        </div>
      ) : (
        <div role="tabpanel" id="panel-history" aria-labelledby="tab-history" className="detail-body">
          <ol className="history">
            {[...entry.history].reverse().map((c) => (
              <li key={c.id}>
                <div className="history-summary">{c.summary}</div>
                <div className="history-meta">
                  {c.by}, {fmtTime(c.at)}
                  {c.project && <> in {c.project}</>}
                </div>
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}

function FieldRow({ it, value, onSave }: { it: InfoType; value: string; onSave: (v: string) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const id = useId();
  return (
    <div className="field-row">
      <dt>
        <label htmlFor={id}>{it.name}</label>
      </dt>
      <dd>
        {it.kind === "picklist" ? (
          <select id={id} value={draft} onChange={(e) => { setDraft(e.target.value); onSave(e.target.value); }}>
            <option value="">Not set</option>
            {it.values?.map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
            {draft && !it.values?.includes(draft) && <option value={draft}>{draft}</option>}
          </select>
        ) : it.level === "concept" ? (
          <textarea id={id} value={draft} rows={2} placeholder="Not set" onChange={(e) => setDraft(e.target.value)} onBlur={() => draft !== value && onSave(draft)} />
        ) : (
          <input id={id} value={draft} placeholder="Not set" onChange={(e) => setDraft(e.target.value)} onBlur={() => draft !== value && onSave(draft)} />
        )}
      </dd>
    </div>
  );
}

function AddInfoType({ tb }: { tb: Termbase }) {
  const { dispatch, notify } = useStore();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [scope, setScope] = useState<InfoType["level"]>("concept");
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  const existing = tb.infoTypes.find((i) => i.name.toLocaleLowerCase() === name.trim().toLocaleLowerCase());
  const similar = !existing && name.trim().length >= 3 ? tb.infoTypes.find((i) => i.name.toLocaleLowerCase().startsWith(name.trim().slice(0, 3).toLocaleLowerCase())) : undefined;

  if (!open)
    return (
      <button type="button" className="btn btn-quiet btn-small" onClick={() => setOpen(true)}>
        <Plus size={14} aria-hidden="true" />
        Add a field
      </button>
    );
  return (
    <form
      className="add-field"
      onSubmit={(e) => {
        e.preventDefault();
        if (!name.trim() || existing) return;
        dispatch({ type: "addInfoType", termbaseId: tb.id, infoType: { id: `it-${Date.now().toString(36)}`, name: name.trim(), level: scope, kind: "text" } });
        notify(`Added field “${name.trim()}” to ${tb.name}`);
        setOpen(false);
        setName("");
      }}
    >
      <label htmlFor="new-field">Field name</label>
      <input id="new-field" ref={ref} value={name} onChange={(e) => setName(e.target.value)} placeholder="For example Source, Region, Register" aria-describedby="new-field-hint" />
      <p id="new-field-hint" className="help" aria-live="polite">
        {existing ? `${tb.name} already has “${existing.name}”.` : similar ? `Did you mean the existing field “${similar.name}”?` : "Every entry in this termbase gets the field. Nothing else changes."}
      </p>
      <fieldset className="scope">
        <legend>Is the value the same in every language?</legend>
        <label className="check">
          <input type="radio" name="scope" checked={scope === "concept"} onChange={() => setScope("concept")} />
          <span>Yes, one value for the whole entry</span>
        </label>
        <label className="check">
          <input type="radio" name="scope" checked={scope === "term"} onChange={() => setScope("term")} />
          <span>No, each term has its own</span>
        </label>
      </fieldset>
      <div className="row">
        <button type="submit" className="btn btn-primary btn-small" disabled={!name.trim() || !!existing}>
          Add field
        </button>
        <button type="button" className="btn btn-quiet btn-small" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}
