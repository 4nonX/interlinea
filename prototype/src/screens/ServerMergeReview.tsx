import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Check, Clock, X } from "lucide-react";
import { useStore, useWords } from "../store";
import { errorText, useRemote } from "../remote";
import { ApiError, type MergeFilter, type MergeItem, type MergeProgress, type MergeRequest } from "../server";
import type { CoreEntry } from "../termio";
import { fmtDate, plural } from "../format";
import { LangChip, PageHeader } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";

const KIND_TEXT: Record<MergeItem["kind"], string> = {
  new: "New entry",
  changed: "Changed entry",
  deleted: "Deleted in the project",
};

const STATE_TEXT: Record<string, string> = {
  waiting: "Waiting",
  approved: "Approved",
  "approved-with-edits": "Approved with edits",
  "changes-requested": "Changes requested",
  rejected: "Rejected",
  postponed: "Postponed",
  reverted: "Reverted",
};

type View = "waiting" | "decided" | "all";

/**
 * What goes into the master at the conflicting places if one side is
 * taken: the value itself for one conflict, how many places for several.
 */
function resolution(it: MergeItem, side: "ours" | "theirs"): string {
  const cs = it.conflicts ?? [];
  if (cs.length === 1) return `“${cs[0]![side] || "nothing"}”`;
  return `${cs.length} places`;
}
const DECIDED = ["approved", "approved-with-edits", "changes-requested", "rejected", "postponed", "reverted"];

function Terms({ entry }: { entry: CoreEntry }) {
  return (
    <dl className="mr-terms">
      {entry.languages.map((l) => (
        <div key={l.lang}>
          <dt>
            <LangChip code={l.lang} />
          </dt>
          <dd>{l.terms.map((t) => t.text).join("; ")}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * A merge request on the team server: a project termbase's changes to a
 * master, decided one by one or many at once by the master's reviewers.
 */
export function ServerMergeReview({ id }: { id: number }) {
  const { notify } = useStore();
  const w = useWords();
  const r = useRemote();
  const [mr, setMr] = useState<MergeRequest | null>(null);
  const [progress, setProgress] = useState<MergeProgress | null>(null);
  const [items, setItems] = useState<MergeItem[]>([]);
  const [total, setTotal] = useState(0);
  const [view, setView] = useState<View>("waiting");
  const [problem, setProblem] = useState("");

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      const states = view === "waiting" ? ["waiting"] : view === "decided" ? DECIDED : [];
      const [m, p, page] = await Promise.all([r.api.mergeRequest(id), r.api.mergeProgress(id), r.api.mergeItems(id, states, 0)]);
      setMr(m);
      setProgress(p);
      setItems(page.items);
      setTotal(page.total);
      setProblem("");
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api, id, view]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => e.type === "termbase" && void load()), [listen, load]);

  async function decide(filter: MergeFilter, state: string, edited?: CoreEntry, what?: string) {
    try {
      const res = await r.api!.decide(id, filter, { state, edited });
      const left = res.needResolution?.length ?? 0;
      notify(
        `${what ?? STATE_TEXT[state]}: ${plural(res.decided, "entry", "entries")}` +
          (left ? `; ${plural(left, "entry needs", "entries need")} a decision one by one (the master changed the same places)` : ""),
      );
      await load();
    } catch (e) {
      notify(e instanceof ApiError && e.status === 403 ? `${e.message} Ask a manager of ${mr?.target} for the approve role.` : errorText(e));
    }
  }

  if (!r.conn)
    return (
      <div className="page page-narrow">
        <PageHeader title="Review" />
        <p>
          Sign in to the team server in <a href="#/settings">Settings</a>.
        </p>
      </div>
    );
  if (problem && !mr)
    return (
      <div className="page page-narrow">
        <PageHeader title="Review" />
        <p className="error" role="alert">
          {problem}
        </p>
      </div>
    );
  if (!mr || !progress)
    return (
      <p className="boot" role="status">
        Loading the request…
      </p>
    );
  const open = mr.state === "open";
  const waiting = progress.states.waiting ?? 0;
  return (
    <div className="page">
      <PageHeader title={mr.title}>
        <a className="btn btn-quiet" href={`#/terminology/srv:${encodeURIComponent(mr.project)}`}>
          Back to the termbase
        </a>
      </PageHeader>
      <p className="help">
        {mr.project} into {mr.target}. Submitted by {mr.createdBy} on {fmtDate(mr.createdAt)}
        {mr.deadline ? `, to be decided by ${fmtDate(mr.deadline)}` : ""}.{" "}
        {open ? "" : `Closed on ${fmtDate(mr.closedAt ?? mr.createdAt)}.`}
      </p>
      <section className="panel" aria-labelledby="mr-progress">
        <h2 id="mr-progress" className="panel-title">
          Progress
        </h2>
        <p>
          {plural(progress.total, "entry", "entries")}: {Object.entries(progress.states)
            .filter(([, n]) => n > 0)
            .map(([s, n]) => `${n} ${(STATE_TEXT[s] ?? s).toLowerCase()}`)
            .join(", ")}
          {progress.conflicts ? `; ${plural(progress.conflicts, "conflict", "conflicts")} with the master` : ""}.
        </p>
        <progress max={progress.total || 1} value={progress.total - waiting} aria-label="Decided" />
        {open && (
          <div className="server-actions">
            <button
              type="button"
              className="btn btn-primary"
              disabled={waiting === 0}
              onClick={() => void decide({ states: ["waiting"], conflicts: false }, "approved", undefined, "Approved all waiting entries without conflicts")}
            >
              <Check size={15} aria-hidden="true" /> Approve all waiting without conflicts
            </button>
            <ConfirmButton
              small={false}
              label="Close the request"
              confirm={waiting ? `${plural(waiting, "entry is", "entries are")} still waiting. Close anyway?` : "Close the request?"}
              action="Close"
              onConfirm={() =>
                void r.api!.closeMergeRequest(id).then(
                  async () => {
                    notify("Closed the request");
                    await load();
                  },
                  (e) => notify(errorText(e)),
                )
              }
            />
          </div>
        )}
      </section>

      <div className="tabs tabs-small" role="tablist" aria-label="Entries to show">
        {(["waiting", "decided", "all"] as View[]).map((v) => (
          <button key={v} type="button" role="tab" aria-selected={view === v} onClick={() => setView(v)}>
            {v === "waiting" ? `Waiting (${waiting})` : v === "decided" ? "Decided" : "All"}
          </button>
        ))}
      </div>

      {items.length === 0 ? (
        <p className="muted">{view === "waiting" ? "Nothing is waiting." : "No entries here."}</p>
      ) : (
        <ul className="mr-items">
          {items.map((it) => (
            <li key={it.id} className="panel mr-item">
              <div className="mr-head">
                <span className="cell-title">{KIND_TEXT[it.kind]}</span>
                <span className="cell-sub">
                  by {it.author}, {fmtDate(it.changedAt)}
                </span>
                <span className={`status ${it.state === "waiting" ? "status-proposed" : it.state.startsWith("approved") ? "status-approved" : "status-forbidden"}`}>
                  {STATE_TEXT[it.state] ?? it.state}
                </span>
              </div>
              <Terms entry={it.entry} />
              {it.changes && it.changes.length > 0 && it.kind === "changed" && (
                <ul className="mr-changes">
                  {it.changes.map((c, i) => (
                    <li key={i}>
                      <span className="cell-sub">{c.path}:</span> <del>{c.old || "nothing"}</del> → <ins>{c.new || "nothing"}</ins>
                    </li>
                  ))}
                </ul>
              )}
              {it.conflicts && it.conflicts.length > 0 && (
                <div className="notice" role="note">
                  <AlertTriangle size={16} aria-hidden="true" />
                  <div className="notice-body">
                    <p className="notice-title">The master changed the same places</p>
                    <ul className="mr-changes">
                      {it.conflicts.map((c, i) => (
                        <li key={i}>
                          {c.path}: the project has “{c.ours || "nothing"}”, {mr.target} has “{c.theirs || "nothing"}”
                          {c.base ? ` (was “${c.base}”)` : ""}
                        </li>
                      ))}
                    </ul>
                    {open && it.state === "waiting" && it.merged && (
                      <p className="cell-sub">
                        Choose which of the two goes into {mr.target}. What only one side changed is merged either way.
                      </p>
                    )}
                  </div>
                </div>
              )}
              {it.forbidden && <p className="cell-sub cell-warn">It has a forbidden term.</p>}
              {it.duplicates && it.duplicates.length > 0 && (
                <p className="cell-sub cell-warn">{plural(it.duplicates.length, "master entry shares", "master entries share")} a term with it.</p>
              )}
              {it.comment && <p className="cell-sub">“{it.comment}”{it.reviewer ? `, ${it.reviewer}` : ""}</p>}
              {open && it.state === "waiting" && (
                <div className="server-actions">
                  {it.conflicts?.length && it.merged ? (
                    <>
                      <button
                        type="button"
                        className="btn btn-small"
                        onClick={() => void decide({ ids: [it.id] }, "approved-with-edits", it.mergedOurs ?? it.entry, `Used the project version (${resolution(it, "ours")})`)}
                      >
                        <Check size={14} aria-hidden="true" /> {w("useProject")}: {resolution(it, "ours")}
                      </button>
                      <button
                        type="button"
                        className="btn btn-small"
                        onClick={() => void decide({ ids: [it.id] }, "approved-with-edits", it.merged, `Kept the master version (${resolution(it, "theirs")})`)}
                      >
                        <Check size={14} aria-hidden="true" /> {w("keepMaster")}: {resolution(it, "theirs")}
                      </button>
                    </>
                  ) : (
                    <button type="button" className="btn btn-small btn-primary" onClick={() => void decide({ ids: [it.id] }, "approved")}>
                      <Check size={14} aria-hidden="true" /> Approve
                    </button>
                  )}
                  <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "postponed")}>
                    <Clock size={14} aria-hidden="true" /> Postpone
                  </button>
                  <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "rejected")}>
                    <X size={14} aria-hidden="true" /> Reject
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {total > items.length && <p className="cell-sub">Showing the first {items.length} of {total}.</p>}
    </div>
  );
}
