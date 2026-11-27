// What a termbase kept in this browser offers besides its entries, as a
// termbase on the team server does: the versions of an entry to restore,
// the deleted entries to bring back, and under a project termbase the
// master's entries, to change for the project or leave out.

import { Pencil } from "lucide-react";
import type { Entry, Removal, Termbase } from "../data";
import { useLocalStore } from "../store";
import { toCoreEntry } from "../termio";
import { fmtTime } from "../format";
import { EntryTerms, LangChip } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { headOf, ReadEntry, termsLine } from "./ServerTermbase";
import { tl, tlCore } from "../i18n";

let seq = 0;
const newId = (p: string) => `${p}-${Date.now().toString(36)}-l${(seq++).toString(36)}`;

/** The versions of an entry, newest first; an earlier one is restored. */
export function LocalHistory({ tb, entry }: { tb: Termbase; entry: Entry }) {
  const { dispatch, notify } = useLocalStore();
  const newest = entry.history[entry.history.length - 1]?.id;
  return (
    <ol className="history">
      {[...entry.history].reverse().map((c) => (
        <li key={c.id}>
          <div className="history-summary">{c.summary ? tlCore(c.summary) : tl("Changed")}</div>
          <div className="history-meta">
            {c.by && c.project
              ? tl("{by}, {at} in {project}", { by: c.by, at: fmtTime(c.at), project: c.project })
              : c.by
                ? tl("{by}, {at}", { by: c.by, at: fmtTime(c.at) })
                : c.project
                  ? tl("{at} in {project}", { at: fmtTime(c.at), project: c.project })
                  : fmtTime(c.at)}
          </div>
          {c.snapshot && <div className="history-held">{termsLine(toCoreEntry({ ...entry, ...c.snapshot }, tb))}</div>}
          {c.snapshot && c.id !== newest && (
            <ConfirmButton
              plain
              label={tl("Restore this version")}
              confirm={tl("Set the entry back to this version? What it is now stays in the history.")}
              action={tl("Restore")}
              onConfirm={() => {
                dispatch({ type: "restoreVersion", entryId: entry.id, commitId: c.id, now: new Date().toISOString() });
                notify(tl("Restored the version of {at}", { at: fmtTime(c.at) }));
              }}
            />
          )}
        </li>
      ))}
    </ol>
  );
}

/** The deleted entries of a termbase kept in this browser, to bring back. */
export function LocalDeleted({ tb }: { tb: Termbase }) {
  const { state, dispatch, notify } = useLocalStore();
  const gone = state.deleted.filter((d) => d.entry.termbaseId === tb.id);
  if (gone.length === 0)
    return (
      <div className="empty">
        <p className="empty-title">{tl("Nothing deleted")}</p>
        <p>{tl("Entries you delete are kept here and can be brought back.")}</p>
      </div>
    );
  return (
    <ul className="deleted-list" aria-label={tl("Deleted entries of {name}", { name: tb.name })}>
      {gone.map((d) => {
        const core = toCoreEntry(d.entry, tb);
        return (
          <li key={d.entry.id} className="deleted-entry">
            <dl className="mr-terms">
              {core.languages
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
              {d.summary && d.summary !== "Deleted" && d.summary !== tl("Deleted")
                ? d.by
                  ? tl("Deleted {at} by {by}. {summary}", { at: fmtTime(d.at), by: d.by, summary: tlCore(d.summary) })
                  : tl("Deleted {at}. {summary}", { at: fmtTime(d.at), summary: tlCore(d.summary) })
                : d.by
                  ? tl("Deleted {at} by {by}", { at: fmtTime(d.at), by: d.by })
                  : tl("Deleted {at}", { at: fmtTime(d.at) })}</p>
            <button
              type="button"
              className="btn btn-small"
              onClick={() => {
                const head = headOf(core, tb.languages[0]);
                dispatch({ type: "restoreDeleted", entryId: d.entry.id, now: new Date().toISOString() });
                notify(head ? tl("Brought back “{head}”", { head }) : tl("Brought the entry back"));
              }}
            >
              {tl("Bring back")}
            </button>
          </li>
        );
      })}
    </ul>
  );
}

/** A master's entry a project termbase uses as it is, with the master it is in. */
export interface LocalMasterEntry {
  entry: Entry;
  master: Termbase;
}

/**
 * The entries of the masters a project termbase in this browser is based
 * on and uses as they are: those it has no version of its own of and does
 * not leave out. With them, the entries it leaves out.
 */
export function useLocalMasters(tb: Termbase): { masters: LocalMasterEntry[]; removals: (Removal & { head: string; master: string })[] } {
  const { state } = useLocalStore();
  if (tb.remote || tb.kind !== "project") return { masters: [], removals: [] };
  const based = (tb.basedOn ?? []).map((id) => state.termbases.find((t) => t.id === id && !t.remote)).filter((t): t is Termbase => !!t);
  const key = (masterId: string, entryId: string) => `${masterId}\n${entryId}`;
  const own = new Set(state.entries.filter((e) => e.termbaseId === tb.id && e.base).map((e) => key(e.base!.termbaseId, e.base!.entryId)));
  const left = state.removals.filter((x) => x.termbaseId === tb.id);
  const out = new Set(left.map((x) => key(x.masterId, x.entryId)));
  const masters: LocalMasterEntry[] = [];
  for (const master of based)
    for (const entry of state.entries)
      if (entry.termbaseId === master.id && !own.has(key(master.id, entry.id)) && !out.has(key(master.id, entry.id))) masters.push({ entry, master });
  const removals = left.flatMap((x) => {
    const master = based.find((t) => t.id === x.masterId);
    const entry = master && state.entries.find((e) => e.id === x.entryId && e.termbaseId === master.id);
    // An entry the master no longer has is left out of nothing.
    if (!master || !entry) return [];
    return [{ ...x, master: master.name, head: headOf(toCoreEntry(entry, master), tb.languages[0]) || tl("No term") }];
  });
  return { masters, removals };
}

/** The list of the masters' entries under a project termbase in this browser. */
export function LocalMasterList({
  tb,
  masters,
  removals,
  selected,
  onSelect,
}: {
  tb: Termbase;
  masters: LocalMasterEntry[];
  removals: (Removal & { head: string; master: string })[];
  selected: string;
  onSelect: (entryId: string) => void;
}) {
  const { dispatch, notify } = useLocalStore();
  return (
    <>
      <p className="help pane-bar">{tl("The master's entries this project uses as they are. Change one for this project only, or leave it out.")}</p>
      {masters.length === 0 && <p className="muted pane-pad">{tl("The masters have no entries the project does not have itself.")}</p>}
      <ul className="entry-list entry-list-fit" aria-label={tl("Entries of the masters of {name}", { name: tb.name })}>
        {masters.map(({ entry, master }) => {
          return (
            <li key={entry.id}>
              <button type="button" className="entry-row" aria-current={entry.id === selected ? "true" : undefined} onClick={() => onSelect(entry.id)}>
                <EntryTerms languages={tb.languages} terms={(l) => entry.terms[l] ?? []} />
                <span className="tb-count">{tl("In {name}", { name: master.name })}</span>
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
          <p className="history-meta">{tl("Merging deletes these in the master, if you accept it there.")}</p>
          <ul>
            {removals.map((x) => (
              <li key={`${x.masterId}\n${x.entryId}`}>
                <span className="entry-term">{x.head}</span>
                <span className="history-meta">
                  {(x.by ? tl("In {master}, left out {at} by {by}", { master: x.master, at: fmtTime(x.at), by: x.by }) : tl("In {master}, left out {at}", { master: x.master, at: fmtTime(x.at) }))}</span>
                <button
                  type="button"
                  className="btn btn-small"
                  onClick={() => {
                    dispatch({ type: "bringBack", termbaseId: tb.id, masterId: x.masterId, entryId: x.entryId });
                    notify(tl("Brought the entry back into the project"));
                  }}
                >
                  {tl("Bring back")}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

/** A master's entry under a project termbase in this browser: read, taken into the project, or left out. */
export function LocalMasterDetail({ tb, item, onTaken }: { tb: Termbase; item: LocalMasterEntry; onTaken: (entryId: string) => void }) {
  const { dispatch, notify } = useLocalStore();
  const { entry, master } = item;
  const core = toCoreEntry(entry, master);
  const changed = entry.history[entry.history.length - 1]?.at;
  return (
    <section className="pane pane-detail" aria-labelledby="master-entry-title">
      <div className="pane-head">
        <div className="pane-head-row">
          <h2 id="master-entry-title" className="pane-title">
            <EntryTerms
              languages={[...new Set([...tb.languages, ...core.languages.filter((x) => x.terms.length).map((x) => x.lang)])]}
              terms={(l) => core.languages.find((x) => x.lang === l)?.terms ?? []}
              big
            />
          </h2>
        </div>
        <div className="pane-sub">
          <span>
            {changed ? tl("In {name}, last changed {changed}", { name: master.name, changed: fmtTime(changed) }) : tl("In {name}", { name: master.name })}</span>
        </div>
      </div>
      <div className="detail-body">
        <p className="notice-plain">
          {tl("This is the master's entry. To change it, take it into the project: your version is used here and goes into the master when you merge.")}
        </p>
        <div className="row-start">
          <button
            type="button"
            className="btn btn-small"
            onClick={() => {
              const id = newId("e");
              dispatch({ type: "takeFromMaster", termbaseId: tb.id, masterEntryId: entry.id, id, now: new Date().toISOString() });
              notify(tl("Took the entry into the project"));
              onTaken(id);
            }}
          >
            <Pencil size={14} aria-hidden="true" />{" "}{tl("Change in this project")}
          </button>
          <ConfirmButton
            label={tl("Leave out of this project")}
            confirm={tl("Leave this entry out of the project? Merging deletes it in the master, if you accept it there.")}
            action={tl("Leave out")}
            onConfirm={() => {
              dispatch({ type: "leaveOut", termbaseId: tb.id, masterId: master.id, entryId: entry.id, now: new Date().toISOString() });
              notify(tl("Left the entry out of the project"));
            }}
          />
        </div>
        <ReadEntry entry={core} />
      </div>
    </section>
  );
}
