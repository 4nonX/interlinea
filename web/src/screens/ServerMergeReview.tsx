import { useCallback, useEffect, useState, type FormEvent } from "react";
import { AlertTriangle, Check, Clock, X } from "lucide-react";
import { useStore, useWords } from "../store";
import { errorText, useRemote } from "../remote";
import { allows, ApiError, type MergeAssignment, type MergeFilter, type MergeItem, type MergeProgress, type MergeRequest, type Person } from "../server";
import { lang } from "../data";
import type { CoreEntry } from "../termio";
import { fmtDate, listOf, plural } from "../format";
import { LangChip, PageHeader } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { tl, tlx } from "../i18n";

const KIND_TEXT: Record<MergeItem["kind"], string> = {
  new: tl("New entry"),
  changed: tl("Changed entry"),
  deleted: tl("Deleted in the project"),
};

const STATE_TEXT: Record<string, string> = {
  waiting: tl("Waiting"),
  approved: tl("Approved"),
  "approved-with-edits": tl("Approved with edits"),
  "changes-requested": tl("Changes requested"),
  rejected: tl("Rejected"),
  postponed: tl("Postponed"),
  reverted: tl("Reverted"),
};

type View = "waiting" | "decided" | "all";

/**
 * What goes into the master at the conflicting places if one side is
 * taken: the value itself for one conflict, how many places for several.
 */
function resolution(it: MergeItem, side: "ours" | "theirs"): string {
  const cs = it.conflicts ?? [];
  if (cs.length === 1) return tl("“{value}”", { value: cs[0]![side] || tl("nothing") });
  return plural(cs.length, "place", "places");
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
  if (scope.langs?.length) parts.push(tl("entries with terms in {languages}", { languages: scope.langs.map((l) => lang(l).name).join(", ") }));
  for (const k of scope.kinds ?? []) parts.push(k === "new" ? tl("new entries") : k === "changed" ? tl("changed entries") : tl("entries deleted in the project"));
  if (scope.conflicts) parts.push(tl("entries with conflicts"));
  if (scope.authors?.length) parts.push(tl("entries by {names}", { names: scope.authors.join(", ") }));
  return parts.length ? parts.join("; ") : tl("all of it");
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
      notify(tl("Choose who reviews."));
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
      notify(tl("{who} reviews a part of the request", { who: nameOf(who) }));
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
        {tl("Reviewers")}
      </h2>
      {reviewers.length === 0 ? (
        <p className="help">{tlx("Nobody is given a part: everyone who may approve changes to {target} sees all of it.", { target: mr.target })}</p>
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
                  {all > 0 ? `: ${left === 0 ? tl("all {all} decided", { all }) : tl("{left} of {all} to decide", { left, all })}` : ""}
                </span>
                {open && manage && (
                  <button
                    type="button"
                    className="btn btn-quiet btn-small"
                    aria-label={tl("Take the part from {reviewer}", { reviewer: nameOf(a.reviewer) })}
                    onClick={() =>
                      void r.api!.unassignReviewer(id, a.reviewer).then(
                        async () => {
                          notify(tl("{reviewer} no longer has a part of the request", { reviewer: nameOf(a.reviewer) }));
                          await onChange();
                        },
                        (err) => notify(errorText(err)),
                      )
                    }
                  >
                    {tl("Remove")}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {open && manage && (
        <>
          <form className="role-add" onSubmit={give} aria-label={tl("Give someone a part")}>
            <label>
              <span className="field-label">{tl("Who")}</span>
              <select value={who} onChange={(e) => setWho(e.target.value)}>
                <option value="">{tl("Choose someone")}</option>
                {people.map((p) => (
                  <option key={p.username} value={p.username}>
                    {p.name ? `${p.name} (${p.username})` : p.username}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="field-label">{tl("Part")}</span>
              <select value={part} onChange={(e) => setPart(e.target.value)}>
                <option value="">{tl("All of it")}</option>
                {languages.map((l) => (
                  <option key={l} value={`lang:${l}`}>
                    {tlx("Terms in {name}", { name: lang(l).name })}</option>
                ))}
                <option value="kind:new">{tl("New entries")}</option>
                <option value="kind:changed">{tl("Changed entries")}</option>
                <option value="conflicts">{tl("Entries with conflicts")}</option>
              </select>
            </label>
            <button type="submit" className="btn">
              {tl("Give the part")}
            </button>
          </form>
          <p className="help">
            {tlx("A reviewer must be able to approve changes to {target}. Giving someone a part again replaces the one they had.", { target: mr.target })}
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
        tl("{state}: {entries}", { state: what ?? STATE_TEXT[state]!, entries: plural(res.decided, "entry", "entries") }) +
          (left ? ". " + plural(left, "entry needs a decision one by one (the master changed the same places).", "entries need a decision one by one (the master changed the same places).") : ""),
      );
      await load();
    } catch (e) {
      notify(e instanceof ApiError && e.status === 403 ? tl("{message} Ask a manager of {termbase} for the approve role.", { message: e.message, termbase: mr?.target }) : errorText(e));
    }
  }

  if (!r.conn)
    return (
      <div className="page page-narrow">
        <PageHeader title={tl("Review")} />
        <p>
          {tlx("Sign in to the team server in {a}.", { a: <a href="#/settings">{tl("Settings")}</a> })}
        </p>
      </div>
    );
  if (problem && !mr)
    return (
      <div className="page page-narrow">
        <PageHeader title={tl("Review")} />
        <p className="error" role="alert">
          {problem}
        </p>
      </div>
    );
  if (!mr || !progress)
    return (
      <p className="boot" role="status">
        {tl("Loading the request…")}
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
          {tl("Back to the termbase")}
        </a>
      </PageHeader>
      <p className="help">
        {mr.deadline
          ? tl("{project} into {target}. Submitted by {createdBy} on {createdAt}, to be decided by {deadline}.", { project: mr.project, target: mr.target, createdBy: mr.createdBy, createdAt: fmtDate(mr.createdAt), deadline: fmtDate(mr.deadline) })
          : tl("{project} into {target}. Submitted by {createdBy} on {createdAt}.", { project: mr.project, target: mr.target, createdBy: mr.createdBy, createdAt: fmtDate(mr.createdAt) })}
        {!open && " " + tl("Closed on {date}.", { date: fmtDate(mr.closedAt ?? mr.createdAt) })}</p>
      <section className="panel" aria-labelledby="mr-progress">
        <h2 id="mr-progress" className="panel-title">
          {tl("Progress")}
        </h2>
        <p>
          {tl("{entries}: {states}.", {
            entries: plural(progress.total, "entry", "entries"),
            states: Object.entries(progress.states)
              .filter(([, n]) => n > 0)
              .map(([s, n]) => tl("{state} {n}", { state: STATE_TEXT[s] ?? s, n }))
              .join(", "),
          })}
          {progress.conflicts > 0 && " " + plural(progress.conflicts, "conflict with the master.", "conflicts with the master.")}
        </p>
        <progress max={progress.total || 1} value={progress.total - waiting} aria-label={tl("Decided")} />
        {open && target && !approve && <p className="help">{tlx("Those who may approve changes to {target} decide. You see how far they are.", { target: mr.target })}</p>}
        {open && approve && (
          <div className="server-actions">
            <button
              type="button"
              className="btn btn-primary"
              disabled={waiting === 0}
              onClick={() => void decide({ states: ["waiting"], conflicts: false }, "approved", undefined, tl("Approved all waiting entries without conflicts"))}
            >
              <Check size={15} aria-hidden="true" />{" "}{tl("Approve all waiting without conflicts")}
            </button>
            <ConfirmButton
              small={false}
              label={tl("Close the request")}
              confirm={waiting ? plural(waiting, "entry is still waiting. Close anyway?", "entries are still waiting. Close anyway?") : tl("Close the request?")}
              action={tl("Close")}
              onConfirm={() =>
                void r.api!.closeMergeRequest(id).then(
                  async () => {
                    notify(tl("Closed the request"));
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

      <div className="tabs tabs-small" role="tablist" aria-label={tl("Entries to show")}>
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
            {v === "waiting" ? tl("Waiting ({waiting})", { waiting }) : v === "decided" ? tl("Decided") : tl("All")}
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
          {tl("Only my part")}
        </label>
      )}

      {items.length === 0 ? (
        <p className="muted">{view === "waiting" ? tl("Nothing is waiting.") : tl("No entries here.")}</p>
      ) : (
        <ul className="mr-items">
          {items.map((it) => (
            <li key={it.id} className="panel mr-item">
              <div className="mr-head">
                <span className="cell-title">{KIND_TEXT[it.kind]}</span>
                <span className="cell-sub">
                  {it.reviewers?.length && (it.state === "waiting" || it.state === "postponed")
                    ? tl("by {author}, {changedAt}; for {reviewers} to review", { author: it.author, changedAt: fmtDate(it.changedAt), reviewers: listOf(it.reviewers) })
                    : tl("by {author}, {changedAt}", { author: it.author, changedAt: fmtDate(it.changedAt) })}</span>
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
                    <p className="notice-title">{tl("The master changed the same places")}</p>
                    <ul className="mr-changes">
                      {it.conflicts.map((c, i) => (
                        <li key={i}>
                          {c.base
                            ? tl("{path}: the project has “{ours}”, {target} has “{theirs}” (was “{base}”)", { path: c.path, ours: c.ours || tl("nothing"), target: mr.target, theirs: c.theirs || tl("nothing"), base: c.base })
                            : tl("{path}: the project has “{ours}”, {target} has “{theirs}”", { path: c.path, ours: c.ours || tl("nothing"), target: mr.target, theirs: c.theirs || tl("nothing") })}</li>
                      ))}
                    </ul>
                    {open && approve && (it.state === "waiting" || it.state === "postponed") && it.merged && (
                      <p className="cell-sub">
                        {tlx("Choose which of the two goes into {target}. What only one side changed is merged either way.", { target: mr.target })}
                      </p>
                    )}
                  </div>
                </div>
              )}
              {it.forbidden && <p className="cell-sub cell-warn">{tl("It has a forbidden term.")}</p>}
              {it.duplicates && it.duplicates.length > 0 && (
                <p className="cell-sub cell-warn">{plural(it.duplicates.length, "master entry shares a term with it.", "master entries share a term with it.")}</p>
              )}
              {it.comment && <p className="cell-sub">“{it.comment}”{it.reviewer ? `, ${it.reviewer}` : ""}</p>}
              {(it.state === "reverted" || it.state === "rejected" || it.state === "changes-requested") && (
                <p className="cell-sub">
                  {it.state === "reverted"
                    ? tl("Taken out of {target} again. It is submitted again once the project has changed it.", { target: mr.target })
                    : tl("Not in {target}. It is submitted again once the project has changed it.", { target: mr.target })}
                </p>
              )}
              {open && approve && it.state.startsWith("approved") && it.rule !== "identical" && (
                <div>
                  <ConfirmButton
                    label={tl("Undo the merge")}
                    confirm={tl("Take this out of {target} again? The entry there goes back to what it was before.", { target: mr.target })}
                    action={tl("Undo")}
                    onConfirm={() =>
                      void r.api!.revertMergeItem(id, it.id).then(
                        async () => {
                          notify(tl("Took the merge back out of {target}", { target: mr.target }));
                          await load();
                        },
                        async (e) => {
                          notify(
                            e instanceof ApiError && e.status === 409
                              ? tl("The entry in {target} was changed after the merge. Change it there by hand.", { target: mr.target })
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
                  placeholder={tl("A note for the author, with your decision")}
                  aria-label={tl("A note for the author")}
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
                        onClick={() => void decide({ ids: [it.id] }, "approved-with-edits", it.mergedOurs ?? it.entry, tl("Used the project version ({resolution})", { resolution: resolution(it, "ours") }))}
                      >
                        <Check size={14} aria-hidden="true" /> {w("useProject")}: {resolution(it, "ours")}
                      </button>
                      <button
                        type="button"
                        className="btn btn-small"
                        onClick={() => void decide({ ids: [it.id] }, "approved-with-edits", it.merged, tl("Kept the master version ({resolution})", { resolution: resolution(it, "theirs") }))}
                      >
                        <Check size={14} aria-hidden="true" /> {w("keepMaster")}: {resolution(it, "theirs")}
                      </button>
                    </>
                  ) : (
                    <button type="button" className="btn btn-small btn-primary" onClick={() => void decide({ ids: [it.id] }, "approved")}>
                      <Check size={14} aria-hidden="true" />{" "}{tl("Approve")}
                    </button>
                  )}
                  {it.state !== "postponed" && (
                    <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "postponed")}>
                      <Clock size={14} aria-hidden="true" />{" "}{tl("Postpone")}
                    </button>
                  )}
                  <button type="button" className="btn btn-small" onClick={() => void decide({ ids: [it.id] }, "rejected")}>
                    <X size={14} aria-hidden="true" />{" "}{tl("Reject")}
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
            {tlx("Showing the first {length} of {total}.", { length: items.length, total: total })}
          </span>
          <button type="button" className="btn btn-small" onClick={() => setLimit((n) => n + 50)}>
            {tl("Show more")}
          </button>
        </p>
      )}
    </div>
  );
}
