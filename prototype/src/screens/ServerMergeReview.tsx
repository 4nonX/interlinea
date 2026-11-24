import { useCallback, useEffect, useState, type FormEvent } from "react";
import { AlertTriangle, Check, Clock, X } from "lucide-react";
import { useStore, useWords } from "../store";
import { errorText, useRemote } from "../remote";
import { allows, ApiError, type MergeAssignment, type MergeFilter, type MergeItem, type MergeProgress, type MergeRequest, type Person } from "../server";
import { lang } from "../data";
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

/** The part of a request a reviewer has, in words. */
function partText(scope: MergeFilter): string {
  const parts: string[] = [];
  if (scope.langs?.length) parts.push(`entries with terms in ${scope.langs.map((l) => lang(l).name).join(", ")}`);
  for (const k of scope.kinds ?? []) parts.push(k === "new" ? "new entries" : k === "changed" ? "changed entries" : "entries deleted in the project");
  if (scope.conflicts) parts.push("entries with conflicts");
  if (scope.authors?.length) parts.push(`entries by ${scope.authors.join(", ")}`);
  return parts.length ? parts.join("; ") : "all of it";
}

/** Who reviews which part of a request, and giving parts out. */
function Reviewers({
  id,
  mr,
  progress,
  reviewers,
  people,
  languages,
  manage,
  onChange,
}: {
  id: number;
  mr: MergeRequest;
  progress: MergeProgress;
  reviewers: MergeAssignment[];
  people: Person[];
  languages: string[];
  manage: boolean;
  onChange: () => Promise<void>;
}) {
  const { notify } = useStore();
  const r = useRemote();
  const [who, setWho] = useState("");
  const [part, setPart] = useState("");
  const open = mr.state === "open";
  const nameOf = (username: string) => people.find((p) => p.username === username)?.name || username;

  async function give(e: FormEvent) {
    e.preventDefault();
    if (!who) {
      notify("Choose who reviews.");
      return;
    }
    const scope: MergeFilter = part.startsWith("lang:")
      ? { langs: [part.slice(5)] }
      : part.startsWith("kind:")
        ? { kinds: [part.slice(5)] }
        : part === "conflicts"
          ? { conflicts: true }
          : {};
    try {
      await r.api!.assignReviewer(id, { reviewer: who, scope });
      notify(`${nameOf(who)} reviews a part of the request`);
      setWho("");
      setPart("");
      await onChange();
    } catch (err) {
      notify(errorText(err));
    }
  }

  return (
    <section className="panel" aria-labelledby="mr-reviewers">
      <h2 id="mr-reviewers" className="panel-title">
        Reviewers
      </h2>
      {reviewers.length === 0 ? (
        <p className="help">Nobody is given a part: everyone who may approve changes to {mr.target} sees all of it.</p>
      ) : (
        <ul className="mr-reviewers">
          {reviewers.map((a) => {
            const counts = progress.byReviewer?.[a.reviewer] ?? {};
            const all = Object.values(counts).reduce((n, c) => n + c, 0);
            const left = (counts.waiting ?? 0) + (counts.postponed ?? 0);
            return (
              <li key={a.reviewer}>
                <span className="cell-title">{nameOf(a.reviewer)}</span>
                <span className="cell-sub">
                  {partText(a.scope)}
                  {all > 0 ? `: ${left === 0 ? `all ${all} decided` : `${left} of ${all} to decide`}` : ""}
                </span>
                {open && manage && (
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    aria-label={`Take the part from ${nameOf(a.reviewer)}`}
                    onClick={() =>
                      void r.api!.unassignReviewer(id, a.reviewer).then(
                        async () => {
                          notify(`${nameOf(a.reviewer)} no longer has a part of the request`);
                          await onChange();
                        },
                        (err) => notify(errorText(err)),
                      )
                    }
                  >
                    Remove
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {open && manage && (
        <>
          <form className="role-add" onSubmit={give} aria-label="Give someone a part">
            <label>
              <span className="field-label">Who</span>
              <select value={who} onChange={(e) => setWho(e.target.value)}>
                <option value="">Choose someone</option>
                {people.map((p) => (
                  <option key={p.username} value={p.username}>
                    {p.name ? `${p.name} (${p.username})` : p.username}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="field-label">Part</span>
              <select value={part} onChange={(e) => setPart(e.target.value)}>
                <option value="">All of it</option>
                {languages.map((l) => (
                  <option key={l} value={`lang:${l}`}>
                    Terms in {lang(l).name}
                  </option>
                ))}
                <option value="kind:new">New entries</option>
                <option value="kind:changed">Changed entries</option>
                <option value="conflicts">Entries with conflicts</option>
              </select>
            </label>
            <button type="submit" className="btn">
              Give the part
            </button>
          </form>
          <p className="help">
            A reviewer must be able to approve changes to {mr.target}. Giving someone a part again replaces the one they had.
          </p>
        </>
      )}
    </section>
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
  const [limit, setLimit] = useState(50);
  const [mine, setMine] = useState(false);
  const [reviewers, setReviewers] = useState<MergeAssignment[]>([]);
  // The master: its languages, and what the signed-in person may do in it.
  const [target, setTarget] = useState<{ role: string; languages: string[] } | null>(null);
  // Notes for the authors, by entry, until they go with a decision.
  const [notes, setNotes] = useState<Record<number, string>>({});
  const me = r.conn?.user.username ?? "";

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      const states = view === "waiting" ? ["waiting"] : view === "decided" ? DECIDED : [];
      const [m, p, page, rs] = await Promise.all([
        r.api.mergeRequest(id),
        r.api.mergeProgress(id),
        r.api.mergeItems(id, states, 0, limit, mine ? me : ""),
        r.api.mergeReviewers(id),
      ]);
      setMr(m);
      setProgress(p);
      setItems(page.items);
      setTotal(page.total);
      setReviewers(rs);
      setProblem("");
      // Without a look at the master the request is still shown.
      r.api.termbase(m.target).then(
        (t) => setTarget({ role: t.role, languages: t.definition.languages ?? [] }),
        () => undefined,
      );
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api, id, view, limit, mine, me]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => e.type === "termbase" && void load()), [listen, load]);

  async function decide(filter: MergeFilter, state: string, edited?: CoreEntry, what?: string) {
    try {
      // What was written for the one entry goes with its decision.
      const one = filter.ids?.length === 1 ? filter.ids[0]! : 0;
      const comment = one ? (notes[one] ?? "").trim() : "";
      const res = await r.api!.decide(id, filter, { state, edited, comment: comment || undefined });
      if (one) setNotes(({ [one]: _sent, ...rest }) => rest);
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
  const approve = target ? allows(target.role, "approve") : false;
  const manage = target ? allows(target.role, "manage") : false;
  const assigned = reviewers.some((a) => a.reviewer === me);
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
        {open && target && !approve && <p className="help">Those who may approve changes to {mr.target} decide. You see how far they are.</p>}
        {open && approve && (
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

      <Reviewers id={id} mr={mr} progress={progress} reviewers={reviewers} people={r.people} languages={target?.languages ?? []} manage={manage} onChange={load} />

      <div className="tabs tabs-small" role="tablist" aria-label="Entries to show">
        {(["waiting", "decided", "all"] as View[]).map((v) => (
          <button
            key={v}
            type="button"
            role="tab"
            aria-selected={view === v}
            onClick={() => {
              setLimit(50);
              setView(v);
            }}
          >
            {v === "waiting" ? `Waiting (${waiting})` : v === "decided" ? "Decided" : "All"}
          </button>
        ))}
      </div>
      {(assigned || mine) && (
        <label className="check">
          <input
            type="checkbox"
            checked={mine}
            onChange={(e) => {
              setLimit(50);
              setMine(e.target.checked);
            }}
          />
          Only my part
        </label>
      )}

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
                  {it.reviewers?.length && (it.state === "waiting" || it.state === "postponed") ? `; for ${it.reviewers.join(", ")} to review` : ""}
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
                    {open && approve && (it.state === "waiting" || it.state === "postponed") && it.merged && (
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
              {(it.state === "reverted" || it.state === "rejected" || it.state === "changes-requested") && (
                <p className="cell-sub">
                  {it.state === "reverted" ? `Taken out of ${mr.target} again.` : `Not in ${mr.target}.`} It is submitted again once the project has changed it.
                </p>
              )}
              {open && approve && it.state.startsWith("approved") && it.rule !== "identical" && (
                <div>
                  <ConfirmButton
                    label="Undo the merge"
                    confirm={`Take this out of ${mr.target} again? The entry there goes back to what it was before.`}
                    action="Undo"
                    onConfirm={() =>
                      void r.api!.revertMergeItem(id, it.id).then(
                        async () => {
                          notify(`Took the merge back out of ${mr.target}`);
                          await load();
                        },
                        async (e) => {
                          notify(
                            e instanceof ApiError && e.status === 409
                              ? `The entry in ${mr.target} was changed after the merge. Change it there by hand.`
                              : errorText(e),
                          );
                          await load();
                        },
                      )
                    }
                  />
                </div>
              )}
              {open && approve && (it.state === "waiting" || it.state === "postponed") && (
                <input
                  className="mr-note"
                  value={notes[it.id] ?? ""}
                  placeholder="A note for the author, with your decision"
                  aria-label="A note for the author"
                  onChange={(e) => setNotes((n) => ({ ...n, [it.id]: e.target.value }))}
                />
              )}
              {open && approve && (it.state === "waiting" || it.state === "postponed") && (
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
                  {it.state !== "postponed" && (
                    <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "postponed")}>
                      <Clock size={14} aria-hidden="true" /> Postpone
                    </button>
                  )}
                  <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "rejected")}>
                    <X size={14} aria-hidden="true" /> Reject
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {total > items.length && (
        <p className="row">
          <span className="cell-sub">
            Showing the first {items.length} of {total}.
          </span>
          <button type="button" className="btn btn-small" onClick={() => setLimit((n) => n + 50)}>
            Show more
          </button>
        </p>
      )}
    </div>
  );
}
