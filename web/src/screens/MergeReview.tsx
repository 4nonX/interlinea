import { useMemo, useState } from "react";
import { ArrowDownToLine, CheckCircle2, GitMerge } from "lucide-react";
import type { Entry, Termbase } from "../data";
import { useLocalStore, useWords } from "../store";
import {
  computeChanges,
  incomingFromMaster,
  type Change,
  type ChangeKind,
  type Decision,
} from "../merge";
import { fmtDate, fmtTime, plural } from "../format";
import { InLang, LangChip, LocationBadge, PageHeader } from "../components/ui";

export function MergeReview({ projectId }: { projectId: string }) {
  const { state } = useLocalStore();
  const w = useWords();
  const project = state.projects.find((p) => p.id === projectId);
  const masters = (project?.setup.masters ?? [])
    .map((id) => state.termbases.find((t) => t.id === id)!)
    .filter(Boolean);
  const [masterId, setMasterId] = useState(masters[0]?.id ?? "");

  if (
    !project ||
    project.setup.mode !== "layered" ||
    !project.setup.projectTermbaseId
  ) {
    return (
      <div className="page page-narrow">
        <PageHeader title={w("reviewMerge")} />
        <div className="empty">
          <p className="empty-title">Nothing to merge</p>
          <p>This project writes directly into its master or has no master.</p>
          <a className="btn btn-small" href="#/projects">
            Back to projects
          </a>
        </div>
      </div>
    );
  }
  const master = masters.find((m) => m.id === masterId) ?? masters[0]!;
  return (
    <div className="page">
      <PageHeader
        title={`${w("reviewMerge")}: ${project.name}`}
        description={
          <>
            New and changed terms from this project go into its master. The
            master changes only when you merge.
          </>
        }
      >
        <a className="btn btn-quiet" href="#/projects">
          Back to projects
        </a>
      </PageHeader>

      {masters.length > 1 && (
        <div className="tabs" role="tablist" aria-label="Masters">
          {masters.map((m) => (
            <button
              key={m.id}
              type="button"
              role="tab"
              aria-selected={m.id === master.id}
              onClick={() => setMasterId(m.id)}
            >
              {m.name}
            </button>
          ))}
        </div>
      )}
      <MergeIntoMaster key={master.id} projectId={project.id} master={master} />
    </div>
  );
}

function MergeIntoMaster({
  projectId,
  master,
}: {
  projectId: string;
  master: Termbase;
}) {
  const { state, dispatch, notify } = useLocalStore();
  const w = useWords();
  const project = state.projects.find((p) => p.id === projectId)!;
  const ptbId = project.setup.projectTermbaseId!;
  const branchedAt = state.branchedAt[project.id] ?? project.createdAt;
  const masterEntries = state.entries.filter((e) => e.termbaseId === master.id);
  const projectEntries = state.entries.filter((e) => e.termbaseId === ptbId);

  const changes = useMemo(
    () =>
      computeChanges(
        projectEntries,
        master,
        masterEntries,
        state.baseSnapshots,
        branchedAt,
        state.removals.filter((x) => x.termbaseId === ptbId),
      ),
    [projectEntries, master, masterEntries, state.baseSnapshots, branchedAt, state.removals, ptbId],
  );
  const incoming = useMemo(
    () => incomingFromMaster(masterEntries, branchedAt),
    [masterEntries, branchedAt],
  );
  const [decisions, setDecisions] = useState<Record<string, Decision>>({});
  const [filter, setFilter] = useState<ChangeKind | "all">("all");
  const [done, setDone] = useState<number | null>(null);

  const counts = { conflict: 0, changed: 0, new: 0, removed: 0 } as Record<
    ChangeKind,
    number
  >;
  for (const c of changes) counts[c.kind]++;
  const decided = (c: Change): Decision | undefined =>
    decisions[c.project.id] ?? (c.kind === "conflict" ? undefined : "accept");
  const unresolved = changes.filter(
    (c) => c.kind === "conflict" && !decided(c),
  ).length;
  const included = changes.filter(
    (c) => decided(c) && decided(c) !== "skip",
  ).length;
  const shown = changes.filter((c) => filter === "all" || c.kind === filter);
  // What the master's rules cover is listed apart: it needs no review.
  const toReview = shown.filter((c) => !c.rule);
  const covered = shown.filter((c) => c.rule);

  const incomingCount = incoming.created.length + incoming.changed.length;

  if (done !== null) {
    return (
      <div className="panel done">
        <CheckCircle2 size={28} aria-hidden="true" className="done-icon" />
        <h2 className="panel-title" tabIndex={-1} ref={(el) => el?.focus()}>
          {w("merged")} {plural(done, w("change"), w("changes"))} into{" "}
          {master.name}
        </h2>
        <p>
          Every merged entry keeps its {w("history").toLowerCase()}: who added
          it, when, and the sentence it came from.
        </p>
        <div className="row">
          <a className="btn btn-primary" href={`#/terminology/${master.id}`}>
            Open {master.name}
          </a>
          <a className="btn btn-quiet" href="#/projects">
            Back to projects
          </a>
        </div>
      </div>
    );
  }

  return (
    <>
      <section className="merge-head" aria-label="Termbases">
        <div className="merge-side">
          <span className="merge-label">From</span>
          <span className="cell-title">{project.name}</span>
          <span className="cell-sub">
            {w("projectTermbase")}{" "}
            <LocationBadge location={project.setup.location} />
          </span>
        </div>
        <GitMerge size={20} aria-hidden="true" className="merge-arrow" />
        <div className="merge-side">
          <span className="merge-label">Into</span>
          <span className="cell-title">{master.name}</span>
          <span className="cell-sub">
            Master{" "}
            <LocationBadge location={master.location} server={master.server} />
          </span>
        </div>
        <dl className="merge-counts">
          <div>
            <dt>{w("conflict")}s</dt>
            <dd className={counts.conflict ? "count-conflict" : undefined}>
              {counts.conflict}
            </dd>
          </div>
          <div>
            <dt>{w("changedEntry")}</dt>
            <dd>{counts.changed}</dd>
          </div>
          <div>
            <dt>New</dt>
            <dd>{counts.new}</dd>
          </div>
          {counts.removed > 0 && (
            <div>
              <dt>Left out</dt>
              <dd>{counts.removed}</dd>
            </div>
          )}
        </dl>
      </section>

      {incomingCount > 0 && (
        <section className="notice" aria-labelledby="incoming-title">
          <ArrowDownToLine size={18} aria-hidden="true" />
          <div className="notice-body">
            <h2 id="incoming-title" className="notice-title">
              {master.name} has {plural(incomingCount, "change", "changes")}{" "}
              since {fmtDate(branchedAt)}
            </h2>
            <ul className="incoming">
              {[...incoming.created, ...incoming.changed].map((e) => (
                <li key={e.id}>
                  <IncomingItem
                    entry={e}
                    master={master}
                    created={incoming.created.includes(e)}
                  />
                </li>
              ))}
            </ul>
          </div>
          <button
            type="button"
            className="btn btn-small"
            onClick={() => {
              dispatch({
                type: "updateFromMaster",
                projectId: project.id,
                now: new Date().toISOString(),
              });
              notify(w("updated"));
            }}
          >
            {w("updateFromMaster")}
          </button>
        </section>
      )}

      {changes.length === 0 ? (
        <div className="empty">
          <p className="empty-title">No {w("changes")} to merge</p>
          <p>Terms you add or change in this project appear here.</p>
        </div>
      ) : (
        <>
          <div className="segmented" role="radiogroup" aria-label="Show">
            {(["all", "conflict", "changed", "new", ...(counts.removed ? (["removed"] as const) : [])] as const).map((k) => (
              <label key={k} className="segment">
                <input
                  type="radio"
                  name="filter"
                  checked={filter === k}
                  onChange={() => setFilter(k)}
                />
                <span>
                  {k === "all"
                    ? "All"
                    : k === "conflict"
                      ? `${w("conflict")}s`
                      : k === "changed"
                        ? w("changedEntry")
                        : k === "removed"
                          ? "Left out"
                          : "New"}{" "}
                  ({k === "all" ? changes.length : counts[k]})
                </span>
              </label>
            ))}
          </div>

          {toReview.length > 0 && (
            <ul className="changes">
              {toReview.map((c) => (
                <li key={c.project.id}>
                  <ChangeItem
                    change={c}
                    master={master}
                    decision={decided(c)}
                    onDecide={(d) =>
                      setDecisions((x) => ({ ...x, [c.project.id]: d }))
                    }
                  />
                </li>
              ))}
            </ul>
          )}
          {covered.length > 0 && (
            <section className="changes-covered" aria-labelledby="covered-title">
              <h2 id="covered-title" className="tb-group-title">
                Need no review ({covered.length})
              </h2>
              <p className="help">
                The rules of {master.name} cover these. They are merged with
                the rest unless you take them out.
              </p>
              <ul className="changes">
                {covered.map((c) => (
                  <li key={c.project.id}>
                    <ChangeItem
                      change={c}
                      master={master}
                      decision={decided(c)}
                      onDecide={(d) =>
                        setDecisions((x) => ({ ...x, [c.project.id]: d }))
                      }
                    />
                  </li>
                ))}
              </ul>
            </section>
          )}

          <div className="merge-bar">
            <p className="merge-bar-text" aria-live="polite">
              {unresolved > 0
                ? `Resolve ${plural(unresolved, "conflict", "conflicts")} to merge.`
                : `${plural(included, w("change"), w("changes"))} will be merged. What you do not accept stays in the project.`}
            </p>
            <button
              type="button"
              className="btn btn-primary"
              disabled={unresolved > 0 || included === 0}
              onClick={() => {
                dispatch({
                  type: "merge",
                  projectId: project.id,
                  masterId: master.id,
                  changes,
                  decisions,
                  now: new Date().toISOString(),
                });
                notify(
                  `${w("merged")} ${plural(included, w("change"), w("changes"))} into ${master.name}`,
                );
                setDone(included);
              }}
            >
              <GitMerge size={16} aria-hidden="true" />
              {w("mergeInto")} {master.name}
            </button>
          </div>
        </>
      )}
    </>
  );
}

function IncomingItem({
  entry,
  master,
  created,
}: {
  entry: Entry;
  master: Termbase;
  created: boolean;
}) {
  const src = master.languages[0]!;
  const last = entry.history[entry.history.length - 1]!;
  return (
    <>
      <InLang code={src} className="entry-term">
        {entry.terms[src]?.[0]?.text}
      </InLang>{" "}
      <span className="muted">
        {created
          ? "new"
          : last.summary.charAt(0).toLowerCase() + last.summary.slice(1)}
        , {last.by}, {fmtDate(last.at)}
      </span>
    </>
  );
}

function TermsLine({ entry, code }: { entry: Entry; code: string }) {
  const ts = entry.terms[code] ?? [];
  if (!ts.length) return <span className="missing">no term</span>;
  return (
    <InLang code={code}>
      {ts.map((t, i) => (
        <span
          key={t.text}
          className={t.status === "forbidden" ? "term-forbidden" : undefined}
        >
          {i > 0 && ", "}
          {t.text}
        </span>
      ))}
    </InLang>
  );
}

function ChangeItem({
  change,
  master,
  decision,
  onDecide,
}: {
  change: Change;
  master: Termbase;
  decision?: Decision;
  onDecide: (d: Decision) => void;
}) {
  const w = useWords();
  const src = master.languages[0]!;
  const head = change.project.terms[src]?.[0]?.text ?? "(no term)";
  const field = (k: string) =>
    master.infoTypes.find((i) => i.id === k)?.name ?? k;
  const kindLabel =
    change.kind === "conflict"
      ? w("conflict")
      : change.kind === "changed"
        ? w("changedEntry")
        : change.kind === "removed"
          ? "Left out of the project"
          : w("newEntry");
  const fieldNames = change.fields.map((f) => field(f.field)).join(", ");
  const ruleNote =
    change.rule === "identical"
      ? `${master.name} has this entry as it is: the two are linked, nothing is added.`
      : change.rule === "minor-fields"
        ? `Changed in ${fieldNames} only, which needs no approval.`
        : change.rule === "trusted"
          ? `By ${change.author}, whose entries need no review.`
          : change.identicalTo
            ? `${master.name} has this entry as it is: accepting links the two, nothing is added.`
            : "";
  const origin = change.kind === "removed" ? undefined : change.project.history.find((c) => c.segment);
  const name = `d-${change.project.id}`;

  return (
    <article
      className={`change change-${change.kind}`}
      aria-labelledby={`h-${change.project.id}`}
    >
      <header className="change-head">
        <h3 id={`h-${change.project.id}`} className="change-title">
          <InLang code={src}>{head}</InLang>
        </h3>
        <span className={`tag tag-${change.kind}`}>{kindLabel}</span>
        {origin && (
          <span className="change-origin">
            {origin.by}, {fmtTime(origin.at)}
          </span>
        )}
      </header>

      {ruleNote && <p className="change-rule">{ruleNote}</p>}

      {change.kind === "removed" && (
        <>
          <dl className="diff">
            {master.languages.map((code) => (
              <div key={code} className="diff-row">
                <dt>
                  <LangChip code={code} />
                </dt>
                <dd>
                  <del>
                    <TermsLine entry={change.project} code={code} />
                  </del>
                </dd>
              </div>
            ))}
          </dl>
          <p className="change-context">
            Left out {change.leftOutAt ? fmtTime(change.leftOutAt) : ""}
            {change.author ? ` by ${change.author}` : ""}. Accepting deletes the
            entry in {master.name}; it can be brought back from Deleted there.
            {change.masterHistory[0]
              ? ` ${master.name} changed it since, last ${change.masterHistory[change.masterHistory.length - 1]!.by} on ${fmtDate(change.masterHistory[change.masterHistory.length - 1]!.at)}.`
              : ""}
          </p>
        </>
      )}

      {change.kind === "new" && (
        <dl className="diff">
          {master.languages.map((code) => (
            <div key={code} className="diff-row">
              <dt>
                <LangChip code={code} />
              </dt>
              <dd>
                <TermsLine entry={change.project} code={code} />
              </dd>
            </div>
          ))}
        </dl>
      )}

      {change.kind === "changed" && (
        <ul className="diff-list">
          {change.terms.map((t) => (
            <li key={t.lang}>
              <LangChip code={t.lang} />
              {t.added.map((a) => (
                <ins key={a}>
                  <InLang code={t.lang}>{a}</InLang>
                </ins>
              ))}
              {t.removed.map((r) => (
                <del key={r}>
                  <InLang code={t.lang}>{r}</InLang>
                </del>
              ))}
            </li>
          ))}
          {change.fields.map((f) => (
            <li key={f.field}>
              <span className="diff-field">{field(f.field)}</span>
              {f.after ? <ins>{f.after}</ins> : <del>{f.before}</del>}
            </li>
          ))}
        </ul>
      )}

      {change.kind === "conflict" && change.master && (
        <div className="conflict">
          <div className="conflict-side">
            <h4>This project</h4>
            {change.terms.map((t) => (
              <p key={t.lang}>
                <LangChip code={t.lang} />{" "}
                <TermsLine entry={change.project} code={t.lang} />
              </p>
            ))}
          </div>
          <div className="conflict-side">
            <h4>
              {master.name}, changed{" "}
              {change.masterHistory[0]
                ? `by ${change.masterHistory[0].by} on ${fmtDate(change.masterHistory[0].at)}`
                : "meanwhile"}
            </h4>
            {change.terms.map((t) => (
              <p key={t.lang}>
                <LangChip code={t.lang} />{" "}
                <TermsLine entry={change.master!} code={t.lang} />
              </p>
            ))}
          </div>
        </div>
      )}

      {origin?.segment && (
        <p className="change-context">
          From: <q lang={src}>{origin.segment}</q>
        </p>
      )}

      <div
        className="decision"
        role="radiogroup"
        aria-label={`Decision for ${head}`}
      >
        {change.kind === "conflict" ? (
          <>
            <label className="check">
              <input
                type="radio"
                name={name}
                checked={decision === "accept"}
                onChange={() => onDecide("accept")}
              />
              <span>{w("useProject")}</span>
            </label>
            <label className="check">
              <input
                type="radio"
                name={name}
                checked={decision === "keep-master"}
                onChange={() => onDecide("keep-master")}
              />
              <span>{w("keepMaster")}</span>
            </label>
            <label className="check">
              <input
                type="radio"
                name={name}
                checked={decision === "keep-both"}
                onChange={() => onDecide("keep-both")}
              />
              <span>{w("keepBoth")}, as synonyms</span>
            </label>
          </>
        ) : (
          <label className="check">
            <input
              type="checkbox"
              checked={decision !== "skip"}
              onChange={(e) => onDecide(e.target.checked ? "accept" : "skip")}
            />
            <span>{change.kind === "removed" ? `Delete in ${master.name}` : `${w("accept")} in the merge`}</span>
          </label>
        )}
      </div>
    </article>
  );
}
