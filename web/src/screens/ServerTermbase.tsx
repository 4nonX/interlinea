import { useCallback, useEffect, useState } from "react";
import { AddFieldForm, KindSelect, ValuesEditor } from "./TermbaseFields";
import { LEVEL_TO_CORE } from "../termio";
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
import { fmtTime, listOf, plural } from "../format";
import { InLang, LangChip, StatusBadge } from "../components/ui";
import { ConfirmButton, Modal } from "./TermbaseDialogs";
import { AccessPanel } from "./AccessPanel";
import { tl, tlCore, tlx } from "../i18n";

/**
 * Whether the signed-in person may do something in a termbase. On this
 * computer everything is one's own; on the team server the role says.
 */
export const may = (tb: Termbase, p: Permission) => !tb.remote || allows(tb.role ?? "read", p);

/** The ID an entry of a server termbase has on the server. */
const serverId = (tb: Termbase, e: Entry) => e.id.slice(tb.id.length + 1);

export const headOf = (e: CoreEntry, first?: string): string => {
  const ls = [...e.languages].sort((a, b) => Number(b.lang === first) - Number(a.lang === first));
  for (const l of ls) if (l.terms[0]) return l.terms[0].text;
  return "";
};

export const termsLine = (e: CoreEntry) =>
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
  return core ? <ReadEntry entry={core} /> : <p className="muted">{tl("Loading the entry…")}</p>;
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
  if (!versions) return <p className="muted">{tl("Loading the history…")}</p>;
  const newest = versions[versions.length - 1]?.version;
  return (
    <ol className="history">
      {[...versions].reverse().map((v) => (
        <li key={v.version}>
          <div className="history-summary">{v.summary ? tlCore(v.summary) : tl("Changed")}</div>
          <div className="history-meta">
            {v.by ? `${v.by}, ` : ""}
            {fmtTime(v.at)}
          </div>
          <div className="history-held">{termsLine(v.entry)}</div>
          {v.version !== newest && may(tb, "edit") && (
            <ConfirmButton
              plain
              label={tl("Restore this version")}
              confirm={tl("Set the entry back to this version? What it is now stays in the history.")}
              action={tl("Restore")}
              onConfirm={() =>
                void r.api!.revertEntry(tb.name, id, v.version).then(
                  async () => {
                    notify(tl("Restored the version of {at}", { at: fmtTime(v.at) }));
                    await r.openTermbase(tb.name);
                  },
                  (e) => notify(tl("The version was not restored: {e}", { e: errorText(e) })),
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
  if (!gone) return <p className="muted pane-pad">{tl("Loading…")}</p>;
  if (gone.length === 0)
    return (
      <div className="empty">
        <p className="empty-title">{tl("Nothing deleted")}</p>
        <p>{tl("Entries you delete are kept here and can be brought back.")}</p>
      </div>
    );
  return (
    <ul className="deleted-list" aria-label={tl("Deleted entries of {name}", { name: tb.name })}>
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
            {d.summary && d.summary !== "Deleted"
              ? d.by
                ? tl("Deleted {at} by {by}. {summary}", { at: fmtTime(d.at), by: d.by, summary: tlCore(d.summary) })
                : tl("Deleted {at}. {summary}", { at: fmtTime(d.at), summary: tlCore(d.summary) })
              : d.by
                ? tl("Deleted {at} by {by}", { at: fmtTime(d.at), by: d.by })
                : tl("Deleted {at}", { at: fmtTime(d.at) })}</p>
          {may(tb, "edit") && (
            <button
              type="button"
              className="btn btn-small"
              onClick={() =>
                void r.api!.restoreEntry(tb.name, d.entryId).then(
                  async (e) => {
                    const head = headOf(e, tb.languages[0]);
                    notify(head ? tl("Brought back “{head}”", { head }) : tl("Brought the entry back"));
                    await r.openTermbase(tb.name);
                    await load();
                  },
                  (e) => notify(tl("The entry was not brought back: {e}", { e: errorText(e) })),
                )
              }
            >
              {tl("Bring back")}
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
          ? tl("The master's entries this project uses as they are. Change one for this project only, or leave it out.")
          : tl("The master's entries this project uses as they are.")}
      </p>
      {masters.length === 0 && <p className="muted pane-pad">{tl("The masters have no entries the project does not have itself.")}</p>}
      <ul className="entry-list entry-list-fit" aria-label={tl("Entries of the masters of {name}", { name: tb.name })}>
        {masters.map((e) => {
          const parts = tb.languages
            .map((l) => e.languages.find((x) => x.lang === l)?.terms.map((t) => t.text).join(", ") ?? "")
            .filter(Boolean);
          return (
            <li key={masterKey(e)}>
              <button type="button" className="entry-row" aria-current={masterKey(e) === selected ? "true" : undefined} onClick={() => onSelect(masterKey(e))}>
                <span className="entry-head">
                  <span className="entry-term">{parts[0] ?? tl("No term")}</span>
                </span>
                {parts.length > 1 && <span className="entry-other">{parts.slice(1).join(" · ")}</span>}
                <span className="tb-count">{tl("In {name}", { name: e.termbase })}</span>
              </button>
            </li>
          );
        })}
      </ul>
      {removals.length > 0 && (
        <section className="left-out" aria-labelledby="left-out-title">
          <h3 id="left-out-title" className="tb-group-title">
            {tl("Left out of this project")}
          </h3>
          <p className="history-meta">{tl("Merging asks the master's reviewers to delete these there.")}</p>
          <ul>
            {removals.map((x) => (
              <li key={`${x.termbase}\n${x.entryId}`}>
                <span className="entry-term">{x.head}</span>
                <span className="history-meta">
                  {(x.by ? tl("In {termbase}, left out {at} by {by}", { termbase: x.termbase, at: fmtTime(x.at), by: x.by }) : tl("In {termbase}, left out {at}", { termbase: x.termbase, at: fmtTime(x.at) }))}</span>
                {edit && (
                  <button
                    type="button"
                    className="btn btn-small"
                    onClick={() =>
                      void r.api!.bringBack(tb.name, x.termbase, x.entryId).then(
                        async () => {
                          notify(tl("Brought the entry back into the project"));
                          await reload();
                        },
                        (e) => notify(errorText(e)),
                      )
                    }
                  >
                    {tl("Bring back")}
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
            {head || tl("Entry")}
          </h2>
        </div>
        <div className="pane-sub">
          <span>
            {changed ? tl("In {name}, last changed {changed}", { name: entry.termbase, changed: fmtTime(changed) }) : tl("In {name}", { name: entry.termbase })}</span>
        </div>
      </div>
      <div className="detail-body">
        {may(tb, "edit") && (
          <>
            <p className="notice-plain">
              {tl("This is the master's entry. To change it, take it into the project: your version is used here and goes to the master's reviewers when you merge.")}
            </p>
            <div className="row-start">
              <button
                type="button"
                className="btn btn-small"
                onClick={() =>
                  void r.api!.overrideEntry(tb.name, entry.termbase, entry.id ?? "").then(
                    async (copy) => {
                      notify(tl("Took the entry into the project"));
                      const id = await r.openTermbase(tb.name);
                      await reload();
                      onTaken();
                      window.location.hash = `#/terminology/${id}/${id}:${copy.id}`;
                    },
                    (e) => notify(errorText(e)),
                  )
                }
              >
                <Pencil size={14} aria-hidden="true" />{" "}{tl("Change in this project")}
              </button>
              <ConfirmButton
                label={tl("Leave out of this project")}
                confirm={tl("Leave this entry out of the project? Merging asks the master's reviewers to delete it there.")}
                action={tl("Leave out")}
                onConfirm={() =>
                  void r.api!.leaveOut(tb.name, entry.termbase, entry.id ?? "").then(
                    async () => {
                      notify(tl("Left the entry out of the project"));
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
      notify(tl("Renamed to “{to}”", { to }));
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
      <Modal title={tl("Termbase settings")} onClose={onClose} wide>
        {problem ? (
          <p className="error" role="alert">
            {problem}
          </p>
        ) : (
          <p className="muted">{tl("Loading…")}</p>
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
    <Modal title={tl("Termbase settings")} onClose={onClose} wide>
      <label className="field">
        <span className="field-label">{tl("Name")}</span>
        <input value={name} onChange={(e) => setName(e.target.value)} onBlur={() => void rename()} onKeyDown={(e) => e.key === "Enter" && void rename()} />
      </label>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}

      <section className="settings-section" aria-labelledby="stb-langs">
        <h3 id="stb-langs" className="settings-title">
          {tl("Languages")}
        </h3>
        {langs.length === 0 ? (
          <p className="help">{tl("No languages yet. They are added when you add terms or import a file.")}</p>
        ) : (
          <ul className="settings-list">
            {langs.map((l) => (
              <li key={l}>
                <span>
                  {lang(l).name} <LangChip code={l} />
                </span>
                <span className="muted">{plural(termsIn(l), "term", "terms")}</span>
                <ConfirmButton
                  label={<span aria-label={tl("Remove {name}", { name: lang(l).name })}>{tl("Remove")}</span>}
                  confirm={termsIn(l) ? tl("Remove {name} and its {plural}?", { name: lang(l).name, plural: plural(termsIn(l), "term", "terms") }) : `Remove ${lang(l).name}?`}
                  onConfirm={() => void step(() => api.removeLanguage(current, l), `Removed ${lang(l).name}`)}
                />
              </li>
            ))}
          </ul>
        )}
        <form
          className="role-add"
          aria-label={tl("Add a language")}
          onSubmit={(e) => {
            e.preventDefault();
            if (!addLang) return;
            const l = addLang;
            setAddLang("");
            void step(() => api.setDefinition(current, { ...def, languages: [...langs, l] }), `Added ${lang(l).name}`);
          }}
        >
          <select value={addLang} aria-label={tl("Add a language")} onChange={(e) => setAddLang(e.target.value)}>
            <option value="">{tl("Add a language…")}</option>
            {LANGUAGES.filter((l) => !langs.includes(l.code)).map((l) => (
              <option key={l.code} value={l.code}>
                {l.name}
              </option>
            ))}
          </select>
          <button type="submit" className="btn btn-small" disabled={!addLang}>
            {tl("Add")}
          </button>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="stb-fields">
        <h3 id="stb-fields" className="settings-title">
          {tl("Fields")}
        </h3>
        {fields.length === 0 ? (
          <p className="help">{tl("No fields yet: entries hold their terms and each term's status.")}</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{tl("Name")}</th>
                <th scope="col">{tl("Applies to")}</th>
                <th scope="col">{tl("Values")}</th>
                <th scope="col">
                  <span className="sr-only">{tl("Actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {fields.map((f) => (
                <ServerField
                  key={f.name}
                  field={f}
                  onRename={(to) => void step(() => api.renameField(current, f.name, to), tl("Renamed the field to “{to}”", { to }))}
                  onChange={(next) =>
                    void step(() => api.setDefinition(current, { ...def, fields: fields.map((x) => (x.name === f.name ? next : x)) }), "")
                  }
                  onRemove={() => void step(() => api.removeField(current, f.name), tl("Removed the field “{name}”", { name: f.name }))}
                />
              ))}
            </tbody>
          </table>
        )}
        <AddFieldForm
          termbase={current}
          taken={fields.map((f) => f.name)}
          onAdd={(f) =>
            void step(
              () =>
                api.setDefinition(current, {
                  ...def,
                  fields: [...fields, { name: f.name, level: LEVEL_TO_CORE[f.level], kind: f.kind, ...(f.kind === "picklist" ? { values: f.values } : {}) }],
                }),
              tl("Added field “{name}” to {current}", { name: f.name, current }),
            )
          }
        />
      </section>

      {detail.kind === "project" ? (
        <section className="settings-section" aria-labelledby="stb-based">
          <h3 id="stb-based" className="settings-title">
            {tl("Based on")}
          </h3>
          <p className="help">{tl("The termbase shows the entries of these masters under its own, and its new and changed entries are submitted to them.")}</p>
          {[...new Set([...masters, ...upstreams])].map((m) => {
            const on = upstreams.includes(m);
            return (
              <label key={m} className="check" title={on && upstreams.length === 1 ? tl("It needs one master at least.") : undefined}>
                <input
                  type="checkbox"
                  checked={on}
                  disabled={on && upstreams.length === 1}
                  onChange={(e) =>
                    void step(
                      () => api.setUpstreams(current, e.target.checked ? [...upstreams, m] : upstreams.filter((x) => x !== m)),
                      e.target.checked ? tl("Now also based on {m}", { m }) : tl("No longer based on {m}", { m }),
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
            {tl("Review")}
          </h3>
          <p className="help">
            {tl("Projects submit their new and changed entries to this master, and its reviewers approve them. These rules say what is merged without waiting for them.")}
          </p>
          <label className="check">
            <input
              type="checkbox"
              checked={!rules.keepIdentical}
              onChange={(e) =>
                void putRules(
                  { ...rules, keepIdentical: e.target.checked ? undefined : true },
                  e.target.checked ? tl("Entries the master already has are merged without review") : tl("Entries the master already has wait for review, too"),
                )
              }
            />
            {tl("Entries the master already has as they are need no review")}
          </label>
          <span className="field-label">{tl("Fields that need no approval")}</span>
          {fields.length === 0 ? (
            <p className="help">{tl("The termbase has no fields yet. A change to notes or sources only could be merged at once.")}</p>
          ) : (
            <>
              <div className="row-start" role="group" aria-label={tl("Fields that need no approval")}>
                {fields.map((f) => (
                  <label key={f.name} className="check">
                    <input
                      type="checkbox"
                      checked={minor.some((m) => m.toLowerCase() === f.name.toLowerCase())}
                      onChange={(e) => {
                        const next = e.target.checked ? [...minor, f.name] : minor.filter((m) => m !== f.name);
                        void putRules(
                          { ...rules, minorFields: next.length ? next : undefined },
                          e.target.checked ? tl("Changes to {name} only are merged without review", { name: f.name }) : tl("Changes to {name} wait for review", { name: f.name }),
                        );
                      }}
                    />
                    {f.name}
                  </label>
                ))}
              </div>
              <p className="help">{tl("An entry changed in these fields only is merged at once.")}</p>
            </>
          )}
          <span className="field-label">{tl("Trusted authors")}</span>
          {trusted.length === 0 ? (
            <p className="help">{tl("Nobody yet: every entry waits for a reviewer.")}</p>
          ) : (
            <ul className="mr-reviewers">
              {trusted.map((t, i) => (
                <li key={t.author}>
                  <span className="cell-title">{nameOf(t.author)}</span>
                  <span className="cell-sub">
                    {t.scope.langs?.length ? tl("entries with terms in {join}", { join: t.scope.langs.map((l) => lang(l).name).join(", ") }) : tl("all entries")}
                  </span>
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    aria-label={tl("No longer trust {author}", { author: t.author })}
                    onClick={() => {
                      const next = trusted.filter((_, j) => j !== i);
                      void putRules({ ...rules, trusted: next.length ? next : undefined }, tl("The entries of {author} wait for review again", { author: t.author }));
                    }}
                  >
                    {tl("Remove")}
                  </button>
                </li>
              ))}
            </ul>
          )}
          <form
            className="role-add"
            aria-label={tl("Trust an author")}
            onSubmit={(e) => {
              e.preventDefault();
              if (!trustWho) {
                setProblem(tl("Say whose entries to trust."));
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
              void putRules({ ...rules, trusted: next }, tl("The entries of {who} are approved without review", { who }));
            }}
          >
            <label>
              <span className="field-label">{tl("Author")}</span>
              <select value={trustWho} onChange={(e) => setTrustWho(e.target.value)}>
                <option value="">{tl("Choose a person…")}</option>
                {r.people.map((p) => (
                  <option key={p.username} value={p.username}>
                    {nameOf(p.username)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="field-label">{tl("For")}</span>
              <select value={trustLang} onChange={(e) => setTrustLang(e.target.value)}>
                <option value="">{tl("All entries")}</option>
                {langs.map((l) => (
                  <option key={l} value={l}>
                    {tlx("Terms in {name}", { name: lang(l).name })}</option>
                ))}
              </select>
            </label>
            <button type="submit" className="btn btn-small">
              {tl("Trust")}
            </button>
          </form>
          <p className="help">
            {tl("Their new and changed entries are approved as they are submitted. An entry that conflicts with the master always waits for a reviewer.")}
          </p>
        </section>
      )}

      <section className="settings-section" aria-label={tl("Who may use it")}>
        <AccessPanel key={current} kind="termbase" resource={current} title={tl("Who may use it")} embedded />
      </section>

      <section className="settings-section settings-danger" aria-labelledby="stb-delete">
        <h3 id="stb-delete" className="settings-title">
          {tl("Delete termbase")}
        </h3>
        {based.length > 0 ? (
          <p className="help">
            {based.length === 1 ? tl("{name} is based on this termbase. Delete it, or base it on another master, first.", { name: based[0] }) : tl("{names} are based on this termbase. Delete them, or base them on another master, first.", { names: listOf(based) })}
          </p>
        ) : usedBy.length > 0 ? (
          <p className="help">
            {usedBy.length === 1 ? tl("{name} uses this termbase. Remove it from the project first.", { name: usedBy[0] }) : tl("{names} use this termbase. Remove it from the projects first.", { names: listOf(usedBy) })}
          </p>
        ) : (
          <>
            <p className="help">
              {detail.kind === "project"
                ? tl("Deletes the project's {length} and their history. The masters keep theirs. Merge what the masters should have first.", { length: plural(entries.length, "entry", "entries") })
                : tl("Deletes {length} and their history. Export the termbase first if you may need it again.", { length: plural(entries.length, "entry", "entries") })}
            </p>
            <ConfirmButton
              label={tl("Delete termbase")}
              confirm={tl("Delete {current} for good?", { current })}
              action={tl("Delete")}
              onConfirm={() =>
                void api.deleteTermbase(current).then(
                  () => {
                    notify(tl("Deleted {current}", { current }));
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
          {tl("Done")}
        </button>
      </div>
    </Modal>
  );
}

type FieldDef = NonNullable<TermbaseDetail["definition"]["fields"]>[number];

const LEVELS: Record<string, string> = { entry: tl("Entry"), language: tl("Language"), term: tl("Term") };

/** A field of a server termbase: its name, what kind of value it takes, and removing it. */
function ServerField({ field, onRename, onChange, onRemove }: { field: FieldDef; onRename: (to: string) => void; onChange: (f: FieldDef) => void; onRemove: () => void }) {
  const [name, setName] = useState(field.name);
  const rename = () => {
    const to = name.trim();
    if (to && to !== field.name) onRename(to);
    else setName(field.name);
  };
  const kind = (["text", "picklist", "number", "date", "boolean"] as const).find((k) => k === field.kind) ?? "text";
  return (
    <tr>
      <td>
        <input value={name} aria-label={tl("Name of {name}", { name: field.name })} onChange={(e) => setName(e.target.value)} onBlur={rename} onKeyDown={(e) => e.key === "Enter" && rename()} />
      </td>
      <td>{LEVELS[field.level] ?? field.level}</td>
      <td>
        <div className="settings-kind">
          <KindSelect
            label={tl("Values of {name}", { name: field.name })}
            value={kind}
            onChange={(k) => onChange(k === "picklist" ? { ...field, kind: k, values: field.values ?? [] } : { ...field, kind: k, values: undefined })}
          />
          {kind === "picklist" && (
            <ValuesEditor
              label={tl("The values of {name}, one a line", { name: field.name })}
              values={field.values ?? []}
              onChange={(values) => {
                if (values.join("\n") !== (field.values ?? []).join("\n")) onChange({ ...field, values });
              }}
            />
          )}
        </div>
      </td>
      <td className="cell-action">
        <ConfirmButton label={<span aria-label={tl("Remove {name}", { name: field.name })}>{tl("Remove")}</span>} confirm={tl("Remove “{name}” and its values?", { name: field.name })} onConfirm={onRemove} />
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
      <span>{tlx("Based on {join}. Shows only this project's new and changed entries.", { join: masters.join(", ") })}</span>
    </p>
  );
}
