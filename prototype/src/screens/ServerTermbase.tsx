import { useCallback, useEffect, useState, type FormEvent } from "react";
import { GitBranch, Pencil } from "lucide-react";
import { lang, LANGUAGES, type Entry, type Termbase } from "../data";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import {
  allows,
  ApiError,
  type DeletedEntry,
  type EntryVersion,
  type LayeredEntry,
  type Permission,
  type Removal,
  type ReviewRules,
  type TermbaseDetail,
} from "../server";
import type { CoreEntry } from "../termio";
import { fmtTime, plural } from "../format";
import { InLang, LangChip, StatusBadge } from "../components/ui";
import { ConfirmButton, Modal } from "./TermbaseDialogs";
import { AccessPanel } from "./AccessPanel";

/**
 * Whether the signed-in person may do something in a termbase. On this
 * computer everything is one's own; on the team server the role says.
 */
export const may = (tb: Termbase, p: Permission) => !tb.remote || allows(tb.role ?? "read", p);

/** The ID an entry of a server termbase has on the server. */
const serverId = (tb: Termbase, e: Entry) => e.id.slice(tb.id.length + 1);

const headOf = (e: CoreEntry, first?: string): string => {
  const ls = [...e.languages].sort((a, b) => Number(b.lang === first) - Number(a.lang === first));
  for (const l of ls) if (l.terms[0]) return l.terms[0].text;
  return "";
};

const termsLine = (e: CoreEntry) =>
  e.languages
    .filter((l) => l.terms.length)
    .map((l) => `${l.lang}: ${l.terms.map((t) => t.text).join(", ")}`)
    .join("; ");

/** An entry as it is, to read: its fields, languages and terms. */
export function ReadEntry({ entry }: { entry: CoreEntry }) {
  const values = (fs?: { field: string; value: string }[]) =>
    fs && fs.length > 0 ? (
      <dl className="fields fields-read">
        {fs.map((f) => (
          <div key={f.field}>
            <dt>{f.field}</dt>
            <dd>{f.value}</dd>
          </div>
        ))}
      </dl>
    ) : null;
  return (
    <>
      {values(entry.fields)}
      {entry.languages.map((l) => (
        <section key={l.lang} className="lang-block" aria-label={lang(l.lang).name}>
          <h3 className="lang-title">
            {lang(l.lang).name}
            <LangChip code={l.lang} />
          </h3>
          {values(l.fields)}
          <ul className="terms">
            {l.terms.map((t, i) => (
              <li key={i} className="term">
                <div className="term-line">
                  <InLang code={l.lang} className="term-text">
                    {t.text}
                  </InLang>
                  <StatusBadge status={(t.status || "proposed") as never} />
                </div>
                {values(t.fields)}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}

/** An entry of a server termbase as the store has it, for reading. */
export function ReadStoreEntry({ tb, entry }: { tb: Termbase; entry: Entry }) {
  const r = useRemote();
  const [core, setCore] = useState<CoreEntry | null>(null);
  const id = serverId(tb, entry);
  useEffect(() => {
    let live = true;
    r.api?.entry(tb.name, id).then(
      (e) => live && setCore(e),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [r.api, tb.name, id, entry.version]);
  return core ? <ReadEntry entry={core} /> : <p className="muted">Loading the entry…</p>;
}

/** The versions of an entry on the server, newest first; an earlier one is restored. */
export function ServerHistory({ tb, entry }: { tb: Termbase; entry: Entry }) {
  const { notify } = useStore();
  const r = useRemote();
  const [versions, setVersions] = useState<EntryVersion[] | null>(null);
  const [problem, setProblem] = useState("");
  const id = serverId(tb, entry);
  useEffect(() => {
    let live = true;
    r.api?.entryHistory(tb.name, id).then(
      (vs) => live && setVersions(vs),
      (e) => live && setProblem(errorText(e)),
    );
    return () => {
      live = false;
    };
  }, [r.api, tb.name, id, entry.version]);
  if (problem)
    return (
      <p className="error" role="alert">
        {problem}
      </p>
    );
  if (!versions) return <p className="muted">Loading the history…</p>;
  const newest = versions[versions.length - 1]?.version;
  return (
    <ol className="history">
      {[...versions].reverse().map((v) => (
        <li key={v.version}>
          <div className="history-summary">{v.summary || "Changed"}</div>
          <div className="history-meta">
            {v.by ? `${v.by}, ` : ""}
            {fmtTime(v.at)}
          </div>
          <div className="history-held">{termsLine(v.entry)}</div>
          {v.version !== newest && may(tb, "edit") && (
            <ConfirmButton
              plain
              label="Restore this version"
              confirm="Set the entry back to this version? What it is now stays in the history."
              action="Restore"
              onConfirm={() =>
                void r.api!.revertEntry(tb.name, id, v.version).then(
                  async () => {
                    notify(`Restored the version of ${fmtTime(v.at)}`);
                    await r.openTermbase(tb.name);
                  },
                  (e) => notify(`The version was not restored: ${errorText(e)}`),
                )
              }
            />
          )}
        </li>
      ))}
    </ol>
  );
}

/** The deleted entries of a server termbase, to bring back. */
export function ServerDeleted({ tb }: { tb: Termbase }) {
  const { notify } = useStore();
  const r = useRemote();
  const [gone, setGone] = useState<DeletedEntry[] | null>(null);
  const [problem, setProblem] = useState("");
  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      // Of an entry deleted more than once, the last deletion counts.
      const seen = new Set<string>();
      setGone((await r.api.deletedEntries(tb.name)).filter((d) => !seen.has(d.entryId) && seen.add(d.entryId)));
      setProblem("");
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api, tb.name]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => e.type === "termbase" && void load()), [listen, load]);
  if (problem)
    return (
      <p className="error pane-pad" role="alert">
        {problem}
      </p>
    );
  if (!gone) return <p className="muted pane-pad">Loading…</p>;
  if (gone.length === 0)
    return (
      <div className="empty">
        <p className="empty-title">Nothing deleted</p>
        <p>Entries you delete are kept here and can be brought back.</p>
      </div>
    );
  return (
    <ul className="deleted-list" aria-label={`Deleted entries of ${tb.name}`}>
      {gone.map((d) => (
        <li key={d.entryId} className="deleted-entry">
          <dl className="mr-terms">
            {d.entry.languages
              .filter((l) => l.terms.length)
              .map((l) => (
                <div key={l.lang}>
                  <dt>
                    <LangChip code={l.lang} />
                  </dt>
                  <dd>{l.terms.map((t) => t.text).join(", ")}</dd>
                </div>
              ))}
          </dl>
          <p className="history-meta">
            Deleted {fmtTime(d.at)}
            {d.by ? ` by ${d.by}` : ""}
            {d.summary && d.summary !== "Deleted" ? `. ${d.summary}` : ""}
          </p>
          {may(tb, "edit") && (
            <button
              type="button"
              className="btn btn-small"
              onClick={() =>
                void r.api!.restoreEntry(tb.name, d.entryId).then(
                  async (e) => {
                    const head = headOf(e, tb.languages[0]);
                    notify(head ? `Brought back “${head}”` : "Brought the entry back");
                    await r.openTermbase(tb.name);
                    await load();
                  },
                  (e) => notify(`The entry was not brought back: ${errorText(e)}`),
                )
              }
            >
              Bring back
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}

type MasterEntry = LayeredEntry & { changedAt?: string; createdAt?: string };

/** The master's entries a project termbase uses as they are, and those it leaves out. */
export function useMasterEntries(tb: Termbase) {
  const r = useRemote();
  const [masters, setMasters] = useState<MasterEntry[]>([]);
  const [removals, setRemovals] = useState<(Removal & { head: string })[]>([]);
  const project = !!tb.remote && tb.kind === "project";
  const load = useCallback(async () => {
    if (!r.api || !project) {
      setMasters([]);
      setRemovals([]);
      return;
    }
    try {
      const [layered, left] = await Promise.all([r.api.layered(tb.name), r.api.removals(tb.name)]);
      setMasters(layered.filter((e) => e.termbase !== tb.name));
      // An entry left out is named by its terms, as the master has it.
      setRemovals(
        await Promise.all(
          left.map(async (x) => ({
            ...x,
            head: await r.api!.entry(x.termbase, x.entryId).then(
              (e) => headOf(e, tb.languages[0]) || x.entryId,
              () => x.entryId,
            ),
          })),
        ),
      );
    } catch {
      setMasters([]);
      setRemovals([]);
    }
  }, [r.api, project, tb.name, tb.languages]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => e.type === "termbase" && void load()), [listen, load]);
  return { masters, removals, reload: load };
}

export const masterKey = (e: { termbase: string; id?: string }) => `${e.termbase}\n${e.id ?? ""}`;

/** The list of the master's entries under a project termbase. */
export function ServerMasterList({
  tb,
  masters,
  removals,
  selected,
  onSelect,
  reload,
}: {
  tb: Termbase;
  masters: MasterEntry[];
  removals: (Removal & { head: string })[];
  selected: string;
  onSelect: (key: string) => void;
  reload: () => Promise<void>;
}) {
  const { notify } = useStore();
  const r = useRemote();
  const edit = may(tb, "edit");
  return (
    <>
      <p className="help pane-bar">
        {edit
          ? "The master's entries this project uses as they are. Change one for this project only, or leave it out."
          : "The master's entries this project uses as they are."}
      </p>
      {masters.length === 0 && <p className="muted pane-pad">The masters have no entries the project does not have itself.</p>}
      <ul className="entry-list entry-list-fit" aria-label={`Entries of the masters of ${tb.name}`}>
        {masters.map((e) => {
          const parts = tb.languages
            .map((l) => e.languages.find((x) => x.lang === l)?.terms.map((t) => t.text).join(", ") ?? "")
            .filter(Boolean);
          return (
            <li key={masterKey(e)}>
              <button type="button" className="entry-row" aria-current={masterKey(e) === selected ? "true" : undefined} onClick={() => onSelect(masterKey(e))}>
                <span className="entry-head">
                  <span className="entry-term">{parts[0] ?? "No term"}</span>
                </span>
                {parts.length > 1 && <span className="entry-other">{parts.slice(1).join(" · ")}</span>}
                <span className="tb-count">In {e.termbase}</span>
              </button>
            </li>
          );
        })}
      </ul>
      {removals.length > 0 && (
        <section className="left-out" aria-labelledby="left-out-title">
          <h3 id="left-out-title" className="tb-group-title">
            Left out of this project
          </h3>
          <p className="history-meta">Merging asks the master's reviewers to delete these there.</p>
          <ul>
            {removals.map((x) => (
              <li key={`${x.termbase}\n${x.entryId}`}>
                <span className="entry-term">{x.head}</span>
                <span className="history-meta">
                  In {x.termbase}, left out {fmtTime(x.at)}
                  {x.by ? ` by ${x.by}` : ""}
                </span>
                {edit && (
                  <button
                    type="button"
                    className="btn btn-small"
                    onClick={() =>
                      void r.api!.bringBack(tb.name, x.termbase, x.entryId).then(
                        async () => {
                          notify("Brought the entry back into the project");
                          await reload();
                        },
                        (e) => notify(errorText(e)),
                      )
                    }
                  >
                    Bring back
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

/** A master's entry under a project termbase: read, taken into the project, or left out. */
export function ServerMasterDetail({
  tb,
  entry,
  reload,
  onTaken,
}: {
  tb: Termbase;
  entry: MasterEntry;
  reload: () => Promise<void>;
  /** The entry was taken into the project: its copy there is shown. */
  onTaken: () => void;
}) {
  const { notify } = useStore();
  const r = useRemote();
  const head = headOf(entry, tb.languages[0]);
  const changed = entry.changedAt ?? entry.createdAt;
  return (
    <section className="pane pane-detail" aria-labelledby="master-entry-title">
      <div className="pane-head">
        <div className="pane-head-row">
          <h2 id="master-entry-title" className="pane-title">
            {head || "Entry"}
          </h2>
        </div>
        <div className="pane-sub">
          <span>
            In {entry.termbase}
            {changed ? `, last changed ${fmtTime(changed)}` : ""}
          </span>
        </div>
      </div>
      <div className="detail-body">
        {may(tb, "edit") && (
          <>
            <p className="notice-plain">
              This is the master's entry. To change it, take it into the project: your version is used here and goes to the master's reviewers when you
              merge.
            </p>
            <div className="row-start">
              <button
                type="button"
                className="btn btn-small"
                onClick={() =>
                  void r.api!.overrideEntry(tb.name, entry.termbase, entry.id ?? "").then(
                    async (copy) => {
                      notify("Took the entry into the project");
                      const id = await r.openTermbase(tb.name);
                      await reload();
                      onTaken();
                      window.location.hash = `#/terminology/${id}/${id}:${copy.id}`;
                    },
                    (e) => notify(errorText(e)),
                  )
                }
              >
                <Pencil size={14} aria-hidden="true" /> Change in this project
              </button>
              <ConfirmButton
                label="Leave out of this project"
                confirm="Leave this entry out of the project? Merging asks the master's reviewers to delete it there."
                action="Leave out"
                onConfirm={() =>
                  void r.api!.leaveOut(tb.name, entry.termbase, entry.id ?? "").then(
                    async () => {
                      notify("Left the entry out of the project");
                      await reload();
                    },
                    (e) => notify(errorText(e)),
                  )
                }
              />
            </div>
          </>
        )}
        <ReadEntry entry={entry} />
      </div>
    </section>
  );
}

/** A project termbase on the team server, made by hand over masters there. */
export function NewServerProjectTermbase({ onDone }: { onDone: () => void }) {
  const { notify } = useStore();
  const r = useRemote();
  const [name, setName] = useState("");
  const [masters, setMasters] = useState<string[] | null>(null);
  const [chosen, setChosen] = useState<string[]>([]);
  useEffect(() => {
    r.api?.termbases().then(
      (list) => setMasters(list.filter((t) => t.kind === "master").map((t) => t.name)),
      () => setMasters([]),
    );
  }, [r.api]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const n = name.trim();
    if (!n || !chosen.length) return;
    try {
      await r.api!.createTermbase({ name: n }, chosen);
      notify(`Created project termbase “${n}”`);
      const id = await r.openTermbase(n);
      onDone();
      window.location.hash = `#/terminology/${id}`;
    } catch (err) {
      notify(errorText(err));
    }
  }

  return (
    <form className="tb-create" onSubmit={submit} aria-label="New project termbase">
      <h2 className="tb-group-title">New project termbase</h2>
      <label htmlFor="new-ptb">Name</label>
      <input id="new-ptb" value={name} autoFocus placeholder="For example a case or an order" onChange={(e) => setName(e.target.value)} />
      <span className="field-label">Based on</span>
      {masters === null ? (
        <p className="muted">Loading…</p>
      ) : masters.length === 0 ? (
        <p className="help">There is no master here yet. Make one first.</p>
      ) : (
        masters.map((m) => (
          <label key={m} className="check">
            <input type="checkbox" checked={chosen.includes(m)} onChange={(e) => setChosen((c) => (e.target.checked ? [...c, m] : c.filter((x) => x !== m)))} />
            {m}
          </label>
        ))
      )}
      <p className="help">It has the masters' languages and fields. A project's own termbase is made with the project.</p>
      <div className="row">
        <button type="submit" className="btn btn-primary btn-small" disabled={!name.trim() || !chosen.length} aria-label="Create project termbase">
          Create
        </button>
        <button type="button" className="btn btn-quiet btn-small" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  );
}

/**
 * The settings of a termbase on the team server, for those who manage it:
 * its name, languages and fields, the masters a project termbase is based
 * on, how a master is reviewed, who may use it, and deleting it.
 */
export function ServerTermbaseSettings({ tb, entries, onClose }: { tb: Termbase; entries: Entry[]; onClose: () => void }) {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const [detail, setDetail] = useState<TermbaseDetail | null>(null);
  const [name, setName] = useState(tb.name);
  const [current, setCurrent] = useState(tb.name);
  const [problem, setProblem] = useState("");
  const [masters, setMasters] = useState<string[]>([]);
  const [rules, setRules] = useState<ReviewRules>({});
  const [usedBy, setUsedBy] = useState<string[]>([]);
  const [addLang, setAddLang] = useState("");
  const [trustWho, setTrustWho] = useState("");
  const [trustLang, setTrustLang] = useState("");

  const load = useCallback(
    async (n: string) => {
      try {
        const d = await api.termbase(n);
        setDetail(d);
        if (d.kind === "master") setRules(await api.reviewRules(n));
        else setMasters((await api.termbases()).filter((t) => t.kind === "master").map((t) => t.name));
        setUsedBy(
          (r.projects ?? [])
            .filter((p) => {
              const t = p.settings.terminology as { termbase?: string; writeTo?: string; masters?: string[] } | undefined;
              return [t?.termbase, t?.writeTo, ...(t?.masters ?? [])].some((x) => x?.toLowerCase() === n.toLowerCase());
            })
            .map((p) => p.settings.name),
        );
      } catch (e) {
        setProblem(errorText(e));
      }
    },
    [api, r.projects],
  );
  useEffect(() => {
    void load(current);
  }, [load, current]);

  /** Does one step, says it, and reads the termbase again, here and on the page. */
  async function step(what: () => Promise<unknown>, done: string, as = current) {
    try {
      await what();
      setProblem("");
      if (done) notify(done);
      await load(as);
      await r.openTermbase(as);
    } catch (e) {
      setProblem(errorText(e));
    }
  }

  async function rename() {
    const to = name.trim();
    if (!to || to === current) {
      setName(current);
      return;
    }
    try {
      await api.renameTermbase(current, to);
      setProblem("");
      notify(`Renamed to “${to}”`);
      setCurrent(to);
      const id = await r.openTermbase(to);
      window.location.hash = `#/terminology/${id}`;
    } catch (e) {
      setName(current);
      setProblem(errorText(e));
    }
  }

  if (!detail)
    return (
      <Modal title="Termbase settings" onClose={onClose} wide>
        {problem ? (
          <p className="error" role="alert">
            {problem}
          </p>
        ) : (
          <p className="muted">Loading…</p>
        )}
      </Modal>
    );
  const def = detail.definition;
  const langs = def.languages ?? [];
  const fields = def.fields ?? [];
  const termsIn = (l: string) => entries.reduce((n, e) => n + (e.terms[l]?.length ?? 0), 0);
  const nameOf = (username: string) => {
    const p = r.people.find((x) => x.username === username);
    return p?.name ? `${p.name} (${username})` : username;
  };
  const putRules = (next: ReviewRules, done: string) =>
    step(async () => {
      setRules(await api.setReviewRules(current, next));
    }, done);
  const minor = rules.minorFields ?? [];
  const trusted = rules.trusted ?? [];
  const upstreams = detail.upstreams ?? [];
  const based = detail.projects ?? [];

  return (
    <Modal title="Termbase settings" onClose={onClose} wide>
      <label className="field">
        <span className="field-label">Name</span>
        <input value={name} onChange={(e) => setName(e.target.value)} onBlur={() => void rename()} onKeyDown={(e) => e.key === "Enter" && void rename()} />
      </label>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}

      <section className="settings-section" aria-labelledby="stb-langs">
        <h3 id="stb-langs" className="settings-title">
          Languages
        </h3>
        {langs.length === 0 ? (
          <p className="help">No languages yet. They are added when you add terms or import a file.</p>
        ) : (
          <ul className="settings-list">
            {langs.map((l) => (
              <li key={l}>
                <span>
                  {lang(l).name} <LangChip code={l} />
                </span>
                <span className="muted">{plural(termsIn(l), "term", "terms")}</span>
                <ConfirmButton
                  label={<span aria-label={`Remove ${lang(l).name}`}>Remove</span>}
                  confirm={termsIn(l) ? `Remove ${lang(l).name} and its ${plural(termsIn(l), "term", "terms")}?` : `Remove ${lang(l).name}?`}
                  onConfirm={() => void step(() => api.removeLanguage(current, l), `Removed ${lang(l).name}`)}
                />
              </li>
            ))}
          </ul>
        )}
        <form
          className="role-add"
          aria-label="Add a language"
          onSubmit={(e) => {
            e.preventDefault();
            if (!addLang) return;
            const l = addLang;
            setAddLang("");
            void step(() => api.setDefinition(current, { ...def, languages: [...langs, l] }), `Added ${lang(l).name}`);
          }}
        >
          <select value={addLang} aria-label="Add a language" onChange={(e) => setAddLang(e.target.value)}>
            <option value="">Add a language…</option>
            {LANGUAGES.filter((l) => !langs.includes(l.code)).map((l) => (
              <option key={l.code} value={l.code}>
                {l.name}
              </option>
            ))}
          </select>
          <button type="submit" className="btn btn-small" disabled={!addLang}>
            Add
          </button>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="stb-fields">
        <h3 id="stb-fields" className="settings-title">
          Fields
        </h3>
        {fields.length === 0 ? (
          <p className="help">No fields yet. Add them from an entry, or import a file that has them.</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">Applies to</th>
                <th scope="col">Values</th>
                <th scope="col">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {fields.map((f) => (
                <ServerField
                  key={f.name}
                  field={f}
                  onRename={(to) => void step(() => api.renameField(current, f.name, to), `Renamed the field to “${to}”`)}
                  onChange={(next) =>
                    void step(() => api.setDefinition(current, { ...def, fields: fields.map((x) => (x.name === f.name ? next : x)) }), "")
                  }
                  onRemove={() => void step(() => api.removeField(current, f.name), `Removed the field “${f.name}”`)}
                />
              ))}
            </tbody>
          </table>
        )}
      </section>

      {detail.kind === "project" ? (
        <section className="settings-section" aria-labelledby="stb-based">
          <h3 id="stb-based" className="settings-title">
            Based on
          </h3>
          <p className="help">The termbase shows the entries of these masters under its own, and its new and changed entries are submitted to them.</p>
          {[...new Set([...masters, ...upstreams])].map((m) => {
            const on = upstreams.includes(m);
            return (
              <label key={m} className="check" title={on && upstreams.length === 1 ? "It needs one master at least." : undefined}>
                <input
                  type="checkbox"
                  checked={on}
                  disabled={on && upstreams.length === 1}
                  onChange={(e) =>
                    void step(
                      () => api.setUpstreams(current, e.target.checked ? [...upstreams, m] : upstreams.filter((x) => x !== m)),
                      e.target.checked ? `Now also based on ${m}` : `No longer based on ${m}`,
                    )
                  }
                />
                {m}
              </label>
            );
          })}
        </section>
      ) : (
        <section className="settings-section" aria-labelledby="stb-review">
          <h3 id="stb-review" className="settings-title">
            Review
          </h3>
          <p className="help">
            Projects submit their new and changed entries to this master, and its reviewers approve them. These rules say what is merged without waiting
            for them.
          </p>
          <label className="check">
            <input
              type="checkbox"
              checked={!rules.keepIdentical}
              onChange={(e) =>
                void putRules(
                  { ...rules, keepIdentical: e.target.checked ? undefined : true },
                  e.target.checked ? "Entries the master already has are merged without review" : "Entries the master already has wait for review, too",
                )
              }
            />
            Entries the master already has as they are need no review
          </label>
          <span className="field-label">Fields that need no approval</span>
          {fields.length === 0 ? (
            <p className="help">The termbase has no fields yet. A change to notes or sources only could be merged at once.</p>
          ) : (
            <>
              <div className="row-start" role="group" aria-label="Fields that need no approval">
                {fields.map((f) => (
                  <label key={f.name} className="check">
                    <input
                      type="checkbox"
                      checked={minor.some((m) => m.toLowerCase() === f.name.toLowerCase())}
                      onChange={(e) => {
                        const next = e.target.checked ? [...minor, f.name] : minor.filter((m) => m !== f.name);
                        void putRules(
                          { ...rules, minorFields: next.length ? next : undefined },
                          e.target.checked ? `Changes to ${f.name} only are merged without review` : `Changes to ${f.name} wait for review`,
                        );
                      }}
                    />
                    {f.name}
                  </label>
                ))}
              </div>
              <p className="help">An entry changed in these fields only is merged at once.</p>
            </>
          )}
          <span className="field-label">Trusted authors</span>
          {trusted.length === 0 ? (
            <p className="help">Nobody yet: every entry waits for a reviewer.</p>
          ) : (
            <ul className="mr-reviewers">
              {trusted.map((t, i) => (
                <li key={t.author}>
                  <span className="cell-title">{nameOf(t.author)}</span>
                  <span className="cell-sub">
                    {t.scope.langs?.length ? `entries with terms in ${t.scope.langs.map((l) => lang(l).name).join(", ")}` : "all entries"}
                  </span>
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    aria-label={`No longer trust ${t.author}`}
                    onClick={() => {
                      const next = trusted.filter((_, j) => j !== i);
                      void putRules({ ...rules, trusted: next.length ? next : undefined }, `The entries of ${t.author} wait for review again`);
                    }}
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
          <form
            className="role-add"
            aria-label="Trust an author"
            onSubmit={(e) => {
              e.preventDefault();
              if (!trustWho) {
                setProblem("Say whose entries to trust.");
                return;
              }
              // One rule an author: a new one replaces the old.
              const next = [
                ...trusted.filter((t) => t.author.toLowerCase() !== trustWho.toLowerCase()),
                { author: trustWho, scope: trustLang ? { langs: [trustLang] } : {} },
              ];
              const who = trustWho;
              setTrustWho("");
              setTrustLang("");
              void putRules({ ...rules, trusted: next }, `The entries of ${who} are approved without review`);
            }}
          >
            <label>
              <span className="field-label">Author</span>
              <select value={trustWho} onChange={(e) => setTrustWho(e.target.value)}>
                <option value="">Choose a person…</option>
                {r.people.map((p) => (
                  <option key={p.username} value={p.username}>
                    {nameOf(p.username)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="field-label">For</span>
              <select value={trustLang} onChange={(e) => setTrustLang(e.target.value)}>
                <option value="">All entries</option>
                {langs.map((l) => (
                  <option key={l} value={l}>
                    Terms in {lang(l).name}
                  </option>
                ))}
              </select>
            </label>
            <button type="submit" className="btn btn-small">
              Trust
            </button>
          </form>
          <p className="help">
            Their new and changed entries are approved as they are submitted. An entry that conflicts with the master always waits for a reviewer.
          </p>
        </section>
      )}

      <section className="settings-section" aria-label="Who may use it">
        <AccessPanel key={current} kind="termbase" resource={current} title="Who may use it" embedded />
      </section>

      <section className="settings-section settings-danger" aria-labelledby="stb-delete">
        <h3 id="stb-delete" className="settings-title">
          Delete termbase
        </h3>
        {based.length > 0 ? (
          <p className="help">
            {based.join(", ")} {based.length === 1 ? "is" : "are"} based on this termbase. Delete {based.length === 1 ? "it" : "them"}, or base{" "}
            {based.length === 1 ? "it" : "them"} on another master, first.
          </p>
        ) : usedBy.length > 0 ? (
          <p className="help">
            {usedBy.join(", ")} {usedBy.length === 1 ? "uses" : "use"} this termbase. Remove it from the project first.
          </p>
        ) : (
          <>
            <p className="help">
              {detail.kind === "project"
                ? `Deletes the project's ${plural(entries.length, "entry", "entries")} and their history. The masters keep theirs. Merge what the masters should have first.`
                : `Deletes ${plural(entries.length, "entry", "entries")} and their history. Export the termbase first if you may need it again.`}
            </p>
            <ConfirmButton
              label="Delete termbase"
              confirm={`Delete ${current} for good?`}
              action="Delete"
              onConfirm={() =>
                void api.deleteTermbase(current).then(
                  () => {
                    notify(`Deleted ${current}`);
                    onClose();
                    window.location.hash = "#/terminology";
                    void r.refresh();
                  },
                  (e) => setProblem(e instanceof ApiError ? e.message : errorText(e)),
                )
              }
            />
          </>
        )}
      </section>

      <div className="row">
        <button type="button" className="btn btn-primary btn-small" onClick={onClose}>
          Done
        </button>
      </div>
    </Modal>
  );
}

type FieldDef = NonNullable<TermbaseDetail["definition"]["fields"]>[number];

const LEVELS: Record<string, string> = { entry: "Entry", language: "Language", term: "Term" };

/** A field of a server termbase: its name, what kind of value it takes, and removing it. */
function ServerField({ field, onRename, onChange, onRemove }: { field: FieldDef; onRename: (to: string) => void; onChange: (f: FieldDef) => void; onRemove: () => void }) {
  const [name, setName] = useState(field.name);
  const [values, setValues] = useState((field.values ?? []).join("; "));
  const pick = field.kind === "picklist";
  const rename = () => {
    const to = name.trim();
    if (to && to !== field.name) onRename(to);
    else setName(field.name);
  };
  const saveValues = () => {
    const now = [...new Set(values.split(";").map((v) => v.trim()).filter(Boolean))];
    if (now.join("\n") !== (field.values ?? []).join("\n")) onChange({ ...field, values: now });
  };
  return (
    <tr>
      <td>
        <input value={name} aria-label={`Name of ${field.name}`} onChange={(e) => setName(e.target.value)} onBlur={rename} onKeyDown={(e) => e.key === "Enter" && rename()} />
      </td>
      <td>{LEVELS[field.level] ?? field.level}</td>
      <td>
        <span className="row-start">
          <select
            value={pick ? "picklist" : "text"}
            aria-label={`Values of ${field.name}`}
            onChange={(e) => onChange(e.target.value === "picklist" ? { ...field, kind: "picklist", values: field.values ?? [] } : { ...field, kind: "text", values: undefined })}
          >
            <option value="text">Free text</option>
            <option value="picklist">A list of values</option>
          </select>
          {pick && (
            <input
              value={values}
              placeholder="Values, separated by ;"
              aria-label="List values, separated by semicolons"
              onChange={(e) => setValues(e.target.value)}
              onBlur={saveValues}
              onKeyDown={(e) => e.key === "Enter" && saveValues()}
            />
          )}
        </span>
      </td>
      <td className="cell-action">
        <ConfirmButton label={<span aria-label={`Remove ${field.name}`}>Remove</span>} confirm={`Remove “${field.name}” and its values?`} onConfirm={onRemove} />
      </td>
    </tr>
  );
}

/** What a project termbase on the server is based on, in the pane's head. */
export function ServerBranchNote({ tb }: { tb: Termbase }) {
  const masters = (tb.basedOn ?? []).map((id) => id.replace(/^srv:/, ""));
  if (tb.kind !== "project" || !masters.length) return null;
  return (
    <p className="branch-note">
      <GitBranch size={14} aria-hidden="true" />
      <span>Based on {masters.join(", ")}. Shows only this project's new and changed entries.</span>
    </p>
  );
}
