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
import { fmtDate, fmtTime, inSentence, plural } from "../format";
import { InLang, LangChip, LocationBadge, PageHeader } from "../components/ui";
import { tl, tlCore, tlx } from "../i18n";

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
          <p className="empty-title">{tl("Nothing to merge")}</p>
          <p>{tl("This project writes directly into its master or has no master.")}</p>
          <a className="btn btn-small" href="#/projects">
            {tl("Back to projects")}
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
            {tl("New and changed terms from this project go into its master. The master changes only when you merge.")}
          </>
        }
      >
        <a className="btn btn-quiet" href="#/projects">
          {tl("Back to projects")}
        </a>
      </PageHeader>

      {masters.length > 1 && (
        <div className="tabs" role="tablist" aria-label={tl("Masters")}>
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
          {state.gitWords ? tl("Merged {commits} into {name}", { commits: plural(done, "commit", "commits"), name: master.name }) : tl("Merged {changes} into {name}", { changes: plural(done, "change", "changes"), name: master.name })}</h2>
        <p>
          {tlx("Every merged entry keeps its {history}: who added it, when, and the sentence it came from.", { history: inSentence(w("history")) })}
        </p>
        <div className="row">
          <a className="btn btn-primary" href={`#/terminology/${master.id}`}>
            {tlx("Open {name}", { name: master.name })}</a>
          <a className="btn btn-quiet" href="#/projects">
            {tl("Back to projects")}
          </a>
        </div>
      </div>
    );
  }

  return (
    <>
      <section className="merge-head" aria-label={tl("Termbases")}>
        <div className="merge-side">
          <span className="merge-label">{tl("From")}</span>
          <span className="cell-title">{project.name}</span>
          <span className="cell-sub">
            {w("projectTermbase")}{" "}
            <LocationBadge location={project.setup.location} />
          </span>
        </div>
        <GitMerge size={20} aria-hidden="true" className="merge-arrow" />
        <div className="merge-side">
          <span className="merge-label">{tl("Into")}</span>
          <span className="cell-title">{master.name}</span>
          <span className="cell-sub">
            {tl("Master")}{" "}<LocationBadge location={master.location} server={master.server} />
          </span>
        </div>
        <dl className="merge-counts">
          <div>
            <dt>{w("conflicts")}</dt>
            <dd className={counts.conflict ? "count-conflict" : undefined}>
              {counts.conflict}
            </dd>
          </div>
          <div>
            <dt>{w("changedEntry")}</dt>
            <dd>{counts.changed}</dd>
          </div>
          <div>
            <dt>{tl("New")}</dt>
            <dd>{counts.new}</dd>
          </div>
          {counts.removed > 0 && (
            <div>
              <dt>{tl("Left out")}</dt>
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
              {tlx("{name} has {incomingCount} since {branchedAt}", { name: master.name, incomingCount: plural(incomingCount, "change", "changes"), branchedAt: fmtDate(branchedAt) })}</h2>
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
          <p className="empty-title">{state.gitWords ? tl("No commits to merge") : tl("No changes to merge")}</p>
          <p>{tl("Terms you add or change in this project appear here.")}</p>
        </div>
      ) : (
        <>
          <div className="segmented" role="radiogroup" aria-label={tl("Show")}>
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
                    ? tl("All")
                    : k === "conflict"
                      ? w("conflicts")
                      : k === "changed"
                        ? w("changedEntry")
                        : k === "removed"
                          ? tl("Left out")
                          : tl("New")}{" "}
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
                {tlx("Need no review ({length})", { length: covered.length })}
              </h2>
              <p className="help">
                {tlx("The rules of {name} cover these. They are merged with the rest unless you take them out.", { name: master.name })}
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
                ? tl("Resolve {unresolved} to merge.", { unresolved: plural(unresolved, "conflict", "conflicts") })
                : tl("{included} will be merged. What you do not accept stays in the project.", { included: (state.gitWords ? plural(included, "commit", "commits") : plural(included, "change", "changes")) })}
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
                  state.gitWords
                    ? tl("Merged {commits} into {name}", { commits: plural(included, "commit", "commits"), name: master.name })
                    : tl("Merged {changes} into {name}", { changes: plural(included, "change", "changes"), name: master.name }),
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
        {tl("{what}, {by}, {at}", { what: created ? tl("New entry") : tlCore(last.summary), by: last.by, at: fmtDate(last.at) })}
      </span>
    </>
  );
}

function TermsLine({ entry, code }: { entry: Entry; code: string }) {
  const ts = entry.terms[code] ?? [];
  if (!ts.length) return <span className="missing">{tl("no term")}</span>;
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
  const head = change.project.terms[src]?.[0]?.text ?? tl("(no term)");
  const field = (k: string) =>
    master.infoTypes.find((i) => i.id === k)?.name ?? k;
  const kindLabel =
    change.kind === "conflict"
      ? w("conflict")
      : change.kind === "changed"
        ? w("changedEntry")
        : change.kind === "removed"
          ? tl("Left out of the project")
          : w("newEntry");
  const fieldNames = change.fields.map((f) => field(f.field)).join(", ");
  const ruleNote =
    change.rule === "identical"
      ? tl("{name} has this entry as it is: the two are linked, nothing is added.", { name: master.name })
      : change.rule === "minor-fields"
        ? tl("Changed in {fieldNames} only, which needs no approval.", { fieldNames })
        : change.rule === "trusted"
          ? tl("By {author}, whose entries need no review.", { author: change.author })
          : change.identicalTo
            ? tl("{name} has this entry as it is: accepting links the two, nothing is added.", { name: master.name })
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
            {(change.author ? tl("Left out {at} by {by}.", { at: change.leftOutAt ? fmtTime(change.leftOutAt) : "", by: change.author }) : tl("Left out {at}.", { at: change.leftOutAt ? fmtTime(change.leftOutAt) : "" })) + " "}
            {tl("Accepting deletes the entry in {name}; it can be brought back from Deleted there.", { name: master.name })}
            {change.masterHistory[0] &&
              " " +
                tl("{name} changed it since, last {by} on {at}.", {
                  name: master.name,
                  by: change.masterHistory[change.masterHistory.length - 1]!.by,
                  at: fmtDate(change.masterHistory[change.masterHistory.length - 1]!.at),
                })}</p>
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
            <h4>{tl("This project")}</h4>
            {change.terms.map((t) => (
              <p key={t.lang}>
                <LangChip code={t.lang} />{" "}
                <TermsLine entry={change.project} code={t.lang} />
              </p>
            ))}
          </div>
          <div className="conflict-side">
            <h4>
              {change.masterHistory[0]
                ? tl("{name}, changed by {by} on {at}", { name: master.name, by: change.masterHistory[0].by, at: fmtDate(change.masterHistory[0].at) })
                : tl("{name}, changed meanwhile", { name: master.name })}</h4>
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
          {tlx("From: {q}", { q: <q lang={src}>{origin.segment}</q> })}</p>
      )}

      <div
        className="decision"
        role="radiogroup"
        aria-label={tl("Decision for {head}", { head })}
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
              <span>{w("keepBothSynonyms")}</span>
            </label>
          </>
        ) : (
          <label className="check">
            <input
              type="checkbox"
              checked={decision !== "skip"}
              onChange={(e) => onDecide(e.target.checked ? "accept" : "skip")}
            />
            <span>{change.kind === "removed" ? tl("Delete in {name}", { name: master.name }) : tl("{w} in the merge", { w: w("accept") })}</span>
          </label>
        )}
      </div>
    </article>
  );
}
